package main

import (
	"container/list"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	prand "math/rand"
	"net"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/go-socks/socks"
	"github.com/monetas/bmd/addrmgr"
	"github.com/monetas/bmd/wire"
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
	pingTimeoutMinutes = 2
)

var (
	// userAgentName is the user agent name and is used to help identify
	// ourselves to other bitmessage peers.
	userAgentName = "bmd"

	// userAgentVersion is the user agent version and is used to help
	// identify ourselves to other bitmessage peers.
	userAgentVersion = fmt.Sprintf("%d.%d.%d", 0, 0, 1)

	defaultStreamList = []uint32{1}

	shutdownChannel = make(chan struct{})
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
		fmt.Printf("err: %v\n", err)
		debug.PrintStack()
		return nil, err
	}
	ip := net.ParseIP(host)
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		fmt.Printf("err: %v\n", err)
		debug.PrintStack()
		return nil, err
	}
	na := wire.NewNetAddressIPPort(ip, uint16(port), stream, services)
	return na, nil
}

// outMsg is used to house a message to be sent along with a channel to signal
// when the message has been sent (or won't be sent due to things such as
// shutdown)
type outMsg struct {
	msg      wire.Message
	doneChan chan struct{}
}

// peer provides a bitmessage peer for handling bitmessage communications. The
// overall data flow is split into 3 goroutines and a separate block manager.
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
	server          *server
	bmnet           wire.BitmessageNet
	started         int32
	connected       int32
	disconnect      int32 // only to be used atomically
	conn            net.Conn
	addr            string
	na              *wire.NetAddress
	inbound         bool
	persistent      bool
	knownAddresses  map[string]struct{}
	knownInventory  *MruInventoryMap
	knownInvMutex   sync.Mutex
	retryCount      int64
	requestQueue    []*wire.InvVect
	continueHash    *wire.ShaHash
	outputQueue     chan outMsg
	sendQueue       chan outMsg
	sendDoneQueue   chan struct{}
	queueWg         sync.WaitGroup // TODO(oga) wg -> single use channel?
	outputInvChan   chan *wire.InvVect
	quit            chan struct{}
	StatsMtx        sync.Mutex // protects all statistics below here.
	versionKnown    bool
	protocolVersion uint32
	services        wire.ServiceFlag
	timeConnected   time.Time
	lastSend        time.Time
	lastRecv        time.Time
	bytesReceived   uint64
	bytesSent       uint64
	userAgent       string
	streamNumbers   []uint32
}

// String returns the peer's address and directionality as a human-readable
// string.
func (p *peer) String() string {
	return fmt.Sprintf("%s (inbound: %s)", p.addr, p.inbound)
}

// isKnownInventory returns whether or not the peer is known to have the passed
// inventory. It is safe for concurrent access.
func (p *peer) isKnownInventory(invVect *wire.InvVect) bool {
	p.knownInvMutex.Lock()
	defer p.knownInvMutex.Unlock()

	if p.knownInventory.Exists(invVect) {
		return true
	}
	return false
}

// AddKnownInventory adds the passed inventory to the cache of known inventory
// for the peer. It is safe for concurrent access.
func (p *peer) AddKnownInventory(invVect *wire.InvVect) {
	p.knownInvMutex.Lock()
	defer p.knownInvMutex.Unlock()

	p.knownInventory.Add(invVect)
}

// VersionKnown returns the whether or not the version of a peer is known locally.
// It is safe for concurrent access.
func (p *peer) VersionKnown() bool {
	p.StatsMtx.Lock()
	defer p.StatsMtx.Unlock()

	return p.versionKnown
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
func (p *peer) pushVersionMsg() error {
	theirNa := p.na

	// Version message.
	msg := wire.NewMsgVersion(
		p.server.addrManager.GetBestLocalAddress(p.na), theirNa,
		p.server.nonce, p.streamNumbers)
	msg.AddUserAgent(userAgentName, userAgentVersion)

	msg.AddrYou.Services = wire.SFNodeNetwork
	msg.Services = wire.SFNodeNetwork

	// Advertise our max supported protocol version.
	msg.ProtocolVersion = maxProtocolVersion

	p.QueueMessage(msg, nil)
	return nil
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
			p.pushAddrMsg(addresses)
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

	// Set the supported services for the peer to what the remote peer
	// advertised.
	p.services = msg.Services

	// Set the remote peer's user agent.
	p.userAgent = msg.UserAgent
	p.streamNumbers = msg.StreamNumbers

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
			fmt.Printf("err: %v\n", err)
			debug.PrintStack()
			p.logError("Can't get remote address: %v", err)
			p.Disconnect()
			return
		}
		p.na = na

		// Send version.
		err = p.pushVersionMsg()
		if err != nil {
			fmt.Printf("err: %v\n", err)
			debug.PrintStack()
			p.logError("Can't send version message to %s: %v",
				p, err)
			p.Disconnect()
			return
		}
	}

	// Send verack.
	p.QueueMessage(wire.NewMsgVerAck(), nil)

	// Update the address manager and request known addresses from the
	// remote peer for outbound connections. This is skipped when running
	// on the simulation test network since it is only intended to connect
	// to specified peers and actively avoids advertising and connecting to
	// discovered peers.
	p.updateAddresses(msg)
}

// handleInvMsg is invoked when a peer receives an inv bitmessage message and is
// used to examine the inventory being advertised by the remote peer and react
// accordingly. We pass the message down to blockmanager which will call
// QueueMessage with any appropriate responses.
func (p *peer) handleInvMsg(msg *wire.MsgInv) {
	// TODO: send what we have
}

// handleGetData is invoked when a peer receives a getdata bitmessage message and
// is used to deliver information
func (p *peer) handleGetDataMsg(msg *wire.MsgGetData) {
	numAdded := 0

	// We wait on the this wait channel periodically to prevent queueing
	// far more data than we can send in a reasonable time, wasting memory.
	// The waiting occurs after the database fetch for the next one to
	// provide a little pipelining.
	doneChan := make(chan struct{}, 1)

	for _ = range msg.InvList {
		// TODO: add the thing being asked for to the return msg if we have it
		numAdded++
	}

	// Wait for messages to be sent. We can send quite a lot of data at this
	// point and this will keep the peer busy for a decent amount of time.
	// We don't process anything else by them in this time so that we
	// have an idea of when we should hear back from them - else the idle
	// timeout could fire when we were only half done sending the blocks.
	if numAdded > 0 {
		<-doneChan
	}
}

// pushAddrMsg sends one, or more, addr message(s) to the connected peer using
// the provided addresses.
func (p *peer) pushAddrMsg(addresses []*wire.NetAddress) error {
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
			fmt.Printf("err: %v\n", err)
			debug.PrintStack()
			return err
		}
		numAdded++
	}
	if numAdded > 0 {
		for _, na := range msg.AddrList {
			// Add address to known addresses for this peer.
			p.knownAddresses[addrmgr.NetAddressKey(na)] = struct{}{}
		}

		p.QueueMessage(msg, nil)
	}
	return nil
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

// readMessage reads the next bitmessage message from the peer with logging.
func (p *peer) readMessage() (wire.Message, []byte, error) {
	n, msg, buf, err := wire.ReadMessageN(p.conn, p.bmnet)
	p.StatsMtx.Lock()
	p.bytesReceived += uint64(n)
	p.StatsMtx.Unlock()
	p.server.AddBytesReceived(uint64(n))
	if err != nil {
		fmt.Printf("err: %v\n", err)
		debug.PrintStack()
		return nil, nil, err
	}

	return msg, buf, nil
}

// writeMessage sends a bitmessage Message to the peer with logging.
func (p *peer) writeMessage(msg wire.Message) {
	// Don't do anything if we're disconnecting.
	if atomic.LoadInt32(&p.disconnect) != 0 {
		return
	}
	if !p.VersionKnown() {
		switch msg.(type) {
		case *wire.MsgVersion:
			// This is OK.
		default:
			// Drop all messages other than version and reject if
			// the handshake has not already been done.
			return
		}
	}

	// Write the message to the peer.
	n, err := wire.WriteMessageN(p.conn, msg, p.bmnet)
	p.StatsMtx.Lock()
	p.bytesSent += uint64(n)
	p.StatsMtx.Unlock()
	p.server.AddBytesSent(uint64(n))
	if err != nil {
		fmt.Printf("err: %v\n", err)
		debug.PrintStack()
		p.Disconnect()
		p.logError("Can't send message to %s: %v", p, err)
		return
	}
}

// inHandler handles all incoming messages for the peer. It must be run as a
// goroutine.
func (p *peer) inHandler() {
	// Peers must complete the initial version negotiation within a shorter
	// timeframe than a general idle timeout. The timer is then reset below
	// to idleTimeoutMinutes for all future messages.
	idleTimer := time.AfterFunc(negotiateTimeoutSeconds*time.Second, func() {
		p.Disconnect()
	})
out:
	for atomic.LoadInt32(&p.disconnect) == 0 {
		rmsg, _, err := p.readMessage()
		// Stop the timer now, if we go around again we will reset it.
		idleTimer.Stop()
		if err != nil {
			fmt.Printf("err: %v\n", err)
			debug.PrintStack()
			break out
		}
		p.StatsMtx.Lock()
		p.lastRecv = time.Now()
		p.StatsMtx.Unlock()

		// Ensure version message comes first.
		if _, ok := rmsg.(*wire.MsgVersion); !ok && !p.VersionKnown() {
			errStr := "A version message must precede all others"
			p.logError(errStr)
			break out
		}

		// Handle each supported message type.
		markConnected := false
		switch msg := rmsg.(type) {
		case *wire.MsgVersion:
			p.handleVersionMsg(msg)
			markConnected = true

		case *wire.MsgVerAck:
			// Do nothing.

		case *wire.MsgAddr:
			p.handleAddrMsg(msg)
			markConnected = true

		case *wire.MsgInv:
			p.handleInvMsg(msg)
			markConnected = true

		case *wire.MsgGetData:
			p.handleGetDataMsg(msg)
			markConnected = true
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
	p.server.donePeers <- p
}

// queueHandler handles the queueing of outgoing data for the peer. This runs
// as a muxer for various sources of input so we can ensure that blockmanager
// and the server goroutine both will not block on us sending a message.
// We then pass the data on to outHandler to be actually written.
func (p *peer) queueHandler() {
	pendingMsgs := list.New()
	invSendQueue := list.New()
	trickleTicker := time.NewTicker(time.Second * 10)
	defer trickleTicker.Stop()

	// We keep the waiting flag so that we know if we have a message queued
	// to the outHandler or not. We could use the presence of a head of
	// the list for this but then we have rather racy concerns about whether
	// it has gotten it at cleanup time - and thus who sends on the
	// message's done channel. To avoid such confusion we keep a different
	// flag and pendingMsgs only contains messages that we have not yet
	// passed to outHandler.
	waiting := false

	// To avoid duplication below.
	queuePacket := func(msg outMsg, list *list.List, waiting bool) bool {
		if !waiting {
			p.sendQueue <- msg
		} else {
			list.PushBack(msg)
		}
		// we are always waiting now.
		return true
	}
out:
	for {
		select {
		case msg := <-p.outputQueue:
			waiting = queuePacket(msg, pendingMsgs, waiting)

		// This channel is notified when a message has been sent across
		// the network socket.
		case <-p.sendDoneQueue:
			// No longer waiting if there are no more messages
			// in the pending messages queue.
			next := pendingMsgs.Front()
			if next == nil {
				waiting = false
				continue
			}

			// Notify the outHandler about the next item to
			// asynchronously send.
			val := pendingMsgs.Remove(next)
			p.sendQueue <- val.(outMsg)

		case iv := <-p.outputInvChan:
			// No handshake?  They'll find out soon enough.
			if p.VersionKnown() {
				invSendQueue.PushBack(iv)
			}

		case <-trickleTicker.C:
			// Don't send anything if we're disconnecting or there
			// is no queued inventory.
			// version is known if send queue has any entries.
			if atomic.LoadInt32(&p.disconnect) != 0 ||
				invSendQueue.Len() == 0 {
				continue
			}

			// Create and send as many inv messages as needed to
			// drain the inventory send queue.
			invMsg := wire.NewMsgInv()
			for e := invSendQueue.Front(); e != nil; e = invSendQueue.Front() {
				iv := invSendQueue.Remove(e).(*wire.InvVect)

				// Don't send inventory that became known after
				// the initial check.
				if p.isKnownInventory(iv) {
					continue
				}

				invMsg.AddInvVect(iv)
				if len(invMsg.InvList) >= maxInvTrickleSize {
					waiting = queuePacket(
						outMsg{msg: invMsg},
						pendingMsgs, waiting)
					invMsg = wire.NewMsgInv()
				}

				// Add the inventory that is being relayed to
				// the known inventory for the peer.
				p.AddKnownInventory(iv)
			}
			if len(invMsg.InvList) > 0 {
				waiting = queuePacket(outMsg{msg: invMsg},
					pendingMsgs, waiting)
			}

		case <-p.quit:
			break out
		}
	}

	// Drain any wait channels before we go away so we don't leave something
	// waiting for us.
	for e := pendingMsgs.Front(); e != nil; e = pendingMsgs.Front() {
		val := pendingMsgs.Remove(e)
		msg := val.(outMsg)
		if msg.doneChan != nil {
			msg.doneChan <- struct{}{}
		}
	}
cleanup:
	for {
		select {
		case msg := <-p.outputQueue:
			if msg.doneChan != nil {
				msg.doneChan <- struct{}{}
			}
		case <-p.outputInvChan:
			// Just drain channel
		// sendDoneQueue is buffered so doesn't need draining.
		default:
			break cleanup
		}
	}
	p.queueWg.Done()
}

// outHandler handles all outgoing messages for the peer. It must be run as a
// goroutine. It uses a buffered channel to serialize output messages while
// allowing the sender to continue running asynchronously.
func (p *peer) outHandler() {
out:
	for {
		select {
		case msg := <-p.sendQueue:
			// If the message is one we should get a reply for
			// then reset the timer. We specifically do not count inv
			// messages here since they are not sure of a reply if
			// the inv is of no interest explicitly solicited invs
			// should elicit a reply but we don't track them
			// specially.
			switch msg.msg.(type) {
			case *wire.MsgVersion:
				// should get an ack
			case *wire.MsgGetPubKey:
				// should get a pubkey.
			case *wire.MsgGetData:
				// should get objects
			default:
				// Not one of the above, no sure reply.
				// We want to ping if nothing else
				// interesting happens.
			}
			p.writeMessage(msg.msg)
			p.StatsMtx.Lock()
			p.lastSend = time.Now()
			p.StatsMtx.Unlock()
			if msg.doneChan != nil {
				msg.doneChan <- struct{}{}
			}
			p.sendDoneQueue <- struct{}{}
		case <-p.quit:
			break out
		}
	}

	p.queueWg.Wait()

	// Drain any wait channels before we go away so we don't leave something
	// waiting for us. We have waited on queueWg and thus we can be sure
	// that we will not miss anything sent on sendQueue.
cleanup:
	for {
		select {
		case msg := <-p.sendQueue:
			if msg.doneChan != nil {
				msg.doneChan <- struct{}{}
			}
			// no need to send on sendDoneQueue since queueHandler
			// has been waited on and already exited.
		default:
			break cleanup
		}
	}
}

// QueueMessage adds the passed bitmessage message to the peer send queue. It
// uses a buffered channel to communicate with the output handler goroutine so
// it is automatically rate limited and safe for concurrent access.
func (p *peer) QueueMessage(msg wire.Message, doneChan chan struct{}) {
	// Avoid risk of deadlock if goroutine already exited. The goroutine
	// we will be sending to hangs around until it knows for a fact that
	// it is marked as disconnected. *then* it drains the channels.
	if !p.Connected() {
		// avoid deadlock...
		if doneChan != nil {
			go func() {
				doneChan <- struct{}{}
			}()
		}
		return
	}
	p.outputQueue <- outMsg{msg: msg, doneChan: doneChan}
}

// QueueInventory adds the passed inventory to the inventory send queue which
// might not be sent right away, rather it is trickled to the peer in batches.
// Inventory that the peer is already known to have is ignored. It is safe for
// concurrent access.
func (p *peer) QueueInventory(invVect *wire.InvVect) {
	// Don't add the inventory to the send queue if the peer is
	// already known to have it.
	if p.isKnownInventory(invVect) {
		return
	}

	// Avoid risk of deadlock if goroutine already exited. The goroutine
	// we will be sending to hangs around until it knows for a fact that
	// it is marked as disconnected. *then* it drains the channels.
	if !p.Connected() {
		return
	}

	p.outputInvChan <- invVect
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
	close(p.quit)
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

	// Send an initial version message if this is an outbound connection.
	if !p.inbound {
		err := p.pushVersionMsg()
		if err != nil {
			fmt.Printf("err: %v\n", err)
			debug.PrintStack()
			p.logError("Can't send outbound version message %v", err)
			p.Disconnect()
			return err
		}
	}

	// Start processing input and output.
	go p.inHandler()
	// queueWg is kept so that outHandler knows when the queue has exited so
	// it can drain correctly.
	p.queueWg.Add(1)
	go p.queueHandler()
	go p.outHandler()

	return nil
}

// Shutdown gracefully shuts down the peer by disconnecting it.
func (p *peer) Shutdown() {
	p.Disconnect()
}

// newPeerBase returns a new base bitmessage peer for the provided server and
// inbound flag. This is used by the newInboundPeer and newOutboundPeer
// functions to perform base setup needed by both types of peers.
func newPeerBase(s *server, inbound bool, streams []uint32) *peer {
	p := peer{
		server:          s,
		protocolVersion: maxProtocolVersion,
		bmnet:           wire.MainNet,
		services:        wire.SFNodeNetwork,
		inbound:         inbound,
		knownAddresses:  make(map[string]struct{}),
		knownInventory:  NewMruInventoryMap(maxKnownInventory),
		outputQueue:     make(chan outMsg, outputBufferSize),
		sendQueue:       make(chan outMsg, 1),   // nonblocking sync
		sendDoneQueue:   make(chan struct{}, 1), // nonblocking sync
		outputInvChan:   make(chan *wire.InvVect, outputBufferSize),
		quit:            make(chan struct{}),
		streamNumbers:   streams,
	}
	return &p
}

// newInboundPeer returns a new inbound bitmessage peer for the provided server and
// connection. Use Start to begin processing incoming and outgoing messages.
func newInboundPeer(s *server, conn net.Conn, streams []uint32) *peer {
	p := newPeerBase(s, true, streams)
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
	p := newPeerBase(s, false, []uint32{stream})
	p.addr = addr
	p.persistent = persistent
	p.retryCount = retryCount

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
		fmt.Printf("err: %v\n", err)
		debug.PrintStack()
		p.logError("Tried to create a new outbound peer with invalid "+
			"address %s: %v", addr, err)
		return nil
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		fmt.Printf("err: %v\n", err)
		debug.PrintStack()
		p.logError("Tried to create a new outbound peer with invalid "+
			"port %s: %v", portStr, err)
		return nil
	}

	p.na, err = s.addrManager.HostToNetAddress(host, uint16(port), stream, 0)
	if err != nil {
		fmt.Printf("err: %v\n", err)
		debug.PrintStack()
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
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			fmt.Printf("err: %v\n", err)
			debug.PrintStack()
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

// logError makes sure that we only log errors loudly on user peers.
func (p *peer) logError(fmt string, args ...interface{}) {
	if p.persistent {
	} else {
	}
}

// winServiceMain is only invoked on Windows. It detects when bmd is running
// as a service and reacts accordingly.
var winServiceMain func() (bool, error)

// bmdMain is the real main function for bmd. It is necessary to work around
// the fact that deferred functions do not run when os.Exit() is called. The
func bmdMain() error {
	listeners := make([]string, 1)
	listeners[0] = net.JoinHostPort("", "8445")

	// Create server and start it.
	server, err := newServer(listeners)
	if err != nil {
		fmt.Printf("err: %v\n", err)
		debug.PrintStack()
		return err
	}
	server.Start()

	// Monitor for graceful server shutdown and signal the main goroutine
	// when done. This is done in a separate goroutine rather than waiting
	// directly so the main goroutine can be signaled for shutdown by either
	// a graceful shutdown or from the main interrupt handler. This is
	// necessary since the main goroutine must be kept running long enough
	// for the interrupt handler goroutine to finish.
	go func() {
		server.WaitForShutdown()
		shutdownChannel <- struct{}{}
	}()

	// Wait for shutdown signal from either a graceful server stop or from
	// the interrupt handler.
	<-shutdownChannel
	return nil
}

const (
	// supportedServices describes which services are supported by the
	// server.
	supportedServices = wire.SFNodeNetwork

	// connectionRetryInterval is the amount of time to wait in between
	// retries when connecting to persistent peers.
	connectionRetryInterval = time.Second * 10

	// defaultMaxOutbound is the default number of max outbound peers.
	defaultMaxOutbound = 8

	MaxPeers    = 1000
	BanDuration = 1000000
	DefaultPort = 8444
	DataDir     = "/home/jimmy/.bmd"
)

// broadcastMsg provides the ability to house a bitcoin message to be broadcast
// to all connected peers except specified excluded peers.
type broadcastMsg struct {
	message      wire.Message
	excludePeers []*peer
}

// broadcastInventoryAdd is a type used to declare that the InvVect it contains
// needs to be added to the rebroadcast map
type broadcastInventoryAdd relayMsg

// broadcastInventoryDel is a type used to declare that the InvVect it contains
// needs to be removed from the rebroadcast map
type broadcastInventoryDel *wire.InvVect

// relayMsg packages an inventory vector along with the newly discovered
// inventory so the relay has access to that information.
type relayMsg struct {
	invVect *wire.InvVect
	data    interface{}
}

// server provides a bitcoin server for handling communications to and from
// bitcoin peers.
type server struct {
	nonce                uint64
	listeners            []net.Listener
	started              int32      // atomic
	shutdown             int32      // atomic
	shutdownSched        int32      // atomic
	bytesMutex           sync.Mutex // For the following two fields.
	bytesReceived        uint64     // Total bytes received from all peers since start.
	bytesSent            uint64     // Total bytes sent by all peers since start.
	addrManager          *addrmgr.AddrManager
	modifyRebroadcastInv chan interface{}
	newPeers             chan *peer
	donePeers            chan *peer
	banPeers             chan *peer
	wakeup               chan struct{}
	query                chan interface{}
	relayInv             chan relayMsg
	broadcast            chan broadcastMsg
	wg                   sync.WaitGroup
	quit                 chan struct{}
}

type peerState struct {
	peers            *list.List
	outboundPeers    *list.List
	persistentPeers  *list.List
	banned           map[string]time.Time
	outboundGroups   map[string]int
	maxOutboundPeers int
}

// randomUint16Number returns a random uint16 in a specified input range. Note
// that the range is in zeroth ordering; if you pass it 1800, you will get
// values from 0 to 1800.
func randomUint16Number(max uint16) uint16 {
	// In order to avoid modulo bias and ensure every possible outcome in
	// [0, max) has equal probability, the random number must be sampled
	// from a random source that has a range limited to a multiple of the
	// modulus.
	var randomNumber uint16
	var limitRange = (math.MaxUint16 / max) * max
	for {
		binary.Read(rand.Reader, binary.LittleEndian, &randomNumber)
		if randomNumber < limitRange {
			return (randomNumber % max)
		}
	}
}

// AddRebroadcastInventory adds 'iv' to the list of inventories to be
// rebroadcasted at random intervals until they show up in a block.
func (s *server) AddRebroadcastInventory(iv *wire.InvVect, data interface{}) {
	// Ignore if shutting down.
	if atomic.LoadInt32(&s.shutdown) != 0 {
		return
	}

	s.modifyRebroadcastInv <- broadcastInventoryAdd{invVect: iv, data: data}
}

// RemoveRebroadcastInventory removes 'iv' from the list of items to be
// rebroadcasted if present.
func (s *server) RemoveRebroadcastInventory(iv *wire.InvVect) {
	// Ignore if shutting down.
	if atomic.LoadInt32(&s.shutdown) != 0 {
		return
	}

	s.modifyRebroadcastInv <- broadcastInventoryDel(iv)
}

func (p *peerState) Count() int {
	return p.peers.Len() + p.outboundPeers.Len() + p.persistentPeers.Len()
}

func (p *peerState) OutboundCount() int {
	return p.outboundPeers.Len() + p.persistentPeers.Len()
}

func (p *peerState) NeedMoreOutbound() bool {
	return p.OutboundCount() < p.maxOutboundPeers &&
		p.Count() < MaxPeers
}

// forAllOutboundPeers is a helper function that runs closure on all outbound
// peers known to peerState.
func (p *peerState) forAllOutboundPeers(closure func(p *peer)) {
	for e := p.outboundPeers.Front(); e != nil; e = e.Next() {
		closure(e.Value.(*peer))
	}
	for e := p.persistentPeers.Front(); e != nil; e = e.Next() {
		closure(e.Value.(*peer))
	}
}

// forAllPeers is a helper function that runs closure on all peers known to
// peerState.
func (p *peerState) forAllPeers(closure func(p *peer)) {
	for e := p.peers.Front(); e != nil; e = e.Next() {
		closure(e.Value.(*peer))
	}
	p.forAllOutboundPeers(closure)
}

// handleAddPeerMsg deals with adding new peers. It is invoked from the
// peerHandler goroutine.
func (s *server) handleAddPeerMsg(state *peerState, p *peer) bool {
	if p == nil {
		return false
	}

	// Ignore new peers if we're shutting down.
	if atomic.LoadInt32(&s.shutdown) != 0 {
		p.Shutdown()
		return false
	}

	// Disconnect banned peers.
	host, _, err := net.SplitHostPort(p.addr)
	if err != nil {
		fmt.Printf("err: %v\n", err)
		debug.PrintStack()
		p.Shutdown()
		return false
	}
	if banEnd, ok := state.banned[host]; ok {
		if time.Now().Before(banEnd) {
			p.Shutdown()
			return false
		}

		delete(state.banned, host)
	}

	// TODO: Check for max peers from a single IP.

	// Limit max number of total peers.
	if state.Count() >= MaxPeers {
		p.Shutdown()
		// TODO(oga) how to handle permanent peers here?
		// they should be rescheduled.
		return false
	}

	// Add the new peer and start it.
	if p.inbound {
		state.peers.PushBack(p)
		p.Start()
	} else {
		state.outboundGroups[addrmgr.GroupKey(p.na)]++
		if p.persistent {
			state.persistentPeers.PushBack(p)
		} else {
			state.outboundPeers.PushBack(p)
		}
	}

	return true
}

// handleDonePeerMsg deals with peers that have signalled they are done. It is
// invoked from the peerHandler goroutine.
func (s *server) handleDonePeerMsg(state *peerState, p *peer) {
	var list *list.List
	if p.persistent {
		list = state.persistentPeers
	} else if p.inbound {
		list = state.peers
	} else {
		list = state.outboundPeers
	}
	for e := list.Front(); e != nil; e = e.Next() {
		if e.Value == p {
			// Issue an asynchronous reconnect if the peer was a
			// persistent outbound connection.
			if !p.inbound && p.persistent && atomic.LoadInt32(&s.shutdown) == 0 {
				e.Value = newOutboundPeer(s, p.addr, true, p.retryCount+1, p.streamNumbers[0])
				return
			}
			if !p.inbound {
				state.outboundGroups[addrmgr.GroupKey(p.na)]--
			}
			list.Remove(e)
			return
		}
	}
	// If we get here it means that either we didn't know about the peer
	// or we purposefully deleted it.
}

// handleBanPeerMsg deals with banning peers. It is invoked from the
// peerHandler goroutine.
func (s *server) handleBanPeerMsg(state *peerState, p *peer) {
	host, _, err := net.SplitHostPort(p.addr)
	if err != nil {
		fmt.Printf("err: %v\n", err)
		debug.PrintStack()
		return
	}
	state.banned[host] = time.Now().Add(BanDuration)
}

// handleRelayInvMsg deals with relaying inventory to peers that are not already
// known to have it. It is invoked from the peerHandler goroutine.
func (s *server) handleRelayInvMsg(state *peerState, msg relayMsg) {
	state.forAllPeers(func(p *peer) {
		if !p.Connected() {
			return
		}

		// Queue the inventory to be relayed with the next batch.
		// It will be ignored if the peer is already known to
		// have the inventory.
		p.QueueInventory(msg.invVect)
	})
}

// handleBroadcastMsg deals with broadcasting messages to peers. It is invoked
// from the peerHandler goroutine.
func (s *server) handleBroadcastMsg(state *peerState, bmsg *broadcastMsg) {
	state.forAllPeers(func(p *peer) {
		excluded := false
		for _, ep := range bmsg.excludePeers {
			if p == ep {
				excluded = true
			}
		}
		// Don't broadcast to still connecting outbound peers .
		if !p.Connected() {
			excluded = true
		}
		if !excluded {
			p.QueueMessage(bmsg.message, nil)
		}
	})
}

type getConnCountMsg struct {
	reply chan int32
}

type addNodeMsg struct {
	addr      string
	stream    uint32
	permanent bool
	reply     chan error
}

type delNodeMsg struct {
	addr  string
	reply chan error
}

type getAddedNodesMsg struct {
	reply chan []*peer
}

// addPeer adds an ip address to the peer handler and adds permanent connections
// to the set of persistant peers.
// This function exists to add initial peers to the address manager before the
// peerHandler go routine has entered its main loop. By contrast, AddPeer assumes
// that the address manager go routines are already in their main loops.
func (s *server) addPeer(addr string, stream uint32, permanent bool, state *peerState) error {
	// XXX(oga) duplicate oneshots?
	if permanent {
		for e := state.persistentPeers.Front(); e != nil; e = e.Next() {
			peer := e.Value.(*peer)
			if peer.addr == addr {
				return errors.New("peer already connected")
			}
		}
	}
	// TODO(oga) if too many, nuke a non-perm peer.
	if s.handleAddPeerMsg(state, newOutboundPeer(s, addr, permanent, 0, stream)) {
		return nil
	} else {
		return errors.New("failed to add peer")
	}
}

// handleQuery is the central handler for all queries and commands from other
// goroutines related to peer state.
func (s *server) handleQuery(querymsg interface{}, state *peerState) {
	switch msg := querymsg.(type) {
	case getConnCountMsg:
		nconnected := int32(0)
		state.forAllPeers(func(p *peer) {
			if p.Connected() {
				nconnected++
			}
		})
		msg.reply <- nconnected

	case addNodeMsg:
		msg.reply <- s.addPeer(msg.addr, msg.stream, msg.permanent, state)

	case delNodeMsg:
		found := false
		for e := state.persistentPeers.Front(); e != nil; e = e.Next() {
			peer := e.Value.(*peer)
			if peer.addr == msg.addr {
				// Keep group counts ok since we remove from
				// the list now.
				state.outboundGroups[addrmgr.GroupKey(peer.na)]--
				// This is ok because we are not continuing
				// to iterate so won't corrupt the loop.
				state.persistentPeers.Remove(e)
				peer.Disconnect()
				found = true
				break
			}
		}

		if found {
			msg.reply <- nil
		} else {
			msg.reply <- errors.New("peer not found")
		}

	// Request a list of the persistent (added) peers.
	case getAddedNodesMsg:
		// Respond with a slice of the relavent peers.
		peers := make([]*peer, 0, state.persistentPeers.Len())
		for e := state.persistentPeers.Front(); e != nil; e = e.Next() {
			peer := e.Value.(*peer)
			peers = append(peers, peer)
		}
		msg.reply <- peers
	}
}

// listenHandler is the main listener which accepts incoming connections for the
// server. It must be run as a goroutine.
func (s *server) listenHandler(listener net.Listener) {
	for atomic.LoadInt32(&s.shutdown) == 0 {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("err: %v\n", err)
			debug.PrintStack()
			continue
		}
		s.AddPeer(newInboundPeer(s, conn, defaultStreamList))
	}
	s.wg.Done()
}

// // seedFromDNS uses DNS seeding to populate the address manager with peers.
// func (s *server) seedFromDNS() {
// 	addresses := make([]*wire.NetAddress, 10)
// 	i := 0
// 	ip := net.ParseIP("23.239.9.147")
// 	addresses[i] = new(wire.NetAddress)
// 	addresses[i].SetAddress(ip, 8444)
// 	i++
// 	ip = net.ParseIP("98.218.125.214")
// 	addresses[i] = new(wire.NetAddress)
// 	addresses[i].SetAddress(ip, 8444)
// 	i++
// 	ip = net.ParseIP("192.121.170.162")
// 	addresses[i] = new(wire.NetAddress)
// 	addresses[i].SetAddress(ip, 8444)
// 	i++
// 	ip = net.ParseIP("108.61.72.12")
// 	addresses[i] = new(wire.NetAddress)
// 	addresses[i].SetAddress(ip, 28444)
// 	i++
// 	ip = net.ParseIP("158.222.211.81")
// 	addresses[i] = new(wire.NetAddress)
// 	addresses[i].SetAddress(ip, 8080)
// 	i++
// 	ip = net.ParseIP("79.163.240.110")
// 	addresses[i] = new(wire.NetAddress)
// 	addresses[i].SetAddress(ip, 8446)
// 	i++
// 	ip = net.ParseIP("178.62.154.250")
// 	addresses[i] = new(wire.NetAddress)
// 	addresses[i].SetAddress(ip, 8444)
// 	i++
// 	ip = net.ParseIP("178.62.155.6")
// 	addresses[i] = new(wire.NetAddress)
// 	addresses[i].SetAddress(ip, 8444)
// 	i++
// 	ip = net.ParseIP("178.62.155.8")
// 	addresses[i] = new(wire.NetAddress)
// 	addresses[i].SetAddress(ip, 8444)
// 	i++
// 	ip = net.ParseIP("68.42.42.120")
// 	addresses[i] = new(wire.NetAddress)
// 	addresses[i].SetAddress(ip, 8444)
// 	s.addrManager.AddAddresses(addresses, addresses[0])
// }

// peerHandler is used to handle peer operations such as adding and removing
// peers to and from the server, banning peers, and broadcasting messages to
// peers. It must be run in a goroutine.
func (s *server) peerHandler() {
	// Start the address manager, needed by peers.
	// This is done here since their lifecycle is closely tied
	// to this handler and rather than adding more channels to sychronize
	// things, it's easier and slightly faster to simply start and stop
	// them in this handler.
	s.addrManager.Start()

	state := &peerState{
		peers:            list.New(),
		persistentPeers:  list.New(),
		outboundPeers:    list.New(),
		banned:           make(map[string]time.Time),
		maxOutboundPeers: defaultMaxOutbound,
		outboundGroups:   make(map[string]int),
	}
	if MaxPeers < state.maxOutboundPeers {
		state.maxOutboundPeers = MaxPeers
	}

	// Add peers discovered through DNS to the address manager.
	// s.seedFromDNS()

	// The list of default peers used by PyBitmessage.
	s.addPeer("5.45.99.75:8444", 1, true, state)
	s.addPeer("75.167.159.54:8444", 1, true, state)
	s.addPeer("95.165.168.168:8444", 1, true, state)
	s.addPeer("85.180.139.241:8444", 1, true, state)
	s.addPeer("158.222.211.81:8080", 1, true, state)
	s.addPeer("178.62.12.187:8448", 1, true, state)
	s.addPeer("24.188.198.204:8111", 1, true, state)
	s.addPeer("109.147.204.113:1195", 1, true, state)
	s.addPeer("178.11.46.221:8444", 1, true, state)

	// The previous list of default peers.
	/*	s.addPeer("23.239.9.147:8444", 1, true, state)
		s.addPeer("98.218.125.214:8444", 1, true, state)
		s.addPeer("192.121.170.162:8444", 1, true, state)
		s.addPeer("108.61.72.12:28444", 1, true, state)
		s.addPeer("158.222.211.81:8080", 1, true, state)
		s.addPeer("79.163.240.110:8446", 1, true, state)
		s.addPeer("178.62.154.250:8444", 1, true, state)
		s.addPeer("178.62.155.6:8444", 1, true, state)
		s.addPeer("178.62.155.8:8444", 1, true, state)
		s.addPeer("68.42.42.120:8444", 1, true, state)*/

	// if nothing else happens, wake us up soon.
	time.AfterFunc(10*time.Second, func() { s.wakeup <- struct{}{} })

out:
	for {
		select {
		// New peers connected to the server.
		case p := <-s.newPeers:
			s.handleAddPeerMsg(state, p)

		// Disconnected peers.
		case p := <-s.donePeers:
			s.handleDonePeerMsg(state, p)

		// Peer to ban.
		case p := <-s.banPeers:
			s.handleBanPeerMsg(state, p)

		// New inventory to potentially be relayed to other peers.
		case invMsg := <-s.relayInv:
			s.handleRelayInvMsg(state, invMsg)

		// Message to broadcast to all connected peers except those
		// which are excluded by the message.
		case bmsg := <-s.broadcast:
			s.handleBroadcastMsg(state, &bmsg)

		// Used by timers below to wake us back up.
		case <-s.wakeup:
			// this page left intentionally blank

		case qmsg := <-s.query:
			s.handleQuery(qmsg, state)

		// Shutdown the peer handler.
		case <-s.quit:
			// Shutdown peers.
			state.forAllPeers(func(p *peer) {
				p.Shutdown()
			})
			break out
		}

		// Only try connect to more peers if we actually need more.
		if !state.NeedMoreOutbound() ||
			atomic.LoadInt32(&s.shutdown) != 0 {
			continue
		}
		tries := 0
		for state.NeedMoreOutbound() &&
			atomic.LoadInt32(&s.shutdown) == 0 {
			// We bias like bitcoind does, 10 for no outgoing
			// up to 90 (8) for the selection of new vs tried
			//addresses.

			nPeers := state.OutboundCount()
			if nPeers > 8 {
				nPeers = 8
			}
			addr := s.addrManager.GetAddress("any", 10+nPeers*10)
			if addr == nil {
				break
			}
			key := addrmgr.GroupKey(addr.NetAddress())
			// Address will not be invalid, local or unroutable
			// because addrmanager rejects those on addition.
			// Just check that we don't already have an address
			// in the same group so that we are not connecting
			// to the same network segment at the expense of
			// others.
			if state.outboundGroups[key] != 0 {
				break
			}

			tries++
			// After 100 bad tries exit the loop and we'll try again
			// later.
			if tries > 100 {
				break
			}

			// XXX if we have limited that address skip

			// only allow recent nodes (10mins) after we failed 30
			// times
			if time.Now().After(addr.LastAttempt().Add(10*time.Minute)) &&
				tries < 30 {
				continue
			}

			addrStr := addrmgr.NetAddressKey(addr.NetAddress())

			tries = 0
			// any failure will be due to banned peers etc. we have
			// already checked that we have room for more peers.
			if s.handleAddPeerMsg(state,
				// Stream number 1 is hard-coded in here. Will have to handle
				// this more gracefully when we support streams.
				newOutboundPeer(s, addrStr, false, 0, 1)) {
			}
		}

		// We need more peers, wake up in ten seconds and try again.
		if state.NeedMoreOutbound() {
			time.AfterFunc(10*time.Second, func() {
				s.wakeup <- struct{}{}
			})
		}
	}

	s.addrManager.Stop()
	s.wg.Done()
}

// AddPeer adds a new peer that has already been connected to the server.
func (s *server) AddPeer(p *peer) {
	s.newPeers <- p
}

// BanPeer bans a peer that has already been connected to the server by ip.
func (s *server) BanPeer(p *peer) {
	s.banPeers <- p
}

// RelayInventory relays the passed inventory to all connected peers that are
// not already known to have it.
func (s *server) RelayInventory(invVect *wire.InvVect, data interface{}) {
	s.relayInv <- relayMsg{invVect: invVect, data: data}
}

// BroadcastMessage sends msg to all peers currently connected to the server
// except those in the passed peers to exclude.
func (s *server) BroadcastMessage(msg wire.Message, exclPeers ...*peer) {
	// XXX: Need to determine if this is an alert that has already been
	// broadcast and refrain from broadcasting again.
	bmsg := broadcastMsg{message: msg, excludePeers: exclPeers}
	s.broadcast <- bmsg
}

// ConnectedCount returns the number of currently connected peers.
func (s *server) ConnectedCount() int32 {
	replyChan := make(chan int32)

	s.query <- getConnCountMsg{reply: replyChan}

	return <-replyChan
}

// AddedNodeInfo returns an array of structures
// describing the persistent (added) nodes.
func (s *server) AddedNodeInfo() []*peer {
	replyChan := make(chan []*peer)
	s.query <- getAddedNodesMsg{reply: replyChan}
	return <-replyChan
}

// AddAddr adds `addr' as a new outbound peer. If permanent is true then the
// peer will be persistent and reconnect if the connection is lost.
// It is an error to call this with an already existing peer.
func (s *server) AddAddr(addr string, stream uint32, permanent bool) error {
	replyChan := make(chan error)

	s.query <- addNodeMsg{addr: addr, stream: stream, permanent: permanent, reply: replyChan}

	return <-replyChan
}

// RemoveAddr removes `addr' from the list of persistent peers if present.
// An error will be returned if the peer was not found.
func (s *server) RemoveAddr(addr string) error {
	replyChan := make(chan error)

	s.query <- delNodeMsg{addr: addr, reply: replyChan}

	return <-replyChan
}

// AddBytesSent adds the passed number of bytes to the total bytes sent counter
// for the server. It is safe for concurrent access.
func (s *server) AddBytesSent(bytesSent uint64) {
	s.bytesMutex.Lock()
	defer s.bytesMutex.Unlock()

	s.bytesSent += bytesSent
}

// AddBytesReceived adds the passed number of bytes to the total bytes received
// counter for the server. It is safe for concurrent access.
func (s *server) AddBytesReceived(bytesReceived uint64) {
	s.bytesMutex.Lock()
	defer s.bytesMutex.Unlock()

	s.bytesReceived += bytesReceived
}

// NetTotals returns the sum of all bytes received and sent across the network
// for all peers. It is safe for concurrent access.
func (s *server) NetTotals() (uint64, uint64) {
	s.bytesMutex.Lock()
	defer s.bytesMutex.Unlock()

	return s.bytesReceived, s.bytesSent
}

// rebroadcastHandler keeps track of user submitted inventories that we have
// sent out but have not yet made it into a block. We periodically rebroadcast
// them in case our peers restarted or otherwise lost track of them.
func (s *server) rebroadcastHandler() {
	// Wait 5 min before first tx rebroadcast.
	timer := time.NewTimer(5 * time.Minute)
	pendingInvs := make(map[wire.InvVect]interface{})

out:
	for {
		select {
		case riv := <-s.modifyRebroadcastInv:
			switch msg := riv.(type) {
			// Incoming InvVects are added to our map of RPC txs.
			case broadcastInventoryAdd:
				pendingInvs[*msg.invVect] = msg.data

			// When an InvVect has been added to a block, we can
			// now remove it, if it was present.
			case broadcastInventoryDel:
				if _, ok := pendingInvs[*msg]; ok {
					delete(pendingInvs, *msg)
				}
			}

		case <-timer.C:
			// Any inventory we have has not made it into a block
			// yet. We periodically resubmit them until they have.
			for iv, data := range pendingInvs {
				ivCopy := iv
				s.RelayInventory(&ivCopy, data)
			}

			// Process at a random time up to 30mins (in seconds)
			// in the future.
			timer.Reset(time.Second *
				time.Duration(randomUint16Number(1800)))

		case <-s.quit:
			break out
		}
	}

	timer.Stop()

	// Drain channels before exiting so nothing is left waiting around
	// to send.
cleanup:
	for {
		select {
		case <-s.modifyRebroadcastInv:
		default:
			break cleanup
		}
	}
	s.wg.Done()
}

// Start begins accepting connections from peers.
func (s *server) Start() {
	// Already started?
	if atomic.AddInt32(&s.started, 1) != 1 {
		return
	}

	// Start all the listeners. There will not be any if listening is
	// disabled.
	for _, listener := range s.listeners {
		s.wg.Add(1)
		go s.listenHandler(listener)
	}

	// Start the peer handler which in turn starts the address manager.
	s.wg.Add(1)
	go s.peerHandler()
}

// Stop gracefully shuts down the server by stopping and disconnecting all
// peers and the main listener.
func (s *server) Stop() error {
	// Make sure this only happens once.
	if atomic.AddInt32(&s.shutdown, 1) != 1 {
		return nil
	}

	// Stop all the listeners. There will not be any listeners if
	// listening is disabled.
	for _, listener := range s.listeners {
		err := listener.Close()
		if err != nil {
			fmt.Printf("err: %v\n", err)
			debug.PrintStack()
			return err
		}
	}

	// Signal the remaining goroutines to quit.
	close(s.quit)
	return nil
}

// WaitForShutdown blocks until the main listener and peer handlers are stopped.
func (s *server) WaitForShutdown() {
	s.wg.Wait()
}

// ScheduleShutdown schedules a server shutdown after the specified duration.
// It also dynamically adjusts how often to warn the server is going down based
// on remaining duration.
func (s *server) ScheduleShutdown(duration time.Duration) {
	// Don't schedule shutdown more than once.
	if atomic.AddInt32(&s.shutdownSched, 1) != 1 {
		return
	}
	go func() {
		remaining := duration
		tickDuration := dynamicTickDuration(remaining)
		done := time.After(remaining)
		ticker := time.NewTicker(tickDuration)
	out:
		for {
			select {
			case <-done:
				ticker.Stop()
				s.Stop()
				break out
			case <-ticker.C:
				remaining = remaining - tickDuration
				if remaining < time.Second {
					continue
				}

				// Change tick duration dynamically based on remaining time.
				newDuration := dynamicTickDuration(remaining)
				if tickDuration != newDuration {
					tickDuration = newDuration
					ticker.Stop()
					ticker = time.NewTicker(tickDuration)
				}
			}
		}
	}()
}

// parseListeners splits the list of listen addresses passed in addrs into
// IPv4 and IPv6 slices and returns them. This allows easy creation of the
// listeners on the correct interface "tcp4" and "tcp6". It also properly
// detects addresses which apply to "all interfaces" and adds the address to
// both slices.
func parseListeners(addrs []string) ([]string, []string, bool, error) {
	ipv4ListenAddrs := make([]string, 0, len(addrs)*2)
	ipv6ListenAddrs := make([]string, 0, len(addrs)*2)
	haveWildcard := false

	for _, addr := range addrs {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			fmt.Printf("err: %v\n", err)
			debug.PrintStack()
			// Shouldn't happen due to already being normalized.
			return nil, nil, false, err
		}

		// Empty host or host of * on plan9 is both IPv4 and IPv6.
		if host == "" || (host == "*" && runtime.GOOS == "plan9") {
			ipv4ListenAddrs = append(ipv4ListenAddrs, addr)
			ipv6ListenAddrs = append(ipv6ListenAddrs, addr)
			haveWildcard = true
			continue
		}

		// Parse the IP.
		ip := net.ParseIP(host)
		if ip == nil {
			return nil, nil, false, fmt.Errorf("'%s' is not a "+
				"valid IP address", host)
		}

		// To4 returns nil when the IP is not an IPv4 address, so use
		// this determine the address type.
		if ip.To4() == nil {
			ipv6ListenAddrs = append(ipv6ListenAddrs, addr)
		} else {
			ipv4ListenAddrs = append(ipv4ListenAddrs, addr)
		}
	}
	return ipv4ListenAddrs, ipv6ListenAddrs, haveWildcard, nil
}

// newServer returns a new bmd server configured to listen on addr for the
// bitmessage network. Use start to begin accepting connections from peers.
func newServer(listenAddrs []string) (*server, error) {
	nonce, err := wire.RandomUint64()
	if err != nil {
		fmt.Printf("err: %v\n", err)
		debug.PrintStack()
		return nil, err
	}

	amgr := addrmgr.New(DataDir, net.LookupIP)

	var listeners []net.Listener
	ipv4Addrs, ipv6Addrs, wildcard, err := parseListeners(listenAddrs)
	if err != nil {
		fmt.Printf("err: %v\n", err)
		debug.PrintStack()
		return nil, err
	}
	listeners = make([]net.Listener, 0, len(ipv4Addrs)+len(ipv6Addrs))
	discover := true

	// TODO(oga) nonstandard port...
	if wildcard {
		port := DefaultPort
		addrs, _ := net.InterfaceAddrs()
		for _, a := range addrs {
			ip, _, err := net.ParseCIDR(a.String())
			if err != nil {
				fmt.Printf("err: %v\n", err)
				debug.PrintStack()
				continue
			}
			// Stream number 1 is hard-coded in here. When we support streams,
			// this will need to be handled properly.
			na := wire.NewNetAddressIPPort(ip,
				uint16(port), 1, wire.SFNodeNetwork)
			if discover {
				err = amgr.AddLocalAddress(na, addrmgr.InterfacePrio)
			}
		}
	}

	for _, addr := range ipv4Addrs {
		listener, err := net.Listen("tcp4", addr)
		if err != nil {
			fmt.Printf("err: %v\n", err)
			debug.PrintStack()
			continue
		}
		listeners = append(listeners, listener)

		if discover {
			if na, err := amgr.DeserializeNetAddress(addr); err == nil {
				err = amgr.AddLocalAddress(na, addrmgr.BoundPrio)
				if err != nil {
					fmt.Printf("err: %v\n", err)
					debug.PrintStack()
				}
			}
		}
	}

	for _, addr := range ipv6Addrs {
		listener, err := net.Listen("tcp6", addr)
		if err != nil {
			fmt.Printf("err: %v\n", err)
			debug.PrintStack()
			continue
		}
		listeners = append(listeners, listener)
		if discover {
			if na, err := amgr.DeserializeNetAddress(addr); err == nil {
				err = amgr.AddLocalAddress(na, addrmgr.BoundPrio)
			}
		}
	}

	if len(listeners) == 0 {
		return nil, errors.New("no valid listen address")
	}

	s := server{
		nonce:                nonce,
		listeners:            listeners,
		addrManager:          amgr,
		newPeers:             make(chan *peer, MaxPeers),
		donePeers:            make(chan *peer, MaxPeers),
		banPeers:             make(chan *peer, MaxPeers),
		wakeup:               make(chan struct{}),
		query:                make(chan interface{}),
		relayInv:             make(chan relayMsg, MaxPeers),
		broadcast:            make(chan broadcastMsg, MaxPeers),
		quit:                 make(chan struct{}),
		modifyRebroadcastInv: make(chan interface{}),
	}

	return &s, nil
}

// dynamicTickDuration is a convenience function used to dynamically choose a
// tick duration based on remaining time. It is primarily used during
// server shutdown to make shutdown warnings more frequent as the shutdown time
// approaches.
func dynamicTickDuration(remaining time.Duration) time.Duration {
	switch {
	case remaining <= time.Second*5:
		return time.Second
	case remaining <= time.Second*15:
		return time.Second * 5
	case remaining <= time.Minute:
		return time.Second * 15
	case remaining <= time.Minute*5:
		return time.Minute
	case remaining <= time.Minute*15:
		return time.Minute * 5
	case remaining <= time.Hour:
		return time.Minute * 15
	}
	return time.Hour
}

// MruInventoryMap provides a map that is limited to a maximum number of items
// with eviction for the oldest entry when the limit is exceeded.
type MruInventoryMap struct {
	invMap  map[wire.InvVect]*list.Element // nearly O(1) lookups
	invList *list.List                     // O(1) insert, update, delete
	limit   uint
}

// String returns the map as a human-readable string.
func (m MruInventoryMap) String() string {
	return fmt.Sprintf("<%d>%v", m.limit, m.invMap)
}

// Exists returns whether or not the passed inventory item is in the map.
func (m *MruInventoryMap) Exists(iv *wire.InvVect) bool {
	if _, exists := m.invMap[*iv]; exists {
		return true
	}
	return false
}

// Add adds the passed inventory to the map and handles eviction of the oldest
// item if adding the new item would exceed the max limit.
func (m *MruInventoryMap) Add(iv *wire.InvVect) {
	// When the limit is zero, nothing can be added to the map, so just
	// return.
	if m.limit == 0 {
		return
	}

	// When the entry already exists move it to the front of the list
	// thereby marking it most recently used.
	if node, exists := m.invMap[*iv]; exists {
		m.invList.MoveToFront(node)
		return
	}

	// Evict the least recently used entry (back of the list) if the the new
	// entry would exceed the size limit for the map. Also reuse the list
	// node so a new one doesn't have to be allocated.
	if uint(len(m.invMap))+1 > m.limit {
		node := m.invList.Back()
		lru, ok := node.Value.(*wire.InvVect)
		if !ok {
			return
		}

		// Evict least recently used item.
		delete(m.invMap, *lru)

		// Reuse the list node of the item that was just evicted for the
		// new item.
		node.Value = iv
		m.invList.MoveToFront(node)
		m.invMap[*iv] = node
		return
	}

	// The limit hasn't been reached yet, so just add the new item.
	node := m.invList.PushFront(iv)
	m.invMap[*iv] = node
	return
}

// Delete deletes the passed inventory item from the map (if it exists).
func (m *MruInventoryMap) Delete(iv *wire.InvVect) {
	if node, exists := m.invMap[*iv]; exists {
		m.invList.Remove(node)
		delete(m.invMap, *iv)
	}
}

// NewMruInventoryMap returns a new inventory map that is limited to the number
// of entries specified by limit. When the number of entries exceeds the limit,
// the oldest (least recently used) entry will be removed to make room for the
// new entry.
func NewMruInventoryMap(limit uint) *MruInventoryMap {
	m := MruInventoryMap{
		invMap:  make(map[wire.InvVect]*list.Element),
		invList: list.New(),
		limit:   limit,
	}
	return &m
}

func main() {
	// Use all processor cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Call serviceMain on Windows to handle running as a service. When
	// the return isService flag is true, exit now since we ran as a
	// service. Otherwise, just fall through to normal operation.
	if runtime.GOOS == "windows" {
		isService, err := winServiceMain()
		if err != nil {
			fmt.Printf("err: %v\n", err)
			debug.PrintStack()
			fmt.Println(err)
			os.Exit(1)
		}
		if isService {
			os.Exit(0)
		}
	}

	// Work around defer not working after os.Exit()
	if err := bmdMain(); err != nil {
		fmt.Printf("err %v\n", err)
		os.Exit(1)
	}
}
