// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/base64"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/rpc2"
	"github.com/cenkalti/rpc2/jsonrpc"
	"github.com/gorilla/websocket"
	"github.com/monetas/bmd/peer"
	"github.com/monetas/bmutil/wire"
)

const (
	rpcLoc = "ws://localhost:8442/"

	rpcAdminUser = "admin"
	rpcAdminPass = "admin"

	rpcLimitUser = "limit"
	rpcLimitPass = "limit"
)

var serv *server

var subscribeArgs = &RPCSubscribeArgs{
	FromCounter: 0,
}

func rpcTests(client *rpc2.Client, t *testing.T) {
	// Do not change test order. Tests depend on previous tests successfully
	// finishing.
	testRPCAuth(client, t)
	testRPCSendObject(client, t)
	testRPCSubscriptions(client, t)
}

// testRPCAuth tests authentication failures for all RPC methods and also
// Authenticate method.
func testRPCAuth(client *rpc2.Client, t *testing.T) {
	// Test access denied.
	failTests := []struct {
		method string
		args   interface{}
	}{
		{rpcHandleSendObject, "Y="},
		{rpcHandleGetIdentity, "BM-asd5s"},
		{rpcHandleSubscribeMessages, subscribeArgs},
		{rpcHandleSubscribeBroadcasts, subscribeArgs},
		{rpcHandleSubscribeGetpubkeys, subscribeArgs},
		{rpcHandleSubscribePubkeys, subscribeArgs},
		{rpcHandleSubscribeUnknownObjs, subscribeArgs},
	}

	for _, test := range failTests {
		err := client.Call(test.method, test.args, nil)
		if err.Error() != errAccessDenied.Error() { // auth failure
			t.Errorf("for %s expected %v got %v", test.method, errAccessDenied,
				err)
		}
	}

	// Test Authenticate.
	authTests := []struct {
		username string
		password string
		success  bool
	}{
		{"", "", false},
		{"sadsadadoijsad", "asdfsafafdfasdfdf", false},
		{rpcLimitUser, rpcLimitPass, true},
		{rpcAdminUser, rpcAdminPass, true},
	}

	for i, test := range authTests {
		args := &RPCAuthArgs{
			Username: test.username,
			Password: test.password,
		}
		var res bool
		err := client.Call(rpcHandleAuth, args, &res)

		if err != nil {
			t.Errorf("for case #%d got error %v", i, err)
		}
		if test.success != res {
			t.Errorf("for case #%d expected %v got %v", i, test.success, res)
		}
	}
}

// Test SendObject.
func testRPCSendObject(client *rpc2.Client, t *testing.T) {

	// invalid base64 encoding
	err := client.Call(rpcHandleSendObject, "aodisad093predikif", nil)
	if err == nil {
		t.Error("got no error for invalid base64 encoding")
	}

	errorTests := [][]byte{
		[]byte{},                       // empty
		[]byte{0x00, 0x00, 0x00, 0x00}, // invalid object
		[]byte{ // expired
			0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0xe0, 0xf3, // Nonce
			0x00, 0x00, 0x00, 0x00, 0x49, 0x5f, 0xab, 0x29, // 64-bit Timestamp
			0x00, 0x00, 0x00, 0x00, // object type
			0x03, // Version
			0x01, // Stream Number
		},
		[]byte{ // invalid stream
			0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0xe0, 0xf3, // Nonce
			0x00, 0x00, 0x00, 0xff, 0x49, 0x5f, 0xab, 0x29, // 64-bit Timestamp
			0x00, 0x00, 0x00, 0x00, // object type
			0x03, // Version
			0x05, // Stream Number
		},
		[]byte{ // invalid PoW
			0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0xe0, 0xf3, // Nonce
			0x00, 0x00, 0x00, 0xff, 0x49, 0x5f, 0xab, 0x29, // 64-bit Timestamp
			0x00, 0x00, 0x00, 0x00, // object type
			0x03, // Version
			0x01, // Stream Number
		},
	}

	for i, test := range errorTests {
		err = client.Call(rpcHandleSendObject,
			base64.StdEncoding.EncodeToString(test), nil)
		if err == nil {
			t.Errorf("for case #%d got no error", i)
		}
	}

	// Insert valid object.
	data := wire.EncodeMessage(testObj[0]) // getpubkey
	err = client.Call(rpcHandleSendObject,
		base64.StdEncoding.EncodeToString(data), nil)
	if err != nil {
		t.Errorf("for valid SendObject got error %v", err)
	}
	hash := testObj[0].InventoryHash()

	// Check if advertised.
	if ok, err := serv.objectManager.haveInventory(wire.NewInvVect(hash)); !ok {
		t.Error("server doesn't have new object in inventory, error:", err)
	}
}

func testRPCSubscriptions(client *rpc2.Client, t *testing.T) {
	// Test if old objects are sent OK when subscribing.
	var received int32

	// Receive Getpubkey inserted in previous test.
	client.Handle(rpcClientHandleGetpubkey, func(client *rpc2.Client,
		args *RPCReceiveArgs, _ *struct{}) error {
		b, err := base64.StdEncoding.DecodeString(args.Object)
		if err != nil {
			t.Fatal("invalid base64 encoding")
		}
		data := wire.EncodeMessage(testObj[0]) // getpubkey
		if !bytes.Equal(data, b) {
			t.Errorf("invalid getpubkey bytes, expected %v got %v", data, b)
		}
		t.Logf("Received getpubkey: counter=%d, byte length=%d\n", args.Counter,
			len(b))
		atomic.StoreInt32(&received, 1)
		return nil
	})

	err := client.Call(rpcHandleSubscribeGetpubkeys, subscribeArgs, nil)
	if err != nil {
		t.Error("SubscribeGetpubkeys failed: ", err)
	}

	// Wait for received==1
	timer := time.NewTimer(time.Millisecond * 20)
	<-timer.C

	if atomic.LoadInt32(&received) != 1 {
		t.Error("did not receive getpubkey from sendOldObjects")
	}

	// Test if objects inserted after subscribing are handled correctly.
	atomic.StoreInt32(&received, 0)
	data := wire.EncodeMessage(testObj[2]) // pubkey

	client.Handle(rpcClientHandlePubkey, func(client *rpc2.Client,
		args *RPCReceiveArgs, _ *struct{}) error {
		b, err := base64.StdEncoding.DecodeString(args.Object)
		if err != nil {
			t.Fatal("invalid base64 encoding")
		}
		if !bytes.Equal(data, b) {
			t.Errorf("invalid pubkey bytes, expected %v got %v", data, b)
		}
		t.Logf("Received pubkey: counter=%d, byte length=%d\n", args.Counter,
			len(b))
		atomic.StoreInt32(&received, 1)
		return nil
	})

	err = client.Call(rpcHandleSubscribePubkeys, subscribeArgs, nil)
	if err != nil {
		t.Error("SubscribePubkeys failed: ", err)
	}

	err = client.Call(rpcHandleSendObject,
		base64.StdEncoding.EncodeToString(data), nil)
	if err != nil {
		t.Errorf("for valid SendObject got error %v", err)
	}

	timer.Reset(time.Millisecond * 20)
	<-timer.C
	timer.Stop()

	if atomic.LoadInt32(&received) != 1 {
		t.Error("did not receive pubkey from handleSubscribe")
	}
}

func TestRPCConnection(t *testing.T) {
	// Address for mock listener to pass to server. The server
	// needs at least one listener or it won't start so we mock it.
	remoteAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8442}

	// Generate config.
	var err error
	cfg, _, err = loadConfig(true)
	if err != nil {
		t.Fatal("failed to load config")
	}
	cfg.MaxPeers = 0
	cfg.RPCPass = rpcAdminUser
	cfg.RPCUser = rpcAdminPass
	cfg.RPCLimitUser = rpcLimitUser
	cfg.RPCLimitPass = rpcLimitPass
	cfg.DisableRPC = false
	cfg.DisableTLS = true
	cfg.RPCMaxClients = 1
	cfg.DisableDNSSeed = true
	defer resetCfg(cfg)()

	// Load rpc listeners.
	addrs, err := net.LookupHost("localhost")
	if err != nil {
		t.Fatal("Could not look up localhost.")
	}
	cfg.RPCListeners = make([]string, 0, len(addrs))
	for _, addr := range addrs {
		addr = net.JoinHostPort(addr, strconv.Itoa(defaultRPCPort))
		cfg.RPCListeners = append(cfg.RPCListeners, addr)
	}
	defer backendLog.Flush()

	// Create a server.
	listeners := []string{net.JoinHostPort("", "8445")}
	serv, err = newServer(listeners, getMemDb([]*wire.MsgObject{}),
		MockListen([]*MockListener{
			NewMockListener(remoteAddr, make(chan peer.Connection), make(chan struct{}, 1))}))

	if err != nil {
		t.Fatalf("Server creation failed: %s", err)
	}

	serv.Start()

	ws, _, err := websocket.DefaultDialer.Dial(rpcLoc, nil)
	if err != nil {
		t.Fatal(err)
	}

	client := rpc2.NewClientWithCodec(jsonrpc.NewJSONCodec(ws.UnderlyingConn()))

	go client.Run()

	rpcTests(client, t)
	client.Close() // we're done
	ws.Close()

	// Test for authentication timeout.
	ws, _, err = websocket.DefaultDialer.Dial(rpcLoc, nil)
	if err != nil {
		t.Fatal(err)
	}
	client = rpc2.NewClientWithCodec(jsonrpc.NewJSONCodec(ws.UnderlyingConn()))
	go client.Run()

	// wait, give 10ms wiggling room
	<-time.NewTimer(time.Second*rpcAuthTimeoutSeconds + time.Millisecond*10).C

	select {
	case <-client.DisconnectNotify():
	// do nothing
	default:
		t.Error("did not disconnect due to auth timeout")
	}

	// Test for RPCMaxClients
	ws, _, err = websocket.DefaultDialer.Dial(rpcLoc, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = websocket.DefaultDialer.Dial(rpcLoc, nil)
	if err == nil {
		t.Error("RPCMaxClients isn't enforced. Second connection was successful.")
	}

	// Cleanup.
	serv.Stop()
	serv.WaitForShutdown()
}
