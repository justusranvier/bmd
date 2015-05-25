package bmpeer_test

import (
	"testing"
	"net"
	"time"
	"errors"
	
	"github.com/monetas/bmutil/wire"
	"github.com/monetas/bmd/bmpeer"
)

type MessageType uint32

const (
	MessageTypeNone    MessageType = iota
	MessageTypeVersion MessageType = iota
	MessageTypeVerAck  MessageType = iota
	MessageTypeAddr    MessageType = iota
	MessageTypeInv     MessageType = iota
	MessageTypeGetData MessageType = iota
	MessageTypeObject  MessageType = iota
)

// MockLogic is both an instance of Logic and SendQueue. Used for testing
// the peer object. 
type MockLogic struct{
	MessageHeard chan MessageType
	failChan chan struct{}
	failure bool
	inbound bool
}

func (L *MockLogic) SetFailure(b bool) {
	L.failure = b
	if b {
		close(L.failChan)
	} else {
		L.failChan = make(chan struct{})
	}
}

func (L *MockLogic) Listen() MessageType {
	return <-L.MessageHeard
}

func (L *MockLogic) State() bmpeer.PeerState {
	return bmpeer.PeerState(0)
}

func (L *MockLogic) ProtocolVersion() uint32 {
	return 0
}

func (L *MockLogic) Addr() net.Addr {
	return nil
}

func (L *MockLogic) NetAddress() *wire.NetAddress {
	return nil
}

func (L *MockLogic) Inbound() bool {
	return L.inbound
}

func (L *MockLogic) Report(mt MessageType) bool {
	select {
	case <- L.failChan:
		return true
	case L.MessageHeard <- mt :
		return false
	}
}

func (L *MockLogic) HandleVersionMsg(*wire.MsgVersion) error {
	if L.failure || L.Report(MessageTypeVersion) {
		return errors.New("Logic is set to return errors.")
	}
	return nil
}

func (L *MockLogic) HandleVerAckMsg() error {
	if L.failure || L.Report(MessageTypeVerAck) {
		return errors.New("Logic is set to return errors.")
	}
	return nil
}

func (L *MockLogic) HandleAddrMsg(*wire.MsgAddr) error {
	if L.failure || L.Report(MessageTypeAddr) {
		return errors.New("Logic is set to return errors.")
	}
	return nil
}

func (L *MockLogic) HandleInvMsg(*wire.MsgInv) error {
	if L.failure || L.Report(MessageTypeInv) {
		return errors.New("Logic is set to return errors.")
	}
	return nil
}

func (L *MockLogic) HandleGetDataMsg(*wire.MsgGetData) error {
	if L.failure || L.Report(MessageTypeGetData) {
		return errors.New("Logic is set to return errors.")
	}
	return nil
}

func (L *MockLogic) HandleObjectMsg(wire.Message) error {
	if L.failure || L.Report(MessageTypeObject) {
		return errors.New("Logic is set to return errors.")
	}
	return nil
}

func (L *MockLogic) PushVersionMsg() {}

func (L *MockLogic) PushVerAckMsg() {}

func (L *MockLogic) PushAddrMsg(addresses []*wire.NetAddress) error {
	return nil
}

func (L *MockLogic) PushInvMsg(invVect []*wire.InvVect) {}

func (L *MockLogic) PushGetDataMsg(invVect []*wire.InvVect) {}

func (L *MockLogic) PushObjectMsg(sha *wire.ShaHash) {}

func (L *MockLogic) QueueMessage(wire.Message) error {
	return nil
}

func (L *MockLogic) QueueDataRequest([]*wire.InvVect) error {
	L.MessageHeard <- MessageTypeGetData
	return nil
}

func (L *MockLogic) QueueInventory([]*wire.InvVect) error {
	return nil
}

func (L *MockLogic) Start(conn bmpeer.Connection) {}

func (L *MockLogic) Running() bool {
	return true
}
func (L *MockLogic) Stop() {}

// NewMockLogic returns an instance of MockLogic
func NewMockLogic(inbound bool) *MockLogic {
	return &MockLogic {
		MessageHeard: make(chan MessageType), 
		failChan : make(chan struct{}), 
		inbound: inbound, 
	}
}

func TestPeerStartStop(t *testing.T) {
	conn := NewMockConnection(true, false)
	logic := NewMockLogic(false)
	peer := bmpeer.NewPeer(logic, conn, logic, false, 0)
	
	peer.Disconnect()
	if peer.Connected() {
		t.Error("peer should not be running after being stopped before being started. ")
	}

	peer.Start()
	if !peer.Connected() {
		t.Error("peer should be running after being started. ")
	}
	peer.Start()
	if !peer.Connected() {
		t.Error("peer should still be running after being started twice in a row. ")
	}

	peer.Disconnect()
	if peer.Connected() {
		t.Error("peer should not be running after being stopped. ")
	}
	peer.Disconnect()
	if peer.Connected() {
		t.Error("peer should not be running after being stopped twice in a row. ")
	}
	
	// Test the case in which Start and Stop end prematurely because they 
	// are being called by another go routine. 
	waitChan := make(chan struct{})
	startChan := make(chan struct{})
	stopChan := make(chan struct{})
	go func () {
		peer.TstStartWait(waitChan, startChan)
		stopChan <- struct{}{}
	} ()
	// Make sure the queue is definitely in the middle of starting. 
	<-startChan
	peer.Start() 
	waitChan <- struct{}{}
	<-stopChan
	if !peer.Connected() {
		t.Error("peer should be running after being started twice at the same time. ")
	}

	go func () {
		peer.TstDisconnectWait(waitChan, startChan)
		stopChan <- struct{}{}
	} ()
	// Make sure the queue is in the process of stopping already. 
	<-startChan
	peer.Disconnect() 
	waitChan <- struct{}{}
	<-stopChan
	if peer.Connected() {
		t.Error("peer should not be running after being stopped twice at the same time. ")
	}
}

func TestConnect(t *testing.T) {
	conn := NewMockConnection(false, true)
	logic := NewMockLogic(false)
	peer := bmpeer.NewPeer(logic, conn, logic, false, 0)

	err := 	peer.Start()
	if err == nil {
		t.Error("Should have failed to connect.")
	}
	
	conn = NewMockConnection(true, false)
	peer = bmpeer.NewPeer(logic, conn, logic, false, 0)
	err = peer.Connect()
	if err != nil {
		t.Errorf("Connect returned error: %s", err)
	}
	
	conn = NewMockConnection(false, false)
	peer = bmpeer.NewPeer(logic, conn, logic, false, 0)
	err = peer.Connect()
	if err != nil {
		t.Errorf("Connect returned error: %s", err)
	}
	
	conn = NewMockConnection(true, false)
	peer = bmpeer.NewPeer(logic, conn, logic, false, 0)

	waitChan := make(chan struct{})
	startChan := make(chan struct{})
	stopChan := make(chan struct{})
	peer.Start() 
	
	go func () {
		peer.TstDisconnectWait(waitChan, startChan)
		stopChan <- struct{}{}
	} ()
	// Make sure the queue is in the process of stopping already. 
	<-startChan
	err = peer.Connect() 
	waitChan <- struct{}{}
	<-stopChan
	if err == nil {
		t.Error("peer should have returned an error for trying to connect in the middle of a disconnect. ")
	}
}

// TestPeer tests that every kind of message is routed correctly. 
func TestPeer(t *testing.T) {
	conn := NewMockConnection(true, false)
	logic := NewMockLogic(true)
	peer := bmpeer.NewPeer(logic, conn, logic, false, 0)
	
	peer.Start()
	
	var addrin, addrout *wire.NetAddress = wire.NewNetAddressIPPort(net.IPv4(5, 45, 99, 75), 8444, 1, 0),
		wire.NewNetAddressIPPort(net.IPv4(5, 45, 99, 75), 8444, 1, 0)
	
	conn.MockWrite(wire.NewMsgVersion(addrin, addrout, 67, []uint32{1}))
	messageType := logic.Listen()
	if messageType != MessageTypeVersion {
		t.Error("Version message expected; got ", messageType)
	}
	
	conn.MockWrite(&wire.MsgVerAck{})
	messageType = logic.Listen()
	if messageType != MessageTypeVerAck {
		t.Error("VerAck message expected; got ", messageType)
	}
	
	conn.MockWrite(wire.NewMsgAddr())
	messageType = logic.Listen()
	if messageType != MessageTypeAddr {
		t.Error("Addr message expected; got ", messageType)
	}
	
	conn.MockWrite(wire.NewMsgInv())
	messageType = logic.Listen()
	if messageType != MessageTypeInv {
		t.Error("Inv message expected; got ", messageType)
	}
	
	conn.MockWrite(wire.NewMsgGetData())
	messageType = logic.Listen()
	if messageType != MessageTypeGetData {
		t.Error("GetData message expected; got ", messageType)
	}
	
	expires := time.Now().Add(10 * time.Minute)
	obj := wire.NewMsgUnknownObject(345, expires, wire.ObjectType(4), 1, 1, []byte{77, 82, 53, 48, 96, 1})
	conn.MockWrite(obj)
	messageType = logic.Listen()
	if messageType != MessageTypeObject {
		t.Error("Object message expected; got ", messageType)
	}
	
	if peer.Logic() == nil {
		t.Error("Peer's logic not returned.")
	}
	
	peer.Disconnect() 
}

// TestPeerError tests error paths in the peer's inHandler function. 
func TestPeerError(t *testing.T) {
	conn := NewMockConnection(true, false)
	logic := NewMockLogic(true)
	peer := bmpeer.NewPeer(logic, conn, logic, false, 0)
	
	peer.Start()
	conn.SetFailure(true)
	time.Sleep(time.Millisecond * 50)
	
	if peer.Connected() {
		t.Error("Peer should have disconnected.")
	}
	conn.SetFailure(false)
	
	peer = bmpeer.NewPeer(logic, conn, logic, false, 0)
	peer.Start()
	
	logic.SetFailure(true)
	conn.MockWrite(&wire.MsgVerAck{})
}

func TestTimeout(t *testing.T) {
	conn := NewMockConnection(true, false)
	logic := NewMockLogic(true)
	peer := bmpeer.NewPeer(logic, conn, logic, false, 0)

	peer.TstStart(1, 1)
	if !peer.Connected() {
		t.Fatal("Peer should have connected here.")
	}
	time.Sleep(2*time.Second)
	if peer.Connected() {
		t.Error("Peer should have disconnected.")
	}
}
