package bmpeer

import (
	"sync/atomic"
	"time"

	"github.com/monetas/bmd/database"
	"github.com/monetas/bmutil/wire"
)

const (
	// maxProtocolVersion is the max protocol version the peer supports.
	//maxProtocolVersion = 3

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
	//pingTimeoutMinutes = 2
)

// Peer is the part of a bitmessage peer that handles the incoming connection
// and manages all other components. This is not yet completed and has not
// been incorporated into the peer that is in use right now.
type Peer struct {
	logic     Logic
	sendQueue SendQueue
	conn      Connection

	started    int32
	connected  int32
	disconnect int32 // only to be used atomically

	quit chan struct{}
}

// Connected returns whether or not the peer is currently connected.
func (p *Peer) Connected() bool {
	return atomic.LoadInt32(&p.connected) != 0 &&
		atomic.LoadInt32(&p.disconnect) == 0
}

// Disconnect disconnects the peer by closing the connection. It also sets
// a flag so the impending shutdown can be detected.
func (p *Peer) Disconnect() {
	// did we win the race?
	if atomic.AddInt32(&p.disconnect, 1) != 1 {
		return
	}

	p.sendQueue.Stop()
	close(p.quit)

	if atomic.LoadInt32(&p.connected) != 0 {
		p.conn.Close()
	}
}

// Start begins processing input and output messages. It also sends the initial
// version message for outbound connections to start the negotiation process.
func (p *Peer) Start() error {
	// Already started?
	if atomic.AddInt32(&p.started, 1) != 1 {
		return nil
	}

	p.sendQueue.Start(p.conn)

	// Send an initial version message if this is an outbound connection.
	// TODO
	/*if !p.inbound {
		p.PushVersionMsg()
	}*/

	// Start processing input and output.
	go p.inHandler()

	return nil
}

// inHandler handles all incoming messages for the peer. It must be run as a
// goroutine.
func (p *Peer) inHandler() {
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
		err = nil
		switch msg := rmsg.(type) {
		case *wire.MsgVersion:
			err = p.logic.HandleVersionMsg(msg)
			markConnected = true

		case *wire.MsgVerAck:
			err = p.logic.HandleVerAckMsg()
			markConnected = true

		case *wire.MsgAddr:
			err = p.logic.HandleAddrMsg(msg)
			markConnected = true

		case *wire.MsgInv:
			err = p.logic.HandleInvMsg(msg)
			markConnected = true

		case *wire.MsgGetData:
			err = p.sendQueue.QueueDataRequest((rmsg.(*wire.MsgGetData)).InvList)
			markConnected = true

		case *wire.MsgGetPubKey, *wire.MsgPubKey, *wire.MsgMsg, *wire.MsgBroadcast, *wire.MsgUnknownObject:
			err = p.logic.HandleObjectMsg(rmsg)
			markConnected = true
		}

		if err != nil {
			break out
		}

		// reset the timer.
		idleTimer.Reset(idleTimeoutMinutes * time.Minute)

		// TODO
		// Mark the address as currently connected and working as of
		// now if one of the messages that trigger it was processed.
		//		if markConnected && atomic.LoadInt32(&p.disconnect) == 0 {
		//			if p.na == nil {
		//				continue
		//			}
		//			p.server.addrManager.Connected(p.na)
		//		}
		// ok we got a message, reset the timer.
		// timer just calls p.Disconnect() after logging.
		idleTimer.Reset(idleTimeoutMinutes * time.Minute)
	}

	idleTimer.Stop()

	// Ensure connection is closed and notify the server that the peer is
	// done.
	p.Disconnect()

	//TODO
	// Only tell block manager we are gone if we ever told it we existed.
	//if p.HandshakeComplete() {
	//	p.server.objectManager.DonePeer(p)
	//}

	//p.server.donePeers <- p
}

// NewPeer returns a new Peer object.
func NewPeer(logic Logic, conn Connection, db database.Db) *Peer {
	return &Peer{
		logic:     logic,
		sendQueue: NewSendQueue(NewInventory(db)),
		conn:      conn,
	}
}
