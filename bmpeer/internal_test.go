// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bmpeer

import (
	"net"
	"time"
	"sync/atomic"
)

// TstNewConnection is used to create a new connection with a mock conn instead 
// of a real one for testing purposes. 
func TstNewConnection(conn net.Conn) Connection {
	return &connection {
		conn : conn, 
	}
}

// TstNewListneter returns a new listener with a user defined net.Listener, which
// can be a mock object for testing purposes. 
func TstNewListener(netListen net.Listener) Listener {
	return &listener{
		netListener : netListen, 
	}
}

// SwapDialDial swaps out the dialConnection function to mock it for testing
// purposes. It returns the original function so that it can be swapped back in
// at the end of the test.
func TstSwapDial(f func(string, string) (net.Conn, error)) func(string, string) (net.Conn, error) {
	g := dial
	dial = f
	return g
}

func TstSwapListen(f func(string, string) (net.Listener, error)) func(string, string) (net.Listener, error) {
	g := listen
	listen = f
	return g
}

// TstStart is a special way to start the SendQueue without starting the queue
// handler for testing purposes. 
func (sq *sendQueue) tstStart() {
	// Wait in case the object is resetting.
	sq.resetWg.Wait()

	// Already starting?
	if atomic.AddInt32(&sq.started, 1) != 1 {
		return
	}
	
	// When all three go routines are done, the wait group will unlock.
	sq.doneWg.Add(3)
	
	// Start the three main go routines.
	go sq.outHandler()
	go sq.dataRequestHandler()
}

// TstStartQueueHandler allows for starting the queue handler with a special
// ticker for testing purposes. 
func (sq *sendQueue) tstStartQueueHandler(trickleTicker *time.Ticker) {
	go sq.queueHandler(trickleTicker)
}

func TstStart(sq SendQueue) {
	sq.(*sendQueue).tstStart()
}
 
func TstStartQueueHandler(sq SendQueue, trickleTicker *time.Ticker) {
	sq.(*sendQueue).tstStartQueueHandler(trickleTicker)
}
