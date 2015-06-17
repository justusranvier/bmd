// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/DanielKrawisz/maxrate"
	"github.com/monetas/bmutil/wire"
)

// Connection is a bitmessage connection that abstracts the underlying tcp
// connection away. The user of the Connection only uses bitmessage
// wire.Message objects instead of the underlying byte stream.
// This is written as an interface so that it can easily be swapped out for a
// mock object for testing purposes.
type Connection interface {
	WriteMessage(wire.Message) error
	ReadMessage() (wire.Message, error)
	BytesWritten() uint64
	BytesRead() uint64
	LastWrite() time.Time
	LastRead() time.Time
	RemoteAddr() net.Addr
	Connected() bool
	Connect() error
	Close()
}

// connection implements the Connection interface and connects to a
// real outside bitmessage node over the internet.
type connection struct {
	conn          net.Conn
	connMtx       sync.RWMutex
	addr          net.Addr
	sentMtx       sync.RWMutex
	bytesSent     uint64
	receivedMtx   sync.RWMutex
	bytesReceived uint64
	lastRead      time.Time
	lastWrite     time.Time
	timeConnected time.Time
	idleTimeout   time.Duration
	idleTimer     *time.Timer
	maxUp         *maxrate.MaxRate
	maxDown       *maxrate.MaxRate
}

// WriteMessage sends a bitmessage p2p message along the tcp connection.
func (pc *connection) WriteMessage(msg wire.Message) error {
	// conn will be nill if the connection disconnected.
	if !pc.Connected() {
		return errors.New("No connection established.")
	}

	// Write the message to the peer.
	pc.connMtx.RLock()
	conn := pc.conn
	pc.connMtx.RUnlock()
	n, err := wire.WriteMessageN(conn, msg, wire.MainNet)

	pc.sentMtx.Lock()
	pc.bytesSent += uint64(n)
	pc.lastWrite = time.Now()
	pc.maxUp.Transfer(float64(n))
	pc.sentMtx.Unlock()

	if err != nil {
		if !pc.Connected() { // Connection might have been closed while reading.
			return nil
		}
		pc.Close()
		return err
	}

	return nil
}

// ReadMessage reads a bitmessage p2p message from the tcp connection.
func (pc *connection) ReadMessage() (wire.Message, error) {
	// conn will be nill if the connection disconnected.
	if !pc.Connected() {
		return nil, nil
	}

	pc.connMtx.RLock()
	conn := pc.conn
	pc.connMtx.RUnlock()
	n, msg, _, err := wire.ReadMessageN(conn, wire.MainNet)

	pc.receivedMtx.Lock()
	pc.bytesReceived += uint64(n)
	pc.lastRead = time.Now()
	pc.maxDown.Transfer(float64(n))
	pc.receivedMtx.Unlock()

	if err != nil {
		if !pc.Connected() { // Connection might have been closed while reading.
			return nil, nil
		}
		pc.Close()
		return nil, err
	}

	pc.idleTimer.Reset(pc.idleTimeout)

	return msg, nil
}

// BytesWritten returns the total number of bytes written to this connection.
func (pc *connection) BytesWritten() uint64 {
	pc.sentMtx.Lock()
	defer pc.sentMtx.Unlock()
	return pc.bytesSent
}

// BytesRead returns the total number of bytes read by this connection.
func (pc *connection) BytesRead() uint64 {
	pc.receivedMtx.Lock()
	defer pc.receivedMtx.Unlock()
	return pc.bytesReceived
}

// LastWrite returns the last time that a message was written.
func (pc *connection) LastWrite() time.Time {
	t := pc.lastWrite
	return t
}

// LastRead returns the last time that a message was read.
func (pc *connection) LastRead() time.Time {
	t := pc.lastRead
	return t
}

// RemoteAddr returns the address of the remote peer.
func (pc *connection) RemoteAddr() net.Addr {
	return pc.addr
}

// Close disconnects the peer and stops running the connection.
func (pc *connection) Close() {
	if pc.Connected() {
		pc.connMtx.Lock()
		conn := pc.conn
		pc.conn = nil
		pc.connMtx.Unlock()
		conn.Close()
	}

	pc.idleTimer.Stop()
}

// Connected returns whether the connection is connected to a remote peer.
func (pc *connection) Connected() bool {
	pc.connMtx.RLock()
	defer pc.connMtx.RUnlock()
	return pc.conn != nil
}

var dial = net.Dial

// Connect starts running the connection and connects to the remote peer.
func (pc *connection) Connect() error {
	if pc.Connected() {
		return errors.New("already connected")
	}

	conn, err := dial("tcp", pc.addr.String())
	if err != nil {
		return err
	}

	pc.idleTimer.Reset(pc.idleTimeout)

	pc.timeConnected = time.Now()
	pc.connMtx.Lock()
	pc.conn = conn
	pc.connMtx.Unlock()
	return nil
}

// SetDialer sets the dialer used by peer to connect to peers.
func SetDialer(dialer func(string, string) (net.Conn, error)) {
	dial = dialer
}

// NewConnection creates a new *connection.
func NewConnection(addr net.Addr, maxDown, maxUp int64) Connection {
	idleTimeout := time.Minute * pingTimeoutMinutes

	pc := &connection{
		addr:        addr,
		idleTimeout: idleTimeout,
		maxDown:     maxrate.New(float64(maxDown), 1),
		maxUp:       maxrate.New(float64(maxUp), 1),
	}

	pc.idleTimer = time.AfterFunc(pc.idleTimeout, func() {
		pc.WriteMessage(&wire.MsgPong{})
		pc.idleTimer.Reset(idleTimeout)
	})

	pc.idleTimer.Stop()

	return pc
}
