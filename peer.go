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
	"sync/atomic"
	"time"

	"github.com/btcsuite/go-socks/socks"
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
	
	Dial = bmpeer.Dial
)

// newNetAddress attempts to extract the IP address and port from the passed
// net.Addr interface and create a bitmessage NetAddress structure using that
// information.
func newNetAddress(addr net.Addr, stream uint32, services wire.ServiceFlag) (*wire.NetAddress, error) {
	// addr will be a net.TCPAddr when not using a proxy.
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		ip := tcpAddr.IP
		port := uint16(tcpAddr.Port)
		na := wire.NewNetAddressIPPort(ip, port, stream, services)
		return na, nil
	}

	// addr will be a socks.ProxiedAddr when using a proxy.
	if proxiedAddr, ok := addr.(*socks.ProxiedAddr); ok {
		ip := net.ParseIP(proxiedAddr.Host)
		if ip == nil {
			ip = net.ParseIP("0.0.0.0")
		}
		port := uint16(proxiedAddr.Port)
		na := wire.NewNetAddressIPPort(ip, port, stream, services)
		return na, nil
	}

	// For the most part, addr should be one of the two above cases, but
	// to be safe, fall back to trying to parse the information from the
	// address string as a last resort.
	host, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}
	na := wire.NewNetAddressIPPort(ip, uint16(port), stream, services)
	return na, nil
}

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
	started           int32
	connected         int32
	disconnect        int32 // only to be used atomically
	conn              bmpeer.Connection
	sendQueue         bmpeer.SendQueue
	inventory         *bmpeer.Inventory
	addr              string
	na                *wire.NetAddress
	inbound           bool
	persistent        bool
	knownAddresses    map[string]struct{}
	retryCount        int64
	StatsMtx          sync.Mutex // protects all statistics below here.
	versionKnown      bool
	versionSent       bool
	verAckReceived    bool
	handshakeComplete bool
	protocolVersion   uint32
	services          wire.ServiceFlag
	timeConnected     time.Time
	bytesReceived     uint64
	bytesSent         uint64
	userAgent         string
}

// String returns the peer's address and directionality as a human-readable
// string.
func (p *peer) String() string {
	return fmt.Sprintf("%s (inbound: %s)", p.addr, p.inbound)
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

// ProtocolVersion returns the peer protocol version in a manner that is safe
// for concurrent access.
func (p *peer) ProtocolVersion() uint32 {
	p.StatsMtx.Lock()
	defer p.StatsMtx.Unlock()

	return p.protocolVersion
}

// pushVersionMsg sends a version message to the connected peer using the
// current state.
func (p *peer) PushVersionMsg() error {
	theirNa := p.na

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
	return nil
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
func (p *peer) PushObjectMsg(sha *wire.ShaHash) error {
	obj, err := p.server.db.FetchObjectByHash(sha)
	if err != nil {
		return err
	}
	
	msg, err := wire.DecodeMsgObject(obj)
	if err != nil {
		return err
	}
	p.QueueMessage(msg)

	return nil
}

// pushAddrMsg sends one, or more, addr message(s) to the connected peer using
// the provided addresses.
func (p *peer) PushAddrMsg(addresses []*wire.NetAddress) error {
	// Nothing to send.
	if len(addresses) == 0 {
		return nil
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
			return err
		}
		numAdded++
	}
	if numAdded > 0 {
		for _, na := range msg.AddrList {
			// Add address to known addresses for this peer.
			p.knownAddresses[addrmgr.NetAddressKey(na)] = struct{}{}
		}

		p.QueueMessage(msg)
	}
	return nil
}

func (p *peer) QueueMessage(msg wire.Message) {
	if p.sendQueue.QueueMessage(msg) != nil {
		p.Disconnect()
	}
}

// updateAddresses potentially adds addresses to the address manager and
// requests known addresses from the remote peer depending on whether the peer
// is an inbound or outbound peer and other factors such as address routability
// and the negotiated protocol version.
func (p *peer) updateAddresses(msg *wire.MsgVersion) {
	// Outbound connections.
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

// handleVersionMsg is invoked when a peer receives a version bitmessage message
// and is used to negotiate the protocol version details as well as kick start
// the communications.
func (p *peer) handleVersionMsg(msg *wire.MsgVersion) {
	// Detect self connections.
	if msg.Nonce == p.server.nonce {
		p.Disconnect()
		return
	}

	// Updating a bunch of stats.
	p.StatsMtx.Lock()

	// Limit to one version message per peer.
	if p.versionKnown {
		p.logError("Only one version message per peer is allowed %s.",
			p)
		p.StatsMtx.Unlock()

		p.Disconnect()
		return
	}
	p.versionKnown = true
	
	// Set the supported services for the peer to what the remote peer
	// advertised.
	p.services = msg.Services

	// Set the remote peer's user agent.
	p.userAgent = msg.UserAgent

	p.StatsMtx.Unlock()

	// Inbound connections.
	if p.inbound {
		// Set up a NetAddress for the peer to be used with AddrManager.
		// We only do this inbound because outbound set this up
		// at connection time and no point recomputing.
		// We only use the first stream number for now because bitmessage has
		// only one stream.
		na, err := newNetAddress(p.conn.RemoteAddr(), uint32(msg.StreamNumbers[0]), p.services)
		if err != nil {
			p.logError("Can't get remote address: %v", err)
			p.Disconnect()
			return
		}
		p.na = na

		// Send version.
		err = p.PushVersionMsg()
		if err != nil {
			p.logError("Can't send version message to %s: %v",
				p, err)
			p.Disconnect()
			return
		}
	}

	// Send verack.
	p.QueueMessage(wire.NewMsgVerAck())

	// Update the address manager.
	p.updateAddresses(msg)
	
	p.handleInitialConnection()
}

func (p *peer) handleVerAckMsg() {
	// If no version message has been sent disconnect.
	if !p.versionSent {
		p.Disconnect()
	}
	
	p.verAckReceived = true
	p.handleInitialConnection()
}

// handleInvMsg is invoked when a peer receives an inv bitmessage message and is
// used to examine the inventory being advertised by the remote peer and react
// accordingly. We pass the message down to objectmanager which will call
// QueueMessage with any appropriate responses.
func (p *peer) handleInvMsg(msg *wire.MsgInv) {
	// Disconnect if the message is too big. 
	if len(msg.InvList) > wire.MaxInvPerMsg || len(msg.InvList) == 0 {
		p.Disconnect()
	}
	
	// Add inv to known inventory. 
	for _, invVect := range msg.InvList {
		p.inventory.AddKnownInventory(invVect)
	}
	
 	p.server.objectManager.QueueInv(msg, p)
}

// handleGetData is invoked when a peer receives a getdata message and
// is used to deliver object information.
// TODO this function used to wait for each message to be sent so as to avid
// using up too much memory. It still needs to be able to do that, but the
// redesign of peer made it unable to work the way it was originally designed. 
func (p *peer) handleGetDataMsg(msg *wire.MsgGetData) {
	for _, iv := range msg.InvList {
		p.PushObjectMsg(&iv.Hash)
	}
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

// 
func (p *peer) handleObjectMsg(msg wire.Message) {
	
	// TODO should we disconnect the peer if the object was not requested? 
	// We don't remember everything that has been requested necessarily. 

	p.inventory.DeleteRequest(&wire.InvVect{*wire.MessageHash(msg)})
	
	// Send the object to the object handler to be handled. 
	p.server.objectManager.handleObjectMsg(msg)
}

// handleAddrMsg is invoked when a peer receives an addr bitmessage message and
// is used to notify the server about advertised addresses.
func (p *peer) handleAddrMsg(msg *wire.MsgAddr) {

	// A message that has no addresses is invalid.
	if len(msg.AddrList) == 0 {
		p.logError("Command [%s] from %s does not contain any addresses",
			msg.Command(), p)
		p.Disconnect()
		return
	}

	for _, na := range msg.AddrList {
		// Don't add more address if we're disconnecting.
		if atomic.LoadInt32(&p.disconnect) != 0 {
			return
		}

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
}

// inHandler handles all incoming messages for the peer. It must be run as a
// goroutine.
func (p *peer) inHandler() {
	// peers must complete the initial version negotiation within a shorter
	// timeframe than a general idle timeout. The timer is then reset below
	// to idleTimeoutMinutes for all future messages.
	idleTimer := time.AfterFunc(negotiateTimeoutSeconds*time.Second, func() {
		p.Disconnect()
	})
out:
	for atomic.LoadInt32(&p.disconnect) == 0 {
		rmsg, err := p.conn.ReadMessage()
		// Stop the timer now, if we go around again we will reset it.
		idleTimer.Stop()
		if err != nil {
			break out
		}

		// Handle each supported message type.
		markConnected := false
		if !p.HandshakeComplete() {
			switch msg := rmsg.(type) {
			case *wire.MsgVersion:
				p.handleVersionMsg(msg)
				markConnected = true
	
			case *wire.MsgVerAck:
				p.handleVerAckMsg()
				markConnected = true
				
			default :
				p.Disconnect()
			}
			
			// reset the timer.
			idleTimer.Reset(negotiateTimeoutSeconds*time.Second)
		} else {
			switch msg := rmsg.(type) {
			case *wire.MsgVersion:
				p.handleVersionMsg(msg)
				markConnected = true
	
			case *wire.MsgVerAck:
				markConnected = true

			case *wire.MsgAddr:
				p.handleAddrMsg(msg)
				markConnected = true
	
			case *wire.MsgInv:
				p.handleInvMsg(msg)
				markConnected = true
	
			case *wire.MsgGetData:
				getData := rmsg.(*wire.MsgGetData)
				p.sendQueue.QueueDataRequest(getData.InvList)
				markConnected = true

			case *wire.MsgGetPubKey, *wire.MsgPubKey, *wire.MsgMsg, *wire.MsgBroadcast, *wire.MsgUnknownObject:
				p.handleObjectMsg(msg)
				markConnected = true
			}
			
			// reset the timer.
			idleTimer.Reset(idleTimeoutMinutes * time.Minute)
		}

		// Mark the address as currently connected and working as of
		// now if one of the messages that trigger it was processed.
		if markConnected && atomic.LoadInt32(&p.disconnect) == 0 {
			if p.na == nil {
				continue
			}
			p.server.addrManager.Connected(p.na)
		}
		// ok we got a message, reset the timer.
		// timer just calls p.Disconnect() after logging.
		idleTimer.Reset(idleTimeoutMinutes * time.Minute)
		p.retryCount = 0
	}

	idleTimer.Stop()

	// Ensure connection is closed and notify the server that the peer is
	// done.
	p.Disconnect()

	// Only tell block manager we are gone if we ever told it we existed.
	if p.HandshakeComplete() {
		p.server.objectManager.DonePeer(p)
	}
	
	p.server.donePeers <- p
}

// Connected returns whether or not the peer is currently connected.
func (p *peer) Connected() bool {
	return atomic.LoadInt32(&p.connected) != 0 &&
		atomic.LoadInt32(&p.disconnect) == 0
}

// Disconnect disconnects the peer by closing the connection. It also sets
// a flag so the impending shutdown can be detected.
func (p *peer) Disconnect() {
	// did we win the race?
	if atomic.AddInt32(&p.disconnect, 1) != 1 {
		return
	}
	
	//The send que could possibly be nil if Start() was never called on this peer.
	if p.sendQueue != nil {
		p.sendQueue.Stop()
	}
	if atomic.LoadInt32(&p.connected) != 0 {
		p.conn.Close()
	}
}

// Start begins processing input and output messages. It also sends the initial
// version message for outbound connections to start the negotiation process.
func (p *peer) Start() error {
	// Already started?
	if atomic.AddInt32(&p.started, 1) != 1 {
		return nil
	}

	p.sendQueue.Start(p.conn)

	// Send an initial version message if this is an outbound connection.
	if !p.inbound {
		err := p.PushVersionMsg()
		if err != nil {
			p.logError("Can't send outbound version message %v", err)
			p.Disconnect()
			return err
		}
		p.versionSent = true
	}

	// Start processing input and output.
	go p.inHandler()

	return nil
}

// Shutdown gracefully shuts down the peer by disconnecting it.
func (p *peer) Shutdown() {
	p.Disconnect()
}

// logError makes sure that we only log errors loudly on user peers.
func (p *peer) logError(fmt string, args ...interface{}) {
	if p.persistent {
	} else {
	}
}

// newPeerBase returns a new base bitmessage peer for the provided server and
// inbound flag. This is used by the newInboundPeer and newOutboundPeer
// functions to perform base setup needed by both types of peers.
func newPeerBase(s *server, inbound bool) *peer {
	inventory := bmpeer.NewInventory(s.db)
	p := peer{
		server:          s,
		protocolVersion: maxProtocolVersion,
		bmnet:           wire.MainNet,
		services:        wire.SFNodeNetwork,
		inbound:         inbound,
		inventory:       inventory, 
		sendQueue:       bmpeer.NewSendQueue(inventory), 
		knownAddresses:  make(map[string]struct{}), 
	}
	return &p
}

// newInboundPeer returns a new inbound bitmessage peer for the provided server and
// connection. Use Start to begin processing incoming and outgoing messages.
func newInboundPeer(s *server, conn bmpeer.Connection) *peer {
	p := newPeerBase(s, true)
	p.conn = conn
	p.addr = conn.RemoteAddr().String()
	p.timeConnected = time.Now()
	atomic.AddInt32(&p.connected, 1)
	return p
}

// newOutbountPeer returns a new outbound bitmessage peer for the provided server and
// address and connects to it asynchronously. If the connection is successful
// then the peer will also be started.
func newOutboundPeer(s *server, addr string, persistent bool, retryCount int64, stream uint32) *peer {
	p := newPeerBase(s, false)
	p.addr = addr
	p.persistent = persistent
	p.retryCount = retryCount
	p.versionSent = true

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
		p.logError("Tried to create a new outbound peer with invalid "+
			"address %s: %v", addr, err)
		return nil
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		p.logError("Tried to create a new outbound peer with invalid "+
			"port %s: %v", portStr, err)
		return nil
	}

	p.na, err = s.addrManager.HostToNetAddress(host, uint16(port), stream, 0)
	if err != nil {
		p.logError("Can not turn host %s into netaddress: %v",
			host, err)
		return nil
	}

	go func() {
		if atomic.LoadInt32(&p.disconnect) != 0 {
			return
		}
		if p.retryCount > 0 {
			scaledInterval := connectionRetryInterval.Nanoseconds() * p.retryCount / 2
			scaledDuration := time.Duration(scaledInterval)
			time.Sleep(scaledDuration)
		}
		conn, err := Dial("tcp", addr)
		if err != nil {
			p.server.donePeers <- p
			return
		}

		// We may have slept and the server may have scheduled a shutdown. In that
		// case ditch the peer immediately.
		if atomic.LoadInt32(&p.disconnect) == 0 {
			p.timeConnected = time.Now()
			p.server.addrManager.Attempt(p.na)

			// Connection was successful so log it and start peer.
			p.conn = conn
			atomic.AddInt32(&p.connected, 1)
			p.Start()
		}
	}()
	return p
}
