// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bmpeer

import (
	"net"
	"sync"
	"time"
	
	"github.com/monetas/bmutil/wire"
)

// Connection
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
	LocalAddr() net.Addr
	Close() error
}

// TODO handle timeout and ping/pong issues.
// connection implements the Connection interface and connects to a 
// real outside bitmessage node over the internet. 
type connection struct {
	conn          net.Conn
	mtx           sync.Mutex
	bytesSent     uint64
	bytesReceived uint64
	lastRead      time.Time
	lastWrite     time.Time
}

func (pc *connection) WriteMessage(msg wire.Message) error {
	// Write the message to the peer.
	n, err := wire.WriteMessageN(pc.conn, msg, wire.MainNet)
	
	pc.mtx.Lock()
	pc.bytesSent += uint64(n)
	pc.lastWrite = time.Now()
	pc.mtx.Unlock()
	return err
}

func (pc *connection) ReadMessage() (wire.Message, error) {
	n, msg, _, err := wire.ReadMessageN(pc.conn, wire.MainNet)
	pc.mtx.Lock()
	pc.bytesReceived += uint64(n)
	pc.lastRead = time.Now()
	pc.mtx.Unlock()

	if err != nil {
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

// LocalAddr returns the localAddr field of the fake connection and satisfies
// the net.Conn interface.
func (pc *connection) LocalAddr() net.Addr {
	return pc.conn.LocalAddr()
}

// RemoteAddr returns the remoteAddr field of the fake connection and satisfies
// the net.Conn interface.
func (pc *connection) RemoteAddr() net.Addr {
	return pc.conn.RemoteAddr()
}

func (pc *connection) Close() error {
	return pc.conn.Close()
}

var dial func(service, addr string) (net.Conn, error) = net.Dial

// dialNewConnection creates a new peerConnection and
// dials out to the given address. 
func Dial(service, addr string) (Connection, error) {
	conn, err := dial(service, addr)
	if err != nil {
		return nil, err
	}
	
	return &connection {
		conn : conn, 
	}, nil
}
