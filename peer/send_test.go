// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer_test

import (
	"bytes"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/monetas/bmd/database"
	_ "github.com/monetas/bmd/database/memdb"
	"github.com/monetas/bmd/peer"
	"github.com/monetas/bmutil"
	"github.com/monetas/bmutil/wire"
)

// MockConnection implements the Connection interface and is used to test
// send without connecting to the real internet.
type MockConnection struct {
	closed      bool // Whether the connection has been closed.
	connected   bool // Whether the connection is connected.
	failure     bool // When this is true, sending or receiving messages returns an error.
	connectFail bool // When this is true, the connection cannot connect.
	done        chan struct{}
	failChan    chan struct{}
	reply       chan wire.Message
	send        chan wire.Message
	addr        net.Addr
	mutex       sync.RWMutex
	failmtx     sync.Mutex
}

func (mock *MockConnection) WriteMessage(msg wire.Message) error {
	if mock.closed {
		return errors.New("Connection closed.")
	}

	if mock.failure {
		return errors.New("Mock Connection set to fail.")
	}

	mock.reply <- msg

	return nil
}

// MockRead allows for tests as to whether a message has been sent or not.
// It accepts an extra channel that can be read from if no message has been
// sent. Normally, Read blocks until a message is received.
func (mock *MockConnection) MockRead(reset chan struct{}) wire.Message {
	if mock.closed {
		return nil
	}

	select {
	case <-mock.done:
		return nil
	case message := <-mock.reply:
		return message
	case <-reset:
		return nil
	}
}

func (mock *MockConnection) ReadMessage() (wire.Message, error) {
	mock.mutex.RLock()
	fail := mock.failure
	mock.mutex.RUnlock()

	if fail {
		return nil, errors.New("Mock Connection set to fail.")
	}

	mock.failmtx.Lock()
	defer mock.failmtx.Unlock()

	select {
	case msg := <-mock.send:
		return msg, nil
	case <-mock.failChan:
		return nil, errors.New("Mock Connection set to fail.")
	}
}

func (mock *MockConnection) MockWrite(msg wire.Message) {
	mock.send <- msg
}

func (mock *MockConnection) BytesWritten() uint64 {
	return 0
}

func (mock *MockConnection) BytesRead() uint64 {
	return 0
}

func (mock *MockConnection) LastWrite() time.Time {
	return time.Time{}
}

func (mock *MockConnection) LastRead() time.Time {
	return time.Time{}
}

func (mock *MockConnection) RemoteAddr() net.Addr {
	return mock.addr
}

func (mock *MockConnection) LocalAddr() net.Addr {
	return nil
}

func (mock *MockConnection) Close() {
	if mock.closed {
		return
	}
	mock.closed = true
	mock.mutex.Lock()
	mock.connected = false
	mock.mutex.Unlock()
	close(mock.done)

	// Drain any incoming messages.
close:
	for {
		select {
		case <-mock.reply:
		default:
			break close
		}
	}
}

func (mock *MockConnection) Connected() bool {
	mock.mutex.RLock()
	defer mock.mutex.RUnlock()

	return mock.connected
}

func (mock *MockConnection) Connect() error {
	if mock.connectFail {
		return errors.New("Connection set to fail.")
	}
	mock.mutex.Lock()
	mock.connected = true
	mock.mutex.Unlock()
	return nil
}

func (mock *MockConnection) SetFailure(b bool) {
	mock.mutex.Lock()
	mock.failure = b
	mock.mutex.Unlock()

	if b == true {
		// Drain any messages being sent now so that WriteMessage will
		// immediately return an error.
		select {
		case <-mock.done:
		default:
		}

		// Close the failure channel to ensure any messages being read
		// will return an error.
		close(mock.failChan)

	} else {
		mock.failmtx.Lock()
		mock.failChan = make(chan struct{})
		mock.failmtx.Unlock()
	}
}

func NewMockConnection(addr net.Addr, connected bool, fails bool) *MockConnection {
	return &MockConnection{
		done:        make(chan struct{}),
		reply:       make(chan wire.Message),
		send:        make(chan wire.Message),
		failChan:    make(chan struct{}),
		connected:   connected,
		connectFail: fails,
		addr:        addr,
	}
}

var mockAddr net.Addr = &net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 8333}

func TestSendStartStop(t *testing.T) {
	conn := NewMockConnection(mockAddr, true, false)
	db, _ := database.CreateDB("memdb")

	queue := peer.NewSend(peer.NewInventory(), db)

	if queue.Running() {
		t.Errorf("queue should not be running yet. ")
	}
	queue.Stop()
	if queue.Running() {
		t.Errorf("queue should still not be running. ")
	}

	queue.Start(conn)
	if !queue.Running() {
		t.Errorf("queue should be running. ")
	}
	queue.Stop()
	if queue.Running() {
		t.Errorf("queue should not be running anymore. ")
	}

	// Test the case in which Start and Stop end prematurely because they
	// are being called by another go routine.
	waitChan := make(chan struct{})
	startChan := make(chan struct{})
	stopChan := make(chan struct{})
	go func() {
		peer.TstSendStartWait(queue, conn, waitChan, startChan)
		stopChan <- struct{}{}
	}()
	// Make sure the queue is definitely in the middle of starting.
	<-startChan
	queue.Start(conn)
	waitChan <- struct{}{}
	<-stopChan
	if !queue.Running() {
		t.Errorf("queue should be running after being started twice. ")
	}

	go func() {
		peer.TstSendStopWait(queue, waitChan, startChan)
		stopChan <- struct{}{}
	}()
	// Make sure the queue is in the process of stopping already.
	<-startChan
	queue.Stop()
	waitChan <- struct{}{}
	<-stopChan
	if queue.Running() {
		t.Errorf("queue should not be running. ")
	}
}

func TestSendMessage(t *testing.T) {
	conn := NewMockConnection(mockAddr, true, false)
	db, _ := database.CreateDB("memdb")
	var err error

	queue := peer.NewSend(peer.NewInventory(), db)

	message := &wire.MsgVerAck{}

	// Try sending a message to a send that is not running yet.
	// This should return an error.
	err = queue.QueueMessage(message)
	if err == nil {
		t.Errorf("No error returned when queue is not running.")
	}

	// Now try to send the message after the queue is started.
	queue.Start(conn)
	err = queue.QueueMessage(message)
	if err != nil {
		t.Errorf("Error returned: %s", err)
	}

	sentMsg := conn.MockRead(nil)
	if sentMsg != message {
		t.Errorf("Different message received somehow.")
	}

	// Now test that the send shuts down if the connection fails.
	conn.SetFailure(true)
	message = &wire.MsgVerAck{}
	err = queue.QueueMessage(message)
	if err != nil {
		t.Errorf("Error returned at inappropriate time: %s", err)
	}

	// Give the queue some time to shut down completely.
	time.Sleep(time.Millisecond * 50)

	if queue.Running() {
		t.Errorf("queue should not be running after this. ")
	}

	queue.Start(conn)

	queue.Stop()
	time.Sleep(time.Millisecond * 50)

}

func TestRequestData(t *testing.T) {
	conn := NewMockConnection(mockAddr, true, false)
	db, _ := database.CreateDB("memdb")

	var err error

	queue := peer.NewSend(peer.NewInventory(), db)
	message := wire.NewMsgUnknownObject(345, time.Now(), wire.ObjectType(4), 1, 1, []byte{77, 82, 53, 48, 96, 1})

	objMsg, _ := wire.ToMsgObject(message)
	db.InsertObject(objMsg)
	hashes := []*wire.InvVect{&wire.InvVect{Hash: *objMsg.InventoryHash()}}

	// The queue isn't running yet, so this should return an error.
	err = queue.QueueDataRequest(hashes)
	if err == nil {
		t.Errorf("No error returned when queue is not running.")
	}

	queue.Start(conn)

	err = queue.QueueDataRequest(hashes)
	if err != nil {
		t.Errorf("Error returned: %s", err)
	}

	sentData := conn.MockRead(nil)

	if !bytes.Equal(wire.EncodeMessage(message), wire.EncodeMessage(sentData)) {
		t.Errorf("Different message received somehow. ")
	}

	// Try to send something that isn't in the database at all and something
	// that isn't a valid message. These don't return errors, but they are here
	// for code coverage. In order to ensure that the queue behaves correctly,
	// we send a regular message after and receive it.
	badData := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}
	badHash, _ := wire.NewShaHash(bmutil.Sha512(badData)[:32])
	badHashes := []*wire.InvVect{&wire.InvVect{Hash: *badHash}}
	queue.QueueDataRequest(badHashes)

	queue.QueueDataRequest(hashes)
	// Force the send to manage all queued requests.
	sentData = conn.MockRead(nil)
	if !bytes.Equal(wire.EncodeMessage(message), wire.EncodeMessage(sentData)) {
		t.Errorf("Wrong message returned.")
	}

	queue.QueueDataRequest(badHashes)

	queue.QueueDataRequest(hashes)
	sentData = conn.MockRead(nil)
	if !bytes.Equal(wire.EncodeMessage(message), wire.EncodeMessage(sentData)) {
		t.Errorf("Wrong message returned.")
	}

	queue.Stop()

	//Start the queue again to make sure it shuts down properly before the test ends.
	queue.Start(conn)

	// Engineer a situation in which the data channel gets filled up
	// and must be cleaned out.
	i := 0
	for {
		objMsg := wire.NewMsgUnknownObject(555, time.Now(), wire.ObjectType(4), 1, 1, []byte{0, 0, 0, byte(i)})

		msg, _ := wire.ToMsgObject(objMsg)
		db.InsertObject(msg)
		hashes := []*wire.InvVect{&wire.InvVect{Hash: *msg.InventoryHash()}}

		err = queue.QueueDataRequest(hashes)
		// Don't need to fill up the channel this time because there is
		// no error condition to recreate.
		if i == 10 {
			break
		}
		i++
	}

	queue.Stop()
}

func TestQueueInv(t *testing.T) {
	timerChan := make(chan time.Time)

	timer := time.NewTimer(time.Hour)
	timer.C = timerChan // Make the ticker into something I control.

	conn := NewMockConnection(mockAddr, true, false)
	db, _ := database.CreateDB("memdb")

	var err error
	queue := peer.NewSend(peer.NewInventory(), db)

	// The queue isn't running yet, so this should return an error.
	err = queue.QueueInventory([]*wire.InvVect{&wire.InvVect{Hash: *randomShaHash()}})
	if err == nil {
		t.Errorf("No error returned when queue is not running.")
	}

	// MockRead blocks if there is no message, so we run it in a special
	// go routine here to detect whether it received anything.
	reset := make(chan struct{})
	go func() {
		if conn.MockRead(reset) != nil {
			t.Error("message returned when there should have been none.")
		}
	}()

	// Give the other thread some time to confirm that no messages are in the channel.
	time.Sleep(50 * time.Millisecond)
	reset <- struct{}{}

	// Send a tick without any invs having been sent.
	peer.TstSendStart(queue, conn)
	peer.TstSendStartQueueHandler(queue, timer)
	// Time for the send queue to get started.
	time.Sleep(time.Millisecond * 50)
	timerChan <- time.Now()

	queue.Stop()
	peer.TstSendStart(queue, conn)
	close(reset)
	peer.TstSendStartQueueHandler(queue, timer)

	// Send an inv and try to get an inv message.
	inv := &wire.InvVect{Hash: *randomShaHash()}
	queue.QueueInventory([]*wire.InvVect{inv})
	// Pause a teensy bit to ensure that the peer handler gets the message
	// and queues it up and that there will be something to read when
	// we call MockRead.
	time.Sleep(time.Millisecond * 50)
	timerChan <- time.Now()
	msg := conn.MockRead(nil)

	switch msg.(type) {
	case *wire.MsgInv:
		invList := msg.(*wire.MsgInv).InvList
		if !(len(invList) == 1 && invList[0] == inv) {
			t.Error("Wrong inv received?")
		}
	default:
		t.Error("Wrong message type received:", msg)
	}

	queue.Stop()

	peer.TstSendStart(queue, conn)
	peer.TstSendStartQueueHandler(queue, timer)

	for i := 0; i < 6; i++ {
		invTrickleSize := 1200
		invList := make([]*wire.InvVect, invTrickleSize)
		for j := 0; j < invTrickleSize; j++ {
			invList[j] = &wire.InvVect{Hash: *randomShaHash()}
		}
		err = queue.QueueInventory(invList)
	}
	// Give the queue handler some time to run.
	time.Sleep(time.Millisecond * 50)

	// Now drain the Send of all messages.
	reset = make(chan struct{})
	go func() {
		for conn.MockRead(reset) != nil {
		}
	}()

	timerChan <- time.Now()
	time.Sleep(time.Millisecond * 50)

	queue.Stop()
	close(reset)

	// Finally, test the line that drains invSendQueue if the program disconnects.
	peer.TstSendStart(queue, conn)
	peer.TstSendStartQueueHandler(queue, timer)

	for i := 0; i < 6; i++ {
		invTrickleSize := 1200
		invList := make([]*wire.InvVect, invTrickleSize)
		for j := 0; j < invTrickleSize; j++ {
			invList[j] = &wire.InvVect{Hash: *randomShaHash()}
		}
		err = queue.QueueInventory(invList)
	}
	// Give the queue handler some time to run.
	time.Sleep(time.Millisecond * 50)
	queue.Stop()

	// Start and stop again to make sure the test doesn't end before the queue
	// has shut down the last time.
	peer.TstSendStart(queue, conn)
	queue.Stop()
}

func TestRetrieveObject(t *testing.T) {
	db, _ := database.CreateDB("memdb")

	// An object that is not in the database.
	notThere := &wire.InvVect{Hash: *randomShaHash()}

	// A valid object that will be in the database.
	goodData := wire.NewMsgUnknownObject(345, time.Now(),
		wire.ObjectType(4), 1, 1, []byte{77, 82, 53, 48, 96, 1}).ToMsgObject()
	goodInv := &wire.InvVect{Hash: *goodData.InventoryHash()}
	db.InsertObject(goodData)

	// Retrieve objects that are not in the database.
	obj := peer.TstRetrieveObject(db, notThere)
	if obj != nil {
		t.Error("Object returned that should not have been in the database: ", obj)
	}

	// Retrieve good objects from the database.
	if peer.TstRetrieveObject(db, goodInv) == nil {
		t.Error("No object returned.")
	}
}
