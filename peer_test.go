package main

import (
	"bytes"
	"fmt"
	"net"
	"testing"
	"time"
	"math/rand"
	"errors"

	"github.com/monetas/bmutil/wire"
	"github.com/monetas/bmd/database"
	"github.com/monetas/bmd/database/memdb"
	"github.com/monetas/bmd/bmpeer"
)

// PeerAction represents a an action to be taken by the mock peer and information
// about the expected response from the real peer. 
type PeerAction struct {
	// A series of messages to send to the real peer. 
	Messages            []wire.Message  
	
	// If an error is set, the interaction ends immediately.
	Err                 error                
	
	// Whether the interaction is complete.
	InteractionComplete bool 
	
	// For negative tests, we expect the peer to disconnect after the interaction is
	// complete. If disconnectExpected is set, the test fails if the real peer fails
	// to disconnect after half a second. 
	DisconnectExpected  bool  
}

// PeerTest is a type that defines the high-level behavior of a peer. 
type PeerTest interface {
	OnStart() *PeerAction
	OnMsgVersion(p *wire.MsgVersion) *PeerAction
	OnMsgVerAck(p *wire.MsgVerAck) *PeerAction
	OnMsgAddr(p *wire.MsgAddr) *PeerAction
	OnMsgInv(p *wire.MsgInv) *PeerAction
	OnMsgGetData(p *wire.MsgGetData) *PeerAction
	OnMsgObject(p wire.Message) *PeerAction
}

type OutboundHandshakePeerTester struct {
	versionReceived bool
	handshakeComplete bool
	response *PeerAction
	msgAddr wire.Message
}

func (peer *OutboundHandshakePeerTester) OnStart() *PeerAction {
	return nil
}

func (peer *OutboundHandshakePeerTester) OnMsgVersion(version *wire.MsgVersion) *PeerAction {
	if peer.versionReceived {
		return &PeerAction{Err:errors.New("Two version messages received")}
	} else {
		peer.versionReceived = true
		return peer.response
	}
}

func (peer *OutboundHandshakePeerTester) OnMsgVerAck(p *wire.MsgVerAck) *PeerAction {
	if !peer.versionReceived {
		return &PeerAction{Err: errors.New("Expecting version message first.")}
	}
	peer.handshakeComplete = true
	return &PeerAction{
		Messages : []wire.Message{peer.msgAddr}, 
		InteractionComplete : true, 
		DisconnectExpected : false, 
	}
}

func (peer *OutboundHandshakePeerTester) OnMsgAddr(p *wire.MsgAddr) *PeerAction {
	if peer.handshakeComplete {
		return nil
	} else {
		return &PeerAction{Err: errors.New("Not allowed before handshake is complete.")}
	}
}

func (peer *OutboundHandshakePeerTester) OnMsgInv(p *wire.MsgInv) *PeerAction {
	if peer.handshakeComplete {
		return nil
	} else {
		return &PeerAction{Err: errors.New("Not allowed before handshake is complete.")}
	}
}

func (peer *OutboundHandshakePeerTester) OnMsgGetData(p *wire.MsgGetData) *PeerAction {
	if peer.handshakeComplete {
		return nil
	} else {
		return &PeerAction{Err: errors.New("Not allowed before handshake is complete.")}
	}
}

func (peer *OutboundHandshakePeerTester) OnMsgObject(p wire.Message) *PeerAction {
	if peer.handshakeComplete {
		return nil
	} else {
		return &PeerAction{Err: errors.New("Object message not allowed before handshake is complete.")}
	}
}

func NewOutboundHandshakePeerTester(action *PeerAction, msgAddr wire.Message) *OutboundHandshakePeerTester {
	return &OutboundHandshakePeerTester{
		versionReceived: false, 
		handshakeComplete: false, 
		response : action, 
		msgAddr : msgAddr, 
	}
}

// inboundHandsakePeerTester implements the PeerTest interface and is used to test
// handshakes with inbound peers and the exchange of the addr messages. 
type InboundHandshakePeerTester struct {
	verackReceived bool
	versionReceived bool
	handshakeComplete bool
	openMsg *PeerAction
	addrAction *PeerAction
}

func (peer *InboundHandshakePeerTester) OnStart() *PeerAction {
	return peer.openMsg
}

func (peer *InboundHandshakePeerTester) OnMsgVersion(version *wire.MsgVersion) *PeerAction {
	if peer.versionReceived {
		return &PeerAction{Err :errors.New("Two versions received?")}
	}
	peer.versionReceived = true
	if peer.verackReceived {
		// Handshake completed successfully
		peer.handshakeComplete = true
		
		// Sending an addr message to make sure the peer doesn't disconnect.
		return peer.addrAction
	}
	return &PeerAction{[]wire.Message{&wire.MsgVerAck{}}, nil, false, false}
}

func (peer *InboundHandshakePeerTester) OnMsgVerAck(p *wire.MsgVerAck) *PeerAction {
	if peer.verackReceived {
		return &PeerAction{Err:errors.New("Two veracks received?")}
	}
	peer.verackReceived = true
	if peer.versionReceived {
		// Handshake completed successfully
		peer.handshakeComplete = true

		//Sending an addr message to make sure the peer doesn't disconnect.
		return peer.addrAction
	}
	// Return nothing and await a version message.
	return &PeerAction{nil, nil, false, false}
}

func (peer *InboundHandshakePeerTester) OnMsgAddr(p *wire.MsgAddr) *PeerAction {
	if peer.handshakeComplete {
		return &PeerAction{nil, nil, true, false}
	} else {
		return &PeerAction{Err: errors.New("Not allowed before handshake is complete.")}
	}
}

func (peer *InboundHandshakePeerTester) OnMsgInv(p *wire.MsgInv) *PeerAction {
	if peer.handshakeComplete {
		return nil
	} else {
		return &PeerAction{Err: errors.New("Not allowed before handshake is complete.")}
	}
}

func (peer *InboundHandshakePeerTester) OnMsgGetData(p *wire.MsgGetData) *PeerAction {
	if peer.handshakeComplete {
		return nil
	} else {
		return &PeerAction{Err: errors.New("Not allowed before handshake is complete.")}
	}
}

func (peer *InboundHandshakePeerTester) OnMsgObject(p wire.Message) *PeerAction {
	if peer.handshakeComplete {
		return nil
	} else {
		return &PeerAction{Err: errors.New("Object message not allowed before handshake is complete.")}
	}
}

func NewInboundHandshakePeerTester(openingMsg *PeerAction, addrAction *PeerAction) *InboundHandshakePeerTester {
	return &InboundHandshakePeerTester{
		verackReceived : false, 
		versionReceived : false, 
		handshakeComplete : false, 
		openMsg : openingMsg,  
		addrAction : addrAction, 
	}
}

// DataExchangePeerTester implements the PeerTest interface and tests the exchange 
// of inv messages and data. 
type DataExchangePeerTester struct {
	verackReceived bool
	versionReceived bool
	handshakeComplete bool
	objectSent bool
	objectReceived bool
	openMsg *PeerAction
	invAction *PeerAction
	inventory map[wire.ShaHash]wire.Message     // The initial inventory of the mock peer. 
	peerInventory map[wire.ShaHash]wire.Message // The initial inventory of the real peer. 
	requested map[wire.ShaHash]wire.Message     // The messages that were requested. 
}

func (peer *DataExchangePeerTester) OnStart() *PeerAction {
	return peer.openMsg
}

func (peer *DataExchangePeerTester) OnMsgVersion(version *wire.MsgVersion) *PeerAction {
	if peer.versionReceived {
		return &PeerAction{Err :errors.New("Two versions received?")}
	}
	peer.versionReceived = true
	if peer.verackReceived {
		// Handshake completed successfully
		peer.handshakeComplete = true
		
		// Send an inv message.
		return peer.invAction
	}
	return &PeerAction{[]wire.Message{&wire.MsgVerAck{}}, nil, false, false}
}

func (peer *DataExchangePeerTester) OnMsgVerAck(p *wire.MsgVerAck) *PeerAction {
	if peer.verackReceived {
		return &PeerAction{Err:errors.New("Two veracks received?")}
	}
	peer.verackReceived = true
	if peer.versionReceived {
		// Handshake completed successfully
		peer.handshakeComplete = true

		// 
		return peer.invAction
	}
	// Return nothing and await a version message.
	return &PeerAction{nil, nil, false, false}
}

func (peer *DataExchangePeerTester) OnMsgAddr(p *wire.MsgAddr) *PeerAction {
	if peer.handshakeComplete {
		return nil
	} else {
		return &PeerAction{Err: errors.New("Not allowed before handshake is complete.")}
	}
}

func (peer *DataExchangePeerTester) OnMsgInv(inv *wire.MsgInv) *PeerAction {
	if peer.handshakeComplete {
		if len(inv.InvList) == 0 {
			return &PeerAction{Err: errors.New("Empty inv message received.")}
		}		
		
		// TODO Construct the get data method. 
		getData := wire.NewMsgGetData()
		
		return &PeerAction {
			Messages : []wire.Message{getData},
		}
	} else {
		return &PeerAction{Err: errors.New("Not allowed before handshake is complete.")}
	}
}

func (peer *DataExchangePeerTester) OnMsgGetData(getData *wire.MsgGetData) *PeerAction {
	if peer.handshakeComplete {
		if len(getData.InvList) < 1 || len(getData.InvList) > 50000 {
			return &PeerAction{Err: errors.New("Invalid get data message.")}
		}
		
		// The get data message should include no duplicates and should include nothing
		// that the peer already knows. 
		data := make(map[wire.ShaHash]wire.Message)
		for _, invVect := range getData.InvList {
			if _, ok := data[invVect.Hash]; ok {
				return &PeerAction{Err: errors.New("GetData request returned with duplicate entries.")}
			}
			
			if _, ok := peer.peerInventory[invVect.Hash]; ok {
				return &PeerAction{Err: errors.New("Peer requested data that it already has.")}
			}
			
			datum, ok := peer.inventory[invVect.Hash]
			if !ok {
				return &PeerAction{Err: errors.New("Peer requested data that the mock peer does not have.")}
			}
			
			data[invVect.Hash] = datum
		}
		
		messages := make([]wire.Message, len(data))
		i := 0
		for _, datum := range data {
			messages[i] = datum
			i++
		}

		peer.objectSent = true

		if peer.objectReceived {
			return &PeerAction{messages, nil, true, false}
		} else {
			return &PeerAction{messages, nil, false, false}
		}
	} else {
		return &PeerAction{Err: errors.New("Not allowed before handshake is complete.")}
	}
}

func (peer *DataExchangePeerTester) OnMsgObject(message wire.Message) *PeerAction {
	if peer.handshakeComplete {

		hash, _ := wire.MessageHash(message)
		if _, ok := peer.requested[*hash]; !ok {
			return &PeerAction{Err : errors.New("Sent unrequested object.")}
		}
		delete(peer.requested, *hash)
		
		if(len(peer.requested) == 0) {
			peer.objectReceived = true
		}
		
		if peer.objectSent {
			// Exchange has completed successfully. 
			return &PeerAction{nil, nil, true, false}
		} else {
			return nil
		}
	} else {
		return &PeerAction{Err: errors.New("Object message not allowed before handshake is complete.")}
	}
}

func NewDataExchangePeerTester(openMsg *PeerAction, inventory []wire.Message, peerInventory []wire.Message, invAction *PeerAction) *DataExchangePeerTester {
	// Catalog the initial inventories of the mock peer and real peer. 
	in := make(map[wire.ShaHash]wire.Message)
	pin := make(map[wire.ShaHash]wire.Message)
	for _, message := range inventory {
		hash, _ := wire.MessageHash(message)
		in[*hash] = message
	}
	for _, message := range peerInventory {
		hash, _ := wire.MessageHash(message)
		pin[*hash] = message
	}
	
	if invAction == nil {
		// TODO construct an inv action based on the given inventory. 
	}
	
	return &DataExchangePeerTester{
		verackReceived : false, 
		versionReceived : false, 
		handshakeComplete : false, 
		openMsg : openMsg, 
		inventory : in, 
		peerInventory : pin, 
		invAction : invAction, 
	}
}

// The report that the MockConnection sends when its interaction is complete. 
// If the error is nil, then the test has completed successfully. 
type TestReport struct {
	Err error
}

// MockConnection implements the Connection interface and 
// is a mock peer to be used for testing purposes. 
type MockConnection struct {
	// After a message is processed, the replies are sent here.
	reply  chan []wire.Message 
	
	// A channel used to report invalid states to the test.
	report chan TestReport       
	
	// A function that manages the sequence of steps that the test should go through.
	// This is the part that is customized for each test. 
	//handle     func(wire.Message) *PeerAction
	peerTest PeerTest
	
	// A queue of messages to be received by the real peer.
	msgQueue   []wire.Message  
	
	// The current place in the queue.
	queuePlace int             
	
	// The test ends when the timer runs out. Until the interaction is complete,
	// the timer is reset with every message received. Some time is required after 
	// the mock peer is done interacting with the real one to see if the real peer
	// disconnects or not. 
	timer      *time.Timer     
	
	// when interactionComplete is set, the mock peer no longer processes incoming 
	// messages or resets the timer. It is just waiting to see whether the real peer
	// disconnects. 
	interactionComplete bool   
	
	// Whether the peer is expected to disconnect.
	disconnectExpected bool

	localAddr  net.Addr
	remoteAddr net.Addr 
}

func (mock *MockConnection) ReadMessage() (wire.Message, error) {	
	//If the queue is empty, get a new message from the channel.
	for mock.msgQueue == nil || mock.queuePlace >= len(mock.msgQueue) {
		mock.msgQueue = <-mock.reply
		mock.queuePlace = 0
	}
	
	toSend := mock.msgQueue[mock.queuePlace]

	mock.queuePlace++
	
	if mock.queuePlace >= len(mock.msgQueue) {
		mock.msgQueue = nil
	}

	return toSend, nil
}

// Send figures out how to respond to a message once it is decyphered.
func (mock *MockConnection) WriteMessage(rmsg wire.Message) error {
	// We can keep receiving messages after the interaction is done; we just ignore them.	
	// We are waiting to see whether the peer disconnects.
	if mock.interactionComplete {
		return nil
	}
	
	mock.timer.Reset(time.Second)
	mock.handleAction(mock.handleMessage(rmsg))
	return nil
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

// LocalAddr returns the localAddr field of the fake connection and satisfies
// the net.Conn interface.
func (mock *MockConnection) LocalAddr() net.Addr {
	return mock.localAddr
}

// RemoteAddr returns the remoteAddr field of the fake connection and satisfies
// the net.Conn interface.
func (mock *MockConnection) RemoteAddr() net.Addr {
	return mock.remoteAddr
}

func (mock *MockConnection) Close() error {
	return nil
}

func (mock *MockConnection) handleMessage(msg wire.Message) *PeerAction {
	switch msg.(type) {
	case *wire.MsgVersion:
		return mock.peerTest.OnMsgVersion(msg.(*wire.MsgVersion))
	case *wire.MsgVerAck:
		return mock.peerTest.OnMsgVerAck(msg.(*wire.MsgVerAck))
	case *wire.MsgAddr:
		return mock.peerTest.OnMsgAddr(msg.(*wire.MsgAddr))
	case *wire.MsgGetPubKey:
		return mock.peerTest.OnMsgObject(msg.(*wire.MsgGetPubKey))
	case *wire.MsgPubKey:
		return mock.peerTest.OnMsgObject(msg.(*wire.MsgPubKey))
	case *wire.MsgMsg:
		return mock.peerTest.OnMsgObject(msg.(*wire.MsgMsg))
	case *wire.MsgBroadcast:
		return mock.peerTest.OnMsgObject(msg.(*wire.MsgBroadcast))
	case *wire.MsgUnknownObject:
		return mock.peerTest.OnMsgObject(msg.(*wire.MsgUnknownObject))
	default:
		return nil
	}
}

func (mock *MockConnection) handleAction(action *PeerAction) {
	// A nil response means to do nothing. 
	if action == nil {
		return
	}
	
	// If an error is returned, immediately end the interaction.
	if action.Err != nil {
		mock.Done(action.Err)
		return
	}
	
	mock.interactionComplete = mock.interactionComplete || action.InteractionComplete
	
	mock.disconnectExpected = mock.disconnectExpected || action.DisconnectExpected
	
	if action.Messages != nil {
		mock.reply <- action.Messages
	}
}

// ConnectionClosed is called when the real peer closes the connection to the mock peer.
func (mock *MockConnection) ConnectionClosed() {
	if !mock.interactionComplete {
		if !mock.disconnectExpected || (mock.msgQueue != nil && mock.queuePlace < len(mock.msgQueue)) {
			mock.Done(errors.New("Connection closed prematurely."))
		}
		mock.Done(nil)
	}
}

// Done stops the server and ends the test. 
func (mock *MockConnection) Done(err error) {
	mock.timer.Stop()
	mock.interactionComplete = true
	mock.report <- TestReport{err}
}

// Start loads the mock peer's initial action if there is one.
func (mock *MockConnection) Start() {
	action := mock.peerTest.OnStart()
	
	if action == nil {
		return
	}
	
	mock.interactionComplete = mock.interactionComplete || action.InteractionComplete
	
	mock.disconnectExpected = mock.disconnectExpected || action.DisconnectExpected
	
	if action.Messages == nil {
		return
	}
	
	if mock.msgQueue == nil {
		mock.msgQueue = action.Messages
		mock.queuePlace = 0
	}
}

func NewMockConnection(localAddr, remoteAddr net.Addr, report chan TestReport, peerTest PeerTest) *MockConnection{
	mock := &MockConnection{
		localAddr : localAddr, 
		remoteAddr : remoteAddr, 
		report : report,
		peerTest : peerTest,
		interactionComplete : false,
		disconnectExpected: false, 
		reply : make(chan []wire.Message),
	}
	
	mock.timer = time.AfterFunc(time.Millisecond * 100, func() {
		if !mock.interactionComplete {
			if mock.disconnectExpected {
				mock.Done(errors.New("Peer stopped interacting when it was expected to disconnect."))
			} else {
				mock.Done(errors.New("Peer stopped interacting (without disconnecting) before the interaction was complete."))
			}
		} else {
			mock.Done(nil)
		}
	})

	mock.Start()

	return mock
}

// MockListener implements the Listener interface
type MockListener struct {
	incoming   chan bmpeer.Connection
	disconnect chan struct{}
	localAddr  net.Addr
	disconnected bool
}

func (ml *MockListener) Accept() (bmpeer.Connection, error) {
	if ml.disconnected {
		return nil, errors.New("Listner disconnected.")
	}
	select {
	case <-ml.disconnect:
		return nil, errors.New("Listener disconnected.")
	case m := <-ml.incoming:
		return m, nil
	}
}

func (ml *MockListener) Close() error {
	ml.disconnect <- struct{}{}
	return nil
}

// Addr returns the listener's network address.
func (ml *MockListener) Addr() net.Addr {
	return ml.localAddr
}

func NewMockListener(localAddr net.Addr, incoming chan bmpeer.Connection, disconnect chan struct{}) *MockListener {
	return &MockListener{
		incoming : incoming,
		disconnect : disconnect, 
		localAddr : localAddr, 
	}
}

// MockListen returns a mock listener 
func MockListen(listeners []*MockListener) func(string, string) (bmpeer.Listener, error) {
	i := 0
	
	return func(service, addr string) (bmpeer.Listener, error) {
		i ++ 
		if i > len(listeners) {
			return nil, errors.New("No mock listeners remaining.")
		}
		
		return listeners[i-1], nil
	}
}

func msgToBytes(msg wire.Message) []byte {
	buf := &bytes.Buffer{}
	wire.WriteMessageN(buf, msg, wire.MainNet)
	return buf.Bytes()
}

func getMemDb(msgs []wire.Message) database.Db {
	database.AddDBDriver(database.DriverDB{DbType: "memdb", CreateDB: memdb.CreateDB, OpenDB: memdb.OpenDB})

	db, err := database.CreateDB("memdb")
	if err != nil {
		return nil
	}
	
	for _, msg := range msgs {
		data, _ := wire.EncodeMessage(msg)
		db.InsertObject(data)
	}
	
	return db
}

// TestOutboundPeerHandshake tests the initial handshake for an outbound peer, ie,
// the real peer initiates the connection. 
// Test cases:
//  * Respond to a version message with a verack and then with a version.
//  * Respond to a version message with a version and then a verack.
//  * Error case: respond to a version with something other than verack/version.
//  * Send two veracks. (not necessarily an error.)
func TestOutboundPeerHandshake(t *testing.T) {
	// A channel for the mock peer to communicate with the test.
	report := make(chan TestReport)
	
	streams := []uint32{1}
	nonce, _ := wire.RandomUint64()
	var addrin, addrout *wire.NetAddress = wire.NewNetAddressIPPort(net.IPv4(5, 45, 99, 75), 8444, 1, 0),
		wire.NewNetAddressIPPort(net.IPv4(5, 45, 99, 75), 8444, 1, 0)
	msgAddr := wire.NewMsgAddr()
	msgAddr.AddAddress(wire.NewNetAddressIPPort(net.IPv4(5, 45, 99, 75), 8444, 1, 0))
		
	responses := []*PeerAction{
		// Two possible ways of ordering the responses that are both valid. 
		&PeerAction {
			Messages : []wire.Message{&wire.MsgVerAck{}, wire.NewMsgVersion(addrin, addrout, nonce, streams)}, 
		}, 
		&PeerAction {
			Messages : []wire.Message{wire.NewMsgVersion(addrin, addrout, nonce, streams), &wire.MsgVerAck{}}, 
		}, 
		// Extra VerAcks are also ok. 
		&PeerAction {
			Messages : []wire.Message{&wire.MsgVerAck{}, wire.NewMsgVersion(addrin, addrout, nonce, streams), &wire.MsgVerAck{}}, 
		}, 
		&PeerAction {
			Messages : []wire.Message{&wire.MsgVerAck{}, &wire.MsgVerAck{}, wire.NewMsgVersion(addrin, addrout, nonce, streams)}, 
		}, 
		// Send a message that is not allowed at this time. Should result in a disconnect.
		&PeerAction{
			Messages : []wire.Message{msgAddr},
			InteractionComplete : true,
			DisconnectExpected : true, 
		}, 
	}
	
	localAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8333}
	remoteAddr := &net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 8333}

	// A peer that establishes a handshake for outgoing peers.
	handshakePeerBuilder := func(action *PeerAction) func(string, string) (bmpeer.Connection, error) {
		return func(service, addr string) (bmpeer.Connection, error) {
			return NewMockConnection(localAddr, remoteAddr, report, 
					NewOutboundHandshakePeerTester(action, msgAddr)), nil
		}
	}
	
	for test_case, response := range responses {
		Dial = handshakePeerBuilder(response)

		// Create server and start it.
		listeners := []string{net.JoinHostPort("", "8445")}
		serv, err := TstNewServer(listeners, getMemDb([]wire.Message{}),
			MockListen([]*MockListener{
				NewMockListener(localAddr, make(chan bmpeer.Connection), make(chan struct{}))}))
		if err != nil {
			t.Fatal("Server failed to start: %s", err)
		}
		serv.TstStart([]*DefaultPeer{&DefaultPeer{"5.45.99.75:8444", 1, true}})

		go func() {
			msg := <-report
			if msg.Err != nil {
				t.Error(fmt.Sprintf("error case %d: %s", test_case, msg))
			}
			serv.Stop()
		}()

		serv.WaitForShutdown()
	}
}

// Test cases:
//  * Send a version message and get a verack and version message in return. 
//  * Error cases: open with a verack and some other kind of message.
//  * Error case: respond to a verack with something other than verack/version.
//  * Error case: send two versions.
//  * Erorr case: send a version message with a version < 3. 
//  * Send a version message with a version higher than three. The peer should not disconnect.
func TestInboundPeerHandshake(t *testing.T) {
	// A channel for the mock peer to communicate with the test.
	report := make(chan TestReport)
	// A channel to make the fake incomming connection.
	incoming := make(chan bmpeer.Connection)

	streams := []uint32{1}
	nonce, _ := wire.RandomUint64()
	var addrin, addrout *wire.NetAddress = wire.NewNetAddressIPPort(net.IPv4(5, 45, 99, 75), 8444, 1, 0),
		wire.NewNetAddressIPPort(net.IPv4(5, 45, 99, 75), 8444, 1, 0)

	localAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8333}
	remoteAddr := &net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 8333}

	msgAddr := wire.NewMsgAddr()
	msgAddr.AddAddress(wire.NewNetAddressIPPort(net.IPv4(5, 45, 99, 75), 8444, 1, 0))
	
	addrAction := &PeerAction{
		Messages : []wire.Message{msgAddr}, 
		InteractionComplete : true, 
		DisconnectExpected : false, 
	}
	
	incorrectVersion := wire.NewMsgVersion(addrin, addrout, nonce, streams)
	incorrectVersion.ProtocolVersion = int32(2)
	
	futureVersion := wire.NewMsgVersion(addrin, addrout, nonce, streams)
	futureVersion.ProtocolVersion = int32(4)

	//The four test cases are all in this list. 
	openingMsg := []*PeerAction{
		&PeerAction {
			Messages : []wire.Message{wire.NewMsgVersion(addrin, addrout, nonce, streams)},
		}, 
		&PeerAction {
			Messages : []wire.Message{&wire.MsgVerAck{}},
			InteractionComplete : true,
			DisconnectExpected : true,
		}, 
		&PeerAction {
			Messages : []wire.Message{msgAddr},
			InteractionComplete : true,
			DisconnectExpected : true,
		}, 
		&PeerAction {
			Messages : []wire.Message{wire.NewMsgVersion(addrin, addrout, nonce, streams), wire.NewMsgVersion(addrin, addrout, nonce, streams)},
			InteractionComplete : true,
			DisconnectExpected : true,
		}, 
		&PeerAction {
			Messages : []wire.Message{incorrectVersion},
			InteractionComplete : true,
			DisconnectExpected : true,
		}, 
		&PeerAction {
			Messages : []wire.Message{futureVersion},
			InteractionComplete : true,
			DisconnectExpected : false,
		}, 
	}

	for test_case, open := range openingMsg {		
		// Create server and start it.
		listeners := []string{net.JoinHostPort("", "8445")}
		var err error
		serv, err := TstNewServer(listeners, getMemDb([]wire.Message{}),
			MockListen([]*MockListener{
				NewMockListener(localAddr, make(chan bmpeer.Connection), make(chan struct{}))}))
		if err != nil {
			t.Fatal("Server failed to start: %s", err)
		}
		serv.TstStart([]*DefaultPeer{})

		// Test handshake.
		incoming <- NewMockConnection(localAddr, remoteAddr, report, 
			NewInboundHandshakePeerTester(open, addrAction))
	
		go func() {
			msg := <-report
			if msg.Err != nil {
				t.Error(fmt.Sprintf("error case %d: %s", test_case, msg))
			}
			serv.Stop()
		}()
	
		serv.WaitForShutdown()
	}
}

// Test cases
//  * after a successful handshake, get an addr and receive one. 
//  * error case: send an addr message that is too big. 
//  * Give the peer no addresses to send. 
func TestProcessAddr(t *testing.T) {
	// Process a handshake. This should be an incoming peer.
	// Send an addr and receive an addr.
	// Ignore inv messages.

	// A channel for the mock peer to communicate with the test.
	report := make(chan TestReport)
	// A channel to make the fake incomming connection.
	incoming := make(chan bmpeer.Connection)

	srcAddr := &wire.NetAddress{
		Timestamp: time.Now(),
		Services:  wire.SFNodeNetwork,
		IP:        net.ParseIP("173.144.173.111"),
		Port:      8333,
	}

	// Some parameters for the test sequence. 
	streams := []uint32{1}
	nonce, _ := wire.RandomUint64()
	var addrin, addrout *wire.NetAddress = wire.NewNetAddressIPPort(net.IPv4(5, 45, 99, 75), 8444, 1, 0),
		wire.NewNetAddressIPPort(net.IPv4(5, 45, 99, 75), 8444, 1, 0)

	localAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8333}
	remoteAddr := &net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 8333}

	// Some addr messages to use for testing. 
	addrMsg := wire.NewMsgAddr()
	addrMsg.AddAddress(wire.NewNetAddressIPPort(net.IPv4(5, 45, 99, 75), 8444, 1, 0))
	// Maximum number of addresses allowed in an addr message is 1000, so we add 1001.
	addrMsgTooLong := wire.NewMsgAddr()
	for i := 0; i <= 1001; i++ {
			addrMsgTooLong.AddAddress(&wire.NetAddress{
			Timestamp: time.Now(),
			Services:  wire.SFNodeNetwork,
			IP:        net.IPv4(
				byte(rand.Intn(256)),
				byte(rand.Intn(256)),
				byte(rand.Intn(256)),
				byte(rand.Intn(256))),
			Port:      8333,
		})
	}

	AddrTests := []struct {
		AddrAction *PeerAction     // Action for the mock peer to take upon handshake completion. 
		NumAddrs   int             // Number of addresses to put in the address manager. 
	} {
		{
			&PeerAction{[]wire.Message{addrMsg}, nil, false, false},
			25, 
		}, 
		{
			&PeerAction{[]wire.Message{addrMsgTooLong}, nil, true, true},
			25, 
		},
		{
			&PeerAction{nil, nil, true, false},
			0, 
		},
	}

	for test_case, addrTest := range AddrTests {
		// Add some addresses to the address manager. 
		addrs := make([]*wire.NetAddress, addrTest.NumAddrs)
		
		// Create server and start it.
		listeners := []string{net.JoinHostPort("", "8445")}
		serv, err := TstNewServer(listeners, getMemDb([]wire.Message{}),
			MockListen([]*MockListener{
				NewMockListener(localAddr, make(chan bmpeer.Connection), make(chan struct{}))}))
		if err != nil {
			t.Fatal("Server failed to start.")
		}
		serv.TstStart([]*DefaultPeer{})
	
		for i := 0; i < addrTest.NumAddrs; i++ {
			s := fmt.Sprintf("%d.173.147.%d:8333", i/64+60, i%64+60)
			addrs[i], _ = serv.addrManager.DeserializeNetAddress(s)
		}
		serv.addrManager.AddAddresses(addrs, srcAddr)

		incoming <- NewMockConnection(localAddr, remoteAddr, report, 
			NewInboundHandshakePeerTester(
					&PeerAction{Messages : []wire.Message{wire.NewMsgVersion(addrin, addrout, nonce, streams)}},
					addrTest.AddrAction))

		go func() {
			msg := <-report
			if msg.Err != nil {
				t.Error(fmt.Sprintf("error case %d: %s", test_case, msg))
			}
			serv.Stop()
		}()
	
		serv.WaitForShutdown()
	}
}

var expires = time.Now().Add(10 * time.Minute)
var expired = time.Now().Add(-10 * time.Minute).Add(-3 * time.Hour)

// A set of pub keys to create fake objects for testing the database.
var pubkey = []wire.PubKey{
	wire.PubKey([wire.PubKeySize]byte{
		23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38,
		39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54,
		55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 70,
		71, 72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85, 86}),
	wire.PubKey([wire.PubKeySize]byte{
		87, 88, 89, 90, 91, 92, 93, 94, 95, 96, 97, 98, 99, 100, 101, 102,
		103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118,
		119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 129, 130, 131, 132, 133, 134,
		135, 136, 137, 138, 139, 140, 141, 142, 143, 144, 145, 146, 147, 148, 149, 150}),
	wire.PubKey([wire.PubKeySize]byte{
		54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69,
		70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85,
		86, 87, 88, 89, 90, 91, 92, 93, 94, 95, 96, 97, 98, 99, 100, 101,
		102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117}),
	wire.PubKey([wire.PubKeySize]byte{
		118, 119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 129, 130, 131, 132, 133,
		134, 135, 136, 137, 138, 139, 140, 141, 142, 143, 144, 145, 146, 147, 148, 149,
		150, 151, 152, 153, 154, 155, 156, 157, 158, 159, 160, 161, 162, 163, 164, 165,
		166, 167, 168, 169, 170, 171, 172, 173, 174, 175, 176, 177, 178, 179, 180, 181}),
}

var shahash = []wire.ShaHash{
	wire.ShaHash([wire.HashSize]byte{
		98, 99, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113,
		114, 115, 116, 117, 118, 119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 129}),
	wire.ShaHash([wire.HashSize]byte{
		100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115,
		116, 117, 118, 119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 129, 130, 131}),
}

var ripehash = []wire.RipeHash{
	wire.RipeHash([wire.RipeHashSize]byte{
		78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 91, 92, 93, 94, 95, 96, 97}),
	wire.RipeHash([wire.RipeHashSize]byte{
		80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 91, 92, 93, 94, 95, 96, 97, 98, 99}),
}

// Some bitmessage objects that we use for testing. Two of each.
var testObj = []wire.Message{
	wire.NewMsgGetPubKey(654, expires, 4, 1, &ripehash[0], &shahash[0]),
	wire.NewMsgGetPubKey(654, expired, 4, 1, &ripehash[1], &shahash[1]),
	wire.NewMsgPubKey(543, expires, 4, 1, 2, &pubkey[0], &pubkey[1], 3, 5,
		[]byte{4, 5, 6, 7, 8, 9, 10}, &shahash[0], []byte{11, 12, 13, 14, 15, 16, 17, 18}),
	wire.NewMsgPubKey(543, expired, 4, 1, 2, &pubkey[2], &pubkey[3], 3, 5,
		[]byte{4, 5, 6, 7, 8, 9, 10}, &shahash[1], []byte{11, 12, 13, 14, 15, 16, 17, 18}),
	wire.NewMsgMsg(765, expires, 1, 1,
		[]byte{90, 87, 66, 45, 3, 2, 120, 101, 78, 78, 78, 7, 85, 55, 2, 23},
		1, 1, 2, &pubkey[0], &pubkey[1], 3, 5, &ripehash[0], 1,
		[]byte{21, 22, 23, 24, 25, 26, 27, 28},
		[]byte{20, 21, 22, 23, 24, 25, 26, 27},
		[]byte{19, 20, 21, 22, 23, 24, 25, 26}),
	wire.NewMsgMsg(765, expired, 1, 1,
		[]byte{90, 87, 66, 45, 3, 2, 120, 101, 78, 78, 78, 7, 85, 55},
		1, 1, 2, &pubkey[2], &pubkey[3], 3, 5, &ripehash[1], 1,
		[]byte{21, 22, 23, 24, 25, 26, 27, 28, 79},
		[]byte{20, 21, 22, 23, 24, 25, 26, 27, 79},
		[]byte{19, 20, 21, 22, 23, 24, 25, 26, 79}),
	wire.NewMsgBroadcast(876, expires, 1, 1, &shahash[0],
		[]byte{90, 87, 66, 45, 3, 2, 120, 101, 78, 78, 78, 7, 85, 55, 2, 23},
		1, 1, 2, &pubkey[0], &pubkey[1], 3, 5, &ripehash[1], 1,
		[]byte{27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41},
		[]byte{42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56}),
	wire.NewMsgBroadcast(876, expired, 1, 1, &shahash[1],
		[]byte{90, 87, 66, 45, 3, 2, 120, 101, 78, 78, 78, 7, 85, 55},
		1, 1, 2, &pubkey[2], &pubkey[3], 3, 5, &ripehash[0], 1,
		[]byte{27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40},
		[]byte{42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55}),
	wire.NewMsgUnknownObject(345, expires, wire.ObjectType(4), 1, 1, []byte{77, 82, 53, 48, 96, 1}),
	wire.NewMsgUnknownObject(987, expired, wire.ObjectType(4), 1, 1, []byte{1, 2, 3, 4, 5, 0, 6, 7, 8, 9, 100}),
	wire.NewMsgUnknownObject(7288, expires, wire.ObjectType(5), 1, 1, []byte{0, 0, 0, 0, 1, 0, 0}),
	wire.NewMsgUnknownObject(7288, expired, wire.ObjectType(5), 1, 1, []byte{0, 0, 0, 0, 0, 0, 0, 99, 98, 97}),
}

func randomShaHash() *wire.ShaHash {
	b := make([]byte, 32)
	for i := 0; i < 32; i ++ {
		b[i] = byte(rand.Intn(256))
	}
	hash, _ := wire.NewShaHash(b)
	return hash
}

// Test cases
//  * after a successful handshake, get an inv and receive one. request an object
//    from the peer and receive a request for something that the mock peer has. Send
//    and receive responses for the requests. Several cases of this scenario:
//     * The peer already has everything that the mock peer has. (no inv expected.)
//     * The peer has some of what the mock peer has, but not everything. 
//     * The peer needs to send more than one getData request. 
//  * error case: send an inv message that is too big. (peer should disconnect)
//  * error case: send a request for an object that the peer does not have. (peer should not disconnect). 
//  * error case: return an object that was not requested. (peer should disconnect)
/*func TestProcessInvAndObjectExchange(t *testing.T) {
	// Process a handshake.
	// Send an inv and receive an inv.
	// Process a request for an object.
	// Send a request and receive an object.
	// Ignore addr messages.

	tooLongInvVect := make([]*wire.InvVect, wire.MaxInvPerMsg + 1)
	for i := 0; i < wire.MaxInvPerMsg + 1; i++ {
		tooLongInvVect[i] = &wire.InvVect{*randomShaHash()}
	}
	TooLongInv := &wire.MsgInv{tooLongInvVect}
	
	tests := []struct {
		peerDB []wire.Message // The messages already in the peer's db. 
		mockDB []wire.Message // The messages that are already in the 
		invAction *PeerAction // Action for the mock peer to take upon receiving an inv.
	} {
		{
			[]wire.Message{},
			[]wire.Message{testObj[0], testObj[2], testObj[4], testObj[6], testObj[8]},
			nil, 
		},
		{
			[]wire.Message{testObj[0], testObj[2], testObj[4], testObj[6], testObj[8]},
			[]wire.Message{testObj[0], testObj[2], testObj[4], testObj[6], testObj[8]},
			nil, 
		},
		{
			[]wire.Message{testObj[1], testObj[3], testObj[5], testObj[7], testObj[9]},
			[]wire.Message{testObj[0], testObj[2], testObj[4], testObj[6], testObj[8]},
			nil, 
		},
		{
			[]wire.Message{testObj[0], testObj[3], testObj[5], testObj[7], testObj[9]},
			[]wire.Message{testObj[0], testObj[2], testObj[4], testObj[6], testObj[8]},
			nil, 
		},
		{
			[]wire.Message{testObj[0], testObj[2], testObj[4], testObj[6], testObj[8]},
			[]wire.Message{},
			nil, 
		},
		{
			[]wire.Message{}, 
			[]wire.Message{},
			&PeerAction{Messages: []wire.Message{TooLongInv}, DisconnectExpected: true, InteractionComplete: true}, 
		},
	}

	// A channel for the mock peer to communicate with the test.
	report := make(chan TestReport)
	// A channel to make the fake incomming connection.
	incoming := make(chan bmpeer.Connection)
	
	// Some parameters for the test sequence. 
	streams := []uint32{1}
	nonce, _ := wire.RandomUint64()
	var addrin, addrout *wire.NetAddress = wire.NewNetAddressIPPort(net.IPv4(5, 45, 99, 75), 8444, 1, 0),
		wire.NewNetAddressIPPort(net.IPv4(5, 45, 99, 75), 8444, 1, 0)
	
	localAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8333}
	remoteAddr := &net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 8333}

	for test_case, test := range tests {
		// Define the objects that will go in the database. 
		// Create server and start it.
		listeners := []string{net.JoinHostPort("", "8445")}
		serv, err = TstNewServer(listeners, getMemDb([]wire.Message{}),
			MockListen([]*MockListener{
				NewMockListener(localAddr, make(chan bmpeer.Connection), make(chan struct{}))}))
		if err != nil {
			t.Fatal("Server failed to start.")
		}
		serv.TstStart([]DefaultPeer{})

		// Test handshake.
		incoming <- &mockBitmessageConn{
			remoteAddr: tcpAddrYou,
			localAddr:  tcpAddrMe,
			
			mockPeer: NewMockBitmessagePeer(report, serv, 
				NewDataExchangePeerTester(
					&PeerAction{messages : []wire.Message{wire.NewMsgVersion(addrin, addrout, nonce, streams)}}, 
					test.mockDB, test.peerDB, test.invAction), 
			),
		}
		
		go func() {
			msg := <-report
			if msg.Err != nil {
				t.Error(fmt.Sprintf("error case %d: %s", test_case, msg))
			}
			serv.Stop()
		}()
	
		serv.WaitForShutdown()
	}
}*/
