// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bmpeer_test

// TODO make a mock connection suitable for testing this thing. I should be capable
// of blocking and unblocking its functioning to ensure that the channels in sendQueue
// get blocked up. I think a mock database will also be necessary. 

// TODO Test sending a message. Test sending so many messages that the channel fills up.
// Test sending messages that must be cleaned out of the channel. 

// TODO Test sending a data request. Test sending a data request that doesn't exist.
// Test sending a data request already in known inventory. Test that a different 
// kind of message has priority over data requests. Test sending so many data requests
// that the channel locks up. Test sending data requests that have to be cleaned out
// (in both locations.)

// TODO Test sending a trickle of invs to see that an inv request is eventually sent.
// Test sending so many invs that several inv requests are sent. Test sending so many 
// that the channel fills up. 

import (
	"errors"
	"time"
	"net"
	"testing"
	"bytes"
	"sync"
	"fmt"
	"math/rand"
	
	"github.com/monetas/bmutil"
	"github.com/monetas/bmutil/identity"
	"github.com/monetas/bmutil/wire"
	"github.com/monetas/bmd/bmpeer"
	//"github.com/monetas/bmd/database"
)

type MockConnection struct {
	closed bool
	failure bool // When this is true, sending messages returns an error.
	done chan struct{}
	reply chan wire.Message
}

func (mock *MockConnection) WriteMessage(msg wire.Message) error {
	if mock.closed {
		return errors.New("Connection closed.")
	}

	mock.reply <- msg
	
	if mock.failure {
		return errors.New("Mock Connection set to fail.")
	}

	return nil
}

func (mock *MockConnection) MockRead(reset chan struct{}) wire.Message {	
	if mock.closed {
		return nil
	}

	select {
		case <- mock.done :
			return nil
		case message := <- mock.reply :
			return message
		case <- reset:
			return nil
	}
}

func (mc *MockConnection) ReadMessage() (wire.Message, error) {
	return nil, errors.New("This mock connection does not send messages.")
}

func (mc *MockConnection) BytesWritten() uint64 {
	return 0
}

func (mc *MockConnection) BytesRead() uint64 {
	return 0
}

func (mc *MockConnection) LastWrite() time.Time {
	return time.Time{}
}

func (mc *MockConnection) LastRead() time.Time {
	return time.Time{}
}

func (mc *MockConnection) RemoteAddr() net.Addr {
	return nil
}

func (mc *MockConnection) LocalAddr() net.Addr {
	return nil
}

func (mock *MockConnection) Close() error {
	mock.closed = true
	close(mock.done)
	return nil
}

func (mock *MockConnection) SetFailure(b bool) {
	mock.failure = b
}

func NewMockConnection() *MockConnection{
	return &MockConnection{
		done :make(chan struct{}), 
		reply :make(chan wire.Message),
	}
}

// MockDb is a very simple database that does not implement most of the functionality
// which is normally required. 
type MockDb struct {
	get map[wire.ShaHash][]byte
	mtx sync.Mutex
}

func (db *MockDb) Close() error {
	return nil
}

func (db *MockDb) ExistsObject(hash *wire.ShaHash) (bool, error) {
	_, ok := db.get[*hash]
	return ok, nil
}

func (db *MockDb) FetchObjectByHash(hash *wire.ShaHash) ([]byte, error) {
	db.mtx.Lock()
	z, ok := db.get[*hash]
	db.mtx.Unlock()
	if !ok {
		return nil, errors.New("hash not found.")
	}
	return z, nil
}

func (db *MockDb) FetchObjectByCounter(wire.ObjectType, uint64) ([]byte, error) {
	return nil, nil
}

func (db *MockDb) FetchObjectsFromCounter(objType wire.ObjectType, counter uint64,
		count uint64) ([][]byte, uint64, error) {
	return nil, 0, nil
}

func (db *MockDb) FetchIdentityByAddress(*bmutil.Address) (*identity.Public, error) {
	return nil, nil
}

func (db *MockDb) FilterObjects(func(hash *wire.ShaHash,
		obj []byte) bool) (map[wire.ShaHash][]byte, error) {
	return nil, nil
}

func (db *MockDb) FetchRandomInvHashes(count uint64,
		filter func(*wire.ShaHash, []byte) bool) ([]wire.ShaHash, error) {
	return nil, nil	
}

func (db *MockDb) GetCounter(wire.ObjectType) (uint64, error) {
	return 0, nil
}

func (db *MockDb) InsertObject(b []byte) (uint64, error) {
	hash, _ := wire.NewShaHash(bmutil.CalcInventoryHash(b))
	db.get[*hash] = b
	return 0, nil
}

func (db *MockDb) RemoveObject(*wire.ShaHash) error {
	return nil
}

func (db *MockDb) RemoveObjectByCounter(wire.ObjectType, uint64) error {
	return nil
}

func (db *MockDb) RemoveExpiredObjects() error {
	return nil
}

func (db *MockDb) RemovePubKey(*wire.ShaHash) error {
	return nil
}

func (db *MockDb) RollbackClose() (err error) {
	err = nil
	return
}

func (db *MockDb) Sync() (err error) {
	err = nil
	return 
}

func (db *MockDb) Lock() {
	db.mtx.Lock()
}

func (db *MockDb) Unlock() {
	db.mtx.Unlock()
}

func NewMockDb() *MockDb {
	return &MockDb {
		get : make(map[wire.ShaHash][]byte),
	}
}

func TestStartStop(t *testing.T) {
	conn := NewMockConnection()
	db := NewMockDb()
	
	queue := bmpeer.NewSendQueue(conn, db)
	
	queue.Start()
	queue.Stop()
	queue.Start()
	
	// TODO
	// Start the the thing twice at the same time 
	
	// Stop the thing twice at the same time. 
}

func TestSendMessage(t *testing.T) {
	conn := NewMockConnection()
	db := NewMockDb()
	var err error
	
	queue := bmpeer.NewSendQueue(conn, db)
	
	message := &wire.MsgVerAck{}
	
	// The queue isn't running yet, so this should return an error.
	err = queue.QueueMessage(message)
	if err == nil {
		t.Errorf("No error returned when queue is not running.")
	}
	
	queue.Start()
	
	err = queue.QueueMessage(message)
	if err != nil {
		t.Errorf("Error returned: ", err)
	}
	
	sentMsg := conn.MockRead(nil)
	
	if sentMsg != message {
		t.Errorf("Different message received somehow.")
	}
	
	// Now make the connection fail.
	conn.SetFailure(true)
	message = &wire.MsgVerAck{}
	err = queue.QueueMessage(message)
	if err != nil {
		t.Errorf("Error returned at inappropriate time: %s", err)
	}
	conn.MockRead(nil)
	
	if queue.Running() {
		t.Errorf("queue should not be running after this. ")
	}
	conn.SetFailure(false)
	
	queue.Start()
	
	// Engineer a situation in which the message channel gets filled up
	// and must be cleaned out.
	i := 0
	for {
		err = queue.QueueMessage(&wire.MsgVerAck{})
		if i == 20 {
			break
		}
		if err != nil {
			t.Error("Should not have got an error yet.")
		}
		i++
	}
	if err == nil {
		t.Error("Should have got a queue full error.")
	}
	
	queue.Stop()
	
	reset := make(chan struct{})
	go func() {
		for {
			if conn.MockRead(reset) == nil {
				break
			}
		}
	} ()
	
	//Start the queue again to make sure it shuts down properly before the test ends.
	queue.Start()
	close(reset)
}

func TestRequestData(t *testing.T) {
	conn := NewMockConnection()
	db := NewMockDb()
	var err error
	
	queue := bmpeer.NewSendQueue(conn, db)
	
	message := wire.NewMsgUnknownObject(345, time.Now(), wire.ObjectType(4), 1, 1, []byte{77, 82, 53, 48, 96, 1})
	
	hash, _ := wire.MessageHash(message)
	data, _ := wire.EncodeMessage(message)
	db.InsertObject(data)
	hashes := []*wire.InvVect{&wire.InvVect{*hash}}
	
	// The queue isn't running yet, so this should return an error.
	err = queue.QueueDataRequest(hashes)
	if err == nil {
		t.Errorf("No error returned when queue is not running.")
	}
	
	queue.Start()
	
	err = queue.QueueDataRequest(hashes)
	if err != nil {
		t.Errorf("Error returned: ", err)
	}
	
	sentData, _ := wire.EncodeMessage(conn.MockRead(nil))
	
	if !bytes.Equal(data, sentData) {
		t.Errorf("Different message received somehow. ")
	}
	
	// Try to send something that isn't in the database at all and something
	// that isn't a valid message. These don't return errors, but they are here
	// for code coverage. In order to ensure that the queue behaves correctly, 
	// we send a regular message after and receive it.
	badData := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}
	badHash, _ := wire.NewShaHash(bmutil.CalcInventoryHash(badData))
	badHashes := []*wire.InvVect{&wire.InvVect{*badHash}}
	queue.QueueDataRequest(badHashes)
	
	queue.QueueDataRequest(hashes)
	conn.MockRead(nil) // This forces the sendQueue to manage all queued requests.

	db.InsertObject(badData)
	queue.QueueDataRequest(badHashes)

	queue.QueueDataRequest(hashes)
	conn.MockRead(nil) 
	
	fmt.Println("###About to try the thing with all the different things.")
	// Engineer a situation in which the data request channel gets filled up
	// and must be cleaned out.
	db.Lock()
	
	i := 0
	for {
		objMsg := wire.NewMsgUnknownObject(666, time.Now(), wire.ObjectType(4), 1, 1, []byte{0, 0, 0, byte(i)})
		
		hash, _ := wire.MessageHash(objMsg)
		data, _ := wire.EncodeMessage(objMsg)
		db.InsertObject(data)
		hashes := []*wire.InvVect{&wire.InvVect{*hash}}
		
		err = queue.QueueDataRequest(hashes)
		if i == 50 {
			break
		}
		if err != nil {
			t.Errorf("Should not have got an error yet on hash %d", i)
		}
		i++
	}
	if err == nil {
		t.Error("Should have got a queue full error.")
	}
	
	queue.Stop()
	
	reset := make(chan struct{})
	go func() {
		for {
			if conn.MockRead(reset) == nil {
				break
			}
		}
	} ()
	
	//Start the queue again to make sure it shuts down properly before the test ends.
	db.Unlock()
	queue.Start()
	close(reset)
	
	fmt.Println("###About to try the other thing with the stuff.")
	// Engineer a situation in which the data channel gets filled up
	// and must be cleaned out.	
	i = 0
	for {
		objMsg := wire.NewMsgUnknownObject(555, time.Now(), wire.ObjectType(4), 1, 1, []byte{0, 0, 0, byte(i)})
		
		hash, _ := wire.MessageHash(objMsg)
		data, _ := wire.EncodeMessage(objMsg)
		db.InsertObject(data)
		hashes := []*wire.InvVect{&wire.InvVect{*hash}}
		
		err = queue.QueueDataRequest(hashes)
		// Don't need to fill up the channel this time because there is 
		// no error condition to recreate.
		if i == 10 { 
			break
		}
		i++
	}
	
	queue.Stop()
	
	reset = make(chan struct{})
	go func() {
		for {
			if conn.MockRead(reset) == nil {
				break
			}
		}
	} ()
	
	//Start the queue again to make sure it shuts down properly before the test ends.
	queue.Start()
	close(reset)
	fmt.Println("###Test complete.")
}

func randomShaHash() *wire.ShaHash {
	b := make([]byte, 32)
	for i := 0; i < 32; i ++ {
		b[i] = byte(rand.Intn(256))
	}
	hash, _ := wire.NewShaHash(b)
	return hash
}

func TestQueueInv(t *testing.T) {
	tickerChan := make(chan time.Time)
	
	ticker := time.NewTicker(time.Second)
	ticker.C = tickerChan // Make the ticker into something I control. 
	
	conn := NewMockConnection()
	db := NewMockDb()
	
	var err error
	queue := bmpeer.NewSendQueue(conn, db)
	
	// The queue isn't running yet, so this should return an error.
	err = queue.QueueInventory(&wire.InvVect{*randomShaHash()})
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
	} ()
	
	// First send a tick without any invs having been sent. 
	bmpeer.TstStart(queue)
	bmpeer.TstStartQueueHandler(queue, ticker)
	time.Sleep(time.Millisecond * 50)
	tickerChan <- time.Now()
	
	queue.Stop()
	fmt.Println("####Got through first test.")
	bmpeer.TstStart(queue)
	close(reset)
	bmpeer.TstStartQueueHandler(queue, ticker)
	
	// Send an inv and try to get an inv message. 
	inv := &wire.InvVect{*randomShaHash()}
	queue.QueueInventory(inv)
	time.Sleep(time.Millisecond * 50)
	tickerChan <- time.Now()
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
	bmpeer.TstStart(queue)

	// Fill up the channel. 
	i := 0
	for {
		err = queue.QueueInventory(&wire.InvVect{*randomShaHash()})
		if i == 50 {
			break
		}
		if err != nil {
			t.Error("Should not have got an error yet on round ", i)
		}
		i++
	}
	if err == nil {
		t.Error("Should have got a queue full error.")
	}
	
	bmpeer.TstStartQueueHandler(queue, ticker)
	queue.Stop()
	bmpeer.TstStart(queue)
	
	// TODO
	// Require more than one inv be sent in a row. 
}
