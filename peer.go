// Originally derived from: btcsuite/btcd/peer.go
// Copyright (c) 2013-2015 Conformal Systems LLC.

// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	prand "math/rand"
	"net"
	"strconv"
	"sync"
	"time"
	"errors"

	"github.com/monetas/bmd/addrmgr"
	"github.com/monetas/bmd/bmpeer"
	"github.com/monetas/bmutil/wire"
)

const (
	// maxProtocolVersion is the max protocol version the peer supports.
	maxProtocolVersion = 3

	// outputBufferSize is the number of elements the output channels use.
	outputBufferSize = 50

	// invTrickleSize is the maximum amount of inventory to send in a single
	// message when trickling inventory to remote peers.
	maxInvTrickleSize = 1000

	// maxKnownInventory is the maximum number of items to keep in the known
	// inventory cache.
	maxKnownInventory = 1000

	// negotiateTimeoutSeconds is the number of seconds of inactivity before
	// we timeout a peer that hasn't completed the initial version
	// negotiation.
	negotiateTimeoutSeconds = 30

	// idleTimeoutMinutes is the number of minutes of inactivity before
	// we time out a peer.
	idleTimeoutMinutes = 5

	// pingTimeoutMinutes is the number of minutes since we last sent a
	// message requiring a reply before we will ping a host.
	// TODO implement this rule.
	pingTimeoutMinutes = 2
)

var (
	defaultStreamList = []uint32{1}

	// userAgentName is the user agent name and is used to help identify
	// ourselves to other bitmessage peers.
	userAgentName = "bmd"

	// userAgentVersion is the user agent version and is used to help
	// identify ourselves to other bitmessage peers.
	userAgentVersion = fmt.Sprintf("%d.%d.%d", 0, 0, 1)
)

// peer provides a bitmessage peer for handling bitmessage communications. The
// overall data flow is split into 3 goroutines and a separate object manager.
// Inbound messages are read via the inHandler goroutine and generally
// dispatched to their own handler. For inbound data-related messages such as
// blocks, transactions, and inventory, the data is passed on to the block
// manager to handle it. Outbound messages are queued via QueueMessage or
// QueueInventory. QueueMessage is intended for all messages, including
// responses to data such as blocks and transactions. QueueInventory, on the
// other hand, is only intended for relaying inventory as it employs a trickling
// mechanism to batch the inventory together. The data flow for outbound
// messages uses two goroutines, queueHandler and outHandler. The first,
// queueHandler, is used as a way for external entities (mainly block manager)
// to queue messages quickly regardless of whether the peer is currently
// sending or not. It acts as the traffic cop between the external world and
// the actual goroutine which writes to the network socket. In addition, the
// peer contains several functions which are of the form pushX, that are used
// to push messages to the peer. Internally they use QueueMessage.
type peer struct {
	server            *server
	bmnet             wire.BitmessageNet
	//started           int32
	//connected         int32
	//disconnect        int32 // only to be used atomically
	//conn              bmpeer.Connection
	sendQueue         bmpeer.SendQueue
	inventory         *bmpeer.Inventory
	addr              net.Addr
	na                *wire.NetAddress
	inbound           bool
	//persistent        bool
	knownAddresses    map[string]struct{}
	//retryCount        int64
	StatsMtx          sync.Mutex // protects all statistics below here.
	versionKnown      bool
	versionSent       bool
	verAckReceived    bool
	handshakeComplete bool
	protocolVersion   uint32
	services          wire.ServiceFlag
	//timeConnected     time.Time
	//bytesReceived     uint64
	//bytesSent         uint64
	userAgent         string
}

// VersionKnown returns the whether or not the version of a peer is known locally.
// It is safe for concurrent access.
func (p *peer) VersionKnown() bool {
	p.StatsMtx.Lock()
	defer p.StatsMtx.Unlock()

	return p.versionKnown
}

// HandshakeComplete returns the whether or not the version of a peer is known locally.
// It is safe for concurrent access.
func (p *peer) HandshakeComplete() bool {
	p.StatsMtx.Lock()
	defer p.StatsMtx.Unlock()

	return p.handshakeComplete
}

func (p *peer) Addr() net.Addr {
	return p.addr
}

func (p *peer) NetAddress() *wire.NetAddress {
	return p.na
}

func (p *peer) Inbound() bool {
	return p.inbound
}

func (p *peer) State() bmpeer.PeerState {
	if p.HandshakeComplete() {
		return bmpeer.PeerStateHandshakeComplete
	}
	if p.VersionKnown() {
		return bmpeer.PeerStateVersionKnown
	}
	return bmpeer.PeerStateNew
}

// ProtocolVersion returns the peer protocol version in a manner that is safe
// for concurrent access.
func (p *peer) ProtocolVersion() uint32 {
	p.StatsMtx.Lock()
	defer p.StatsMtx.Unlock()

	return p.protocolVersion
}

// pushVersionMsg sends a version message to the connected peer using the
// current state.
// TODO don't send at an inappropriate time. 
func (p *peer) PushVersionMsg() {
	theirNa := p.na
	// p.na could be nil if this is an inbound peer but the version message
	// has not yet been processed. 
	if theirNa == nil {
		return
	}

	// Version message.
	msg := wire.NewMsgVersion(
		p.server.addrManager.GetBestLocalAddress(p.na), theirNa,
		p.server.nonce, defaultStreamList)
	msg.AddUserAgent(userAgentName, userAgentVersion)

	msg.AddrYou.Services = wire.SFNodeNetwork
	msg.Services = wire.SFNodeNetwork

	// Advertise our max supported protocol version.
	msg.ProtocolVersion = maxProtocolVersion

	p.QueueMessage(msg)
	
	p.versionSent = true
}

func (p *peer) PushVerAckMsg() {
	p.QueueMessage(&wire.MsgVerAck{})
}

func max(x, y int) int {
	if x > y {
		return x
	} else {
		return y
	}
}

func (p *peer) PushGetDataMsg(invVect []*wire.InvVect) {
	ivl := p.inventory.FilterRequested(invVect)
	
	if len(ivl) == 0 {
		return
	}

	x := 0
	for len(ivl) - x > wire.MaxInvPerMsg {
		p.QueueMessage(&wire.MsgInv{ivl[x:x+wire.MaxInvPerMsg]})
		x += wire.MaxInvPerMsg
	}
	
	p.QueueMessage(&wire.MsgGetData{ivl[x:]})
}

func (p *peer) PushInvMsg(invVect []*wire.InvVect) {
	ivl := p.inventory.FilterKnown(invVect)

	x := 0
	for len(ivl) - x > wire.MaxInvPerMsg {
		p.QueueMessage(&wire.MsgInv{ivl[x:x+wire.MaxInvPerMsg]})
		x += wire.MaxInvPerMsg
	}
	
	if len(ivl) > 0 {
		p.QueueMessage(&wire.MsgInv{ivl[x:]})
	}
}

// pushObjectMsg sends an object message for the provided object hash to the
// connected peer.  An error is returned if the block hash is not known.
func (p *peer) PushObjectMsg(sha *wire.ShaHash) {
	obj, err := p.server.db.FetchObjectByHash(sha)
	if err != nil {
		return
	}
	
	msg, err := wire.DecodeMsgObject(obj)
	if err != nil {
		return
	}
	p.QueueMessage(msg)
}

// pushAddrMsg sends one, or more, addr message(s) to the connected peer using
// the provided addresses.
func (p *peer) PushAddrMsg(addresses []*wire.NetAddress) error {
	// Nothing to send.
	if len(addresses) == 0 {
		return errors.New("Address list is empty.")
	}

	r := prand.New(prand.NewSource(time.Now().UnixNano()))
	numAdded := 0
	msg := wire.NewMsgAddr()
	for _, na := range addresses {
		// Filter addresses the peer already knows about.
		if _, exists := p.knownAddresses[addrmgr.NetAddressKey(na)]; exists {
			continue
		}

		// If the maxAddrs limit has been reached, randomize the list
		// with the remaining addresses.
		if numAdded == wire.MaxAddrPerMsg {
			msg.AddrList[r.Intn(wire.MaxAddrPerMsg)] = na
			continue
		}

		// Add the address to the message.
		err := msg.AddAddress(na)
		if err != nil {
			return errors.New("Address could not be added.")
		}
		numAdded++
	}
	if numAdded > 0 {
		for _, na := range msg.AddrList {
			// Add address to known addresses for this peer.
			p.knownAddresses[addrmgr.NetAddressKey(na)] = struct{}{}
		}

		p.QueueMessage(msg)
		return nil
	}
	return errors.New("No addresses added.")
}

func (p *peer) QueueMessage(msg wire.Message) {
	p.sendQueue.QueueMessage(msg)
}

// updateAddresses potentially adds addresses to the address manager and
// requests known addresses from the remote peer depending on whether the peer
// is an inbound or outbound peer and other factors such as address routability
// and the negotiated protocol version.
func (p *peer) updateAddresses(msg *wire.MsgVersion) {
	// Outbound connections.
	// TODO figure out how to refactor this.
	if !p.inbound {
		// TODO(davec): Only do this if not doing the initial block
		// download and the local address is routable.
		// Get address that best matches.
		lna := p.server.addrManager.GetBestLocalAddress(p.na)
		if addrmgr.IsRoutable(lna) {
			addresses := []*wire.NetAddress{lna}
			p.PushAddrMsg(addresses)
		}

		// Mark the address as a known good address.
		p.server.addrManager.Good(p.na)
	} else {
		// A peer might not be advertising the same address that it
		// actually connected from. One example of why this can happen
		// is with NAT. Only add the address to the address manager if
		// the addresses agree.
		if addrmgr.NetAddressKey(msg.AddrMe) == addrmgr.NetAddressKey(p.na) {
			p.server.addrManager.AddAddress(p.na, p.na)
			p.server.addrManager.Good(p.na)
		}
	}
}

// HandleVersionMsg is invoked when a peer receives a version bitmessage message
// and is used to negotiate the protocol version details as well as kick start
// the communications.
func (p *peer) HandleVersionMsg(msg *wire.MsgVersion) error {
	// Detect self connections.
	if msg.Nonce == p.server.nonce {
		return errors.New("Self connection detected.")
	}

	// Updating a bunch of stats.
	p.StatsMtx.Lock()

	// Limit to one version message per peer.
	if p.versionKnown {
		p.StatsMtx.Unlock()

		return errors.New("Only one version message allowed per peer.")
	}
	p.versionKnown = true
	
	// Set the supported services for the peer to what the remote peer
	// advertised.
	p.services = msg.Services

	// Set the remote peer's user agent.
	p.userAgent = msg.UserAgent

	p.StatsMtx.Unlock()

	// Inbound connections.
	// TODO
	if p.inbound {
		// Set up a NetAddress for the peer to be used with AddrManager.
		// We only do this inbound because outbound set this up
		// at connection time and no point recomputing.
		// We only use the first stream number for now because bitmessage has
		// only one stream.
		na, err := wire.NewNetAddress(p.addr, uint32(msg.StreamNumbers[0]), p.services)
		if err != nil {
			return errors.New(fmt.Sprintf("Can't send version message: %s",err))
		}
		p.na = na

		// Send version.
		p.PushVersionMsg()
	}

	// Send verack.
	p.QueueMessage(wire.NewMsgVerAck())

	// Update the address manager.
	p.updateAddresses(msg)
	
	p.handleInitialConnection()
	return nil
}

func (p *peer) HandleVerAckMsg() error {
	// If no version message has been sent disconnect.
	if !p.versionSent {
		return errors.New("Version not yet received.")
	}
	
	p.verAckReceived = true
	p.handleInitialConnection()
	return nil
}

// HandleInvMsg is invoked when a peer receives an inv bitmessage message and is
// used to examine the inventory being advertised by the remote peer and react
// accordingly. We pass the message down to objectmanager which will call
// QueueMessage with any appropriate responses.
func (p *peer) HandleInvMsg(msg *wire.MsgInv) error {
	if !p.HandshakeComplete() {
		return errors.New("Handshake not complete.")
	}
	
	// Disconnect if the message is too big. 
	if len(msg.InvList) > wire.MaxInvPerMsg || len(msg.InvList) == 0 {
		return errors.New("Incorrect inv size.")
	}
	
	// Add inv to known inventory. 
	for _, invVect := range msg.InvList {
		p.inventory.AddKnownInventory(invVect)
	}
	
 	p.server.objectManager.QueueInv(msg, p)
	return nil
}

// HandleGetData is invoked when a peer receives a getdata message and
// is used to deliver object information.
func (p *peer) HandleGetDataMsg(msg *wire.MsgGetData) error {
	if !p.HandshakeComplete() {
		return errors.New("Handshake not complete.")
	}
	
	err := p.sendQueue.QueueDataRequest(msg.InvList)
	if err != nil {
		return err
	}
	return nil
}

// 
func (p *peer) HandleObjectMsg(msg wire.Message) error {
	if !p.HandshakeComplete() {
		return errors.New("Handshake not complete.")
	}
	
	// TODO should we disconnect the peer if the object was not requested? 
	// We don't remember everything that has been requested necessarily. 

	p.inventory.DeleteRequest(&wire.InvVect{*wire.MessageHash(msg)})
	
	// Send the object to the object handler to be handled. 
	p.server.objectManager.handleObjectMsg(msg)
	return nil
}

// HandleAddrMsg is invoked when a peer receives an addr bitmessage message and
// is used to notify the server about advertised addresses.
func (p *peer) HandleAddrMsg(msg *wire.MsgAddr) error {
	if !p.HandshakeComplete() {
		return errors.New("Handshake not complete.")
	}
	
	// A message that has no addresses is invalid.
	if len(msg.AddrList) == 0 {
		return errors.New("Empty addr message received.")
	}

	for _, na := range msg.AddrList {

		// Set the timestamp to 5 days ago if it's more than 24 hours
		// in the future so this address is one of the first to be
		// removed when space is needed.
		now := time.Now()
		if na.Timestamp.After(now.Add(time.Minute * 10)) {
			na.Timestamp = now.Add(-1 * time.Hour * 24 * 5)
		}

		// Add address to known addresses for this peer.
		p.knownAddresses[addrmgr.NetAddressKey(na)] = struct{}{}
	}

	// Add addresses to server address manager. The address manager handles
	// the details of things such as preventing duplicate addresses, max
	// addresses, and last seen updates.
	p.server.addrManager.AddAddresses(msg.AddrList, p.na)
	return nil
}

// handleInitialConnection is called once the initial handshake is complete. 
func (p *peer) handleInitialConnection() {
	if !(p.VersionKnown()&&p.verAckReceived) {
		return
	}
	//The initial handshake is complete.
	
	p.StatsMtx.Lock()
	p.handshakeComplete = true
	p.StatsMtx.Unlock()

	// Signal the object manager that a new peer has been connected.
	p.server.objectManager.NewPeer(p)
	
	// Send a big addr message. 
	p.PushAddrMsg(p.server.addrManager.AddressCache())
	
	// Send a big inv message. 
	hashes, _ := p.server.db.FetchRandomInvHashes(wire.MaxInvPerMsg,
		func(*wire.ShaHash, []byte) bool {return true})
	invVectList := make([]*wire.InvVect, len(hashes))
	for i, hash := range hashes {
		invVectList[i] = &wire.InvVect{hash}
	}
	p.PushInvMsg(invVectList)
}

// newPeerBase returns a new base bitmessage peer for the provided server and
// inbound flag. This is used by the newInboundPeer and newOutboundPeer
// functions to perform base setup needed by both types of peers.
func newPeerBase(addr net.Addr, s *server, inventory *bmpeer.Inventory, sendQueue bmpeer.SendQueue, inbound bool) *peer {
	//inventory := bmpeer.NewInventory(s.db)
	p := &peer{
		server:          s,
		protocolVersion: maxProtocolVersion,
		bmnet:           wire.MainNet,
		services:        wire.SFNodeNetwork,
		inventory:       inventory, 
		sendQueue:       sendQueue, 
		addr:            addr, 
		knownAddresses:  make(map[string]struct{}), 
		inbound:         inbound, 
	}
	return p
}

// newInboundPeer returns a new inbound bitmessage peer for the provided server and
// connection. Use Start to begin processing incoming and outgoing messages.
func newInboundPeer(s *server, conn bmpeer.Connection) *bmpeer.Peer {
	inventory := bmpeer.NewInventory(s.db)
	sq := bmpeer.NewSendQueue(inventory)
	p := newPeerBase(conn.RemoteAddr(), s, inventory, sq, true)

	return bmpeer.NewPeer(p, conn, sq, false, 0)
}

// Can be swapped out for testing purposes. 
// TODO handle this more elegantly eventually. 
var NewConn func(net.Addr) bmpeer.Connection = bmpeer.NewConnection

// newOutbountPeer returns a new outbound bitmessage peer for the provided server and
// address and connects to it asynchronously. If the connection is successful
// then the peer will also be started.
func newOutboundPeer(addr string, s *server, stream uint32, persistent bool, retryCount int64) *bmpeer.Peer {
	// Setup p.na with a temporary address that we are connecting to with
	// faked up service flags. We will replace this with the real one after
	// version negotiation is successful. The only failure case here would
	// be if the string was incomplete for connection so can't be split
	// into address and port, and thus this would be invalid anyway. In
	// which case we return nil to be handled by the caller. This must be
	// done before we fork off the goroutine because as soon as this
	// function returns the peer must have a valid netaddress.
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil
	}

	na, err := s.addrManager.HostToNetAddress(host, uint16(port), stream, 0)
	if err != nil {
		return nil
	}

	tcpAddr := &net.TCPAddr{IP: net.ParseIP(host), Port: int(port)}
	conn := NewConn(tcpAddr)
	inventory := bmpeer.NewInventory(s.db)
	sq := bmpeer.NewSendQueue(inventory)
	logic := newPeerBase(tcpAddr, s, inventory, sq, false)

	p := bmpeer.NewPeer(logic, conn, sq, persistent, retryCount)
	
	logic.addr = tcpAddr
	logic.na = na

	go func() {
		// Wait for some time if this is a retry, and wait longer if we have
		// tried multiple times. 
		// TODO: this seems like the wrong place to handle this. Shouldn't the 
		// server manage which connections should be waited for? 
		if retryCount > 0 {
			scaledInterval := connectionRetryInterval.Nanoseconds() * retryCount / 2
			scaledDuration := time.Duration(scaledInterval)
			time.Sleep(scaledDuration)
		}
		
		if p.Start() != nil {
			return
		}

		s.addrManager.Attempt(na)
	}()
	return p
}
