// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bmpeer

import (
	"net"
	"sync"
	"time"
	"errors"

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
	//LocalAddr() net.Addr
	Connected() bool
	Connect() error
	Close()
}

// TODO handle timeout and ping/pong issues.
// connection implements the Connection interface and connects to a
// real outside bitmessage node over the internet.
type connection struct {
	conn          net.Conn
	addr          net.Addr
	mtx           sync.Mutex
	bytesSent     uint64
	bytesReceived uint64
	lastRead      time.Time
	lastWrite     time.Time
	timeConnected time.Time
}

func (pc *connection) WriteMessage(msg wire.Message) error {
	// conn will be nill if the connection disconnected. 
	if pc.conn == nil {
		return errors.New("No connection established.")
	}

	// Write the message to the peer.
	n, err := wire.WriteMessageN(pc.conn, msg, wire.MainNet)

	pc.mtx.Lock()
	pc.bytesSent += uint64(n)
	pc.lastWrite = time.Now()
	pc.mtx.Unlock()
	
	if err != nil {
		pc.conn = nil
		return err
	}

	return nil
}

func (pc *connection) ReadMessage() (wire.Message, error) {
	// conn will be nill if the connection disconnected. 
	if pc.conn == nil {
		return nil, errors.New("No connection established.")
	}
	
	n, msg, _, err := wire.ReadMessageN(pc.conn, wire.MainNet)

	pc.mtx.Lock()
	pc.bytesReceived += uint64(n)
	pc.lastRead = time.Now()
	pc.mtx.Unlock()

	if err != nil {
		pc.conn = nil
		return nil, err
	}

	return msg, nil
}

func (pc *connection) BytesWritten() uint64 {
	pc.mtx.Lock()
	n := pc.bytesSent
	pc.mtx.Unlock()
	return n
}

func (pc *connection) BytesRead() uint64 {
	pc.mtx.Lock()
	n := pc.bytesReceived
	pc.mtx.Unlock()
	return n
}

func (pc *connection) LastWrite() time.Time {
	pc.mtx.Lock()
	t := pc.lastWrite
	pc.mtx.Unlock()
	return t
}

func (pc *connection) LastRead() time.Time {
	pc.mtx.Lock()
	t := pc.lastRead
	pc.mtx.Unlock()
	return t
}

func (pc *connection) RemoteAddr() net.Addr {
	if pc.conn == nil {
		return nil
	}
	return pc.conn.RemoteAddr()
}

func (pc *connection) Close() {
	if pc.conn != nil {
		pc.conn.Close()
		pc.conn = nil
	}
}

func (pc *connection) Connected() bool {
	return pc.conn != nil
}

var dial = net.Dial

func (pc *connection) Connect() error {
	if pc.Connected() {
		return errors.New("already connected.")
	}
	
	conn, err := dial("tcp", pc.addr.String())
	if err != nil {
		return err
	}
	pc.timeConnected = time.Now()
	pc.conn = conn
	return nil
}

func NewConnection(addr net.Addr) Connection {
	return &connection{
		addr: addr, 
	}
}
