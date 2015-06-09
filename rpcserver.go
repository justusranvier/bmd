// Originally derived from: btcsuite/btcd/rpcserver.go
// Copyright (c) 2013-2015 The btcsuite developers.

// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io/ioutil"
	prand "math/rand"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcutil"
	"github.com/cenkalti/rpc2"
	"github.com/cenkalti/rpc2/jsonrpc"
	"github.com/gorilla/websocket"
	"github.com/ishbir/eventemitter"
	"github.com/monetas/bmutil/wire"
)

const (
	// rpcAuthTimeoutSeconds is the number of seconds a connection to the
	// RPC server is allowed to stay open without authenticating before it
	// is closed.
	rpcAuthTimeoutSeconds = 5

	// rpcCounterObjectsSize is the number of objects that db.FetchObjectsFromCounter
	// will fetch per query to the database. This is used when a client requests
	// subscription to an object type from a specified counter value.
	rpcCounterObjectsSize = 100
)

const (
	// RPC server local new object event handlers.
	rpcEvtNewMessage    = "newMessage"
	rpcEvtNewBroadcast  = "newBroadcast"
	rpcEvtNewGetpubkey  = "newGetpubkey"
	rpcEvtNewPubkey     = "newPubkey"
	rpcEvtNewUnknownObj = "newUnknownObject"

	// Methods defined on RPC server
	rpcHandleAuth        = "Authenticate"
	rpcHandleSendObject  = "SendObject"
	rpcHandleGetIdentity = "GetIdentity"

	rpcSubscribePrefix            = "Subscribe"
	rpcHandleSubscribeMessages    = rpcSubscribePrefix + "Messages"
	rpcHandleSubscribeBroadcasts  = rpcSubscribePrefix + "Broadcasts"
	rpcHandleSubscribeGetpubkeys  = rpcSubscribePrefix + "Getpubkeys"
	rpcHandleSubscribePubkeys     = rpcSubscribePrefix + "Pubkeys"
	rpcHandleSubscribeUnknownObjs = rpcSubscribePrefix + "UnknownObjects"

	// Methods defined on RPC client
	rpcClientObjectHandlePrefix = "Receive"
	rpcClientHandleMessage      = rpcClientObjectHandlePrefix + "Message"
	rpcClientHandleBroadcast    = rpcClientObjectHandlePrefix + "Broadcast"
	rpcClientHandleGetpubkey    = rpcClientObjectHandlePrefix + "Getpubkey"
	rpcClientHandlePubkey       = rpcClientObjectHandlePrefix + "Pubkey"
	rpcClientHandleUnknownObj   = rpcClientObjectHandlePrefix + "UnknownObject"

	// Various states contained in client.State
	rpcStateRemoteAddr      = "remoteAddr"      // string
	rpcStateIsAuthenticated = "isAuthenticated" // bool
	rpcStateIsAdmin         = "isAdmin"         // bool
	rpcStateEventsID        = "eventsID"        // int
)

var (
	// upgrader upgrades a normal HTTP connection to websocket.
	upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
	}

	// errAccessDenied is the error sent to the client when it tries to connect
	// to a RPC method without having authenticated.
	errAccessDenied = errors.New("access denied")
)

// rpcClientParams holds items that are relevant to a connected client.
type rpcClientState struct {
	remoteAddr      string
	isAuthenticated bool
	isAdmin         bool
	// a psuedorandom value used to associate client with subscribed events
	eventsID int
}

// constructState constructs a rpcClientState object for a given client.
func rpcConstructState(client *rpc2.Client) *rpcClientState {
	state := new(rpcClientState)

	r, _ := client.State.Get(rpcStateRemoteAddr)
	state.remoteAddr = r.(string)

	ev, _ := client.State.Get(rpcStateEventsID)
	state.eventsID = ev.(int)

	isAuth, _ := client.State.Get(rpcStateIsAuthenticated)
	state.isAuthenticated = isAuth.(bool)

	isAdmin, _ := client.State.Get(rpcStateIsAdmin)
	state.isAdmin = isAdmin.(bool)

	return state
}

// rpcServer holds the items the rpc server may need to access (config,
// shutdown, main server, etc.)
type rpcServer struct {
	server       *server
	rpcSrv       *rpc2.Server
	listeners    []net.Listener
	evtMgr       *eventemitter.EventEmitter
	limitauthsha [sha256.Size]byte
	authsha      [sha256.Size]byte
	mutex        sync.RWMutex
	clients      map[*rpc2.Client]struct{}
	started      int32
	shutdown     int32
	wg           sync.WaitGroup
	quit         chan int
}

// addHandlers is responsible for adding RPC method handlers to the underlying
// RPC server. Moreover, it also adds event handlers to the event manager so
// that events sent by bmd (like new broadcast/message/pubkey) or those from the
// RPC server can be handled (e.g. notifying any interested clients).
func (s *rpcServer) addHandlers() {
	// When client connects/disconnects
	s.rpcSrv.OnConnect(s.onClientConnect)
	s.rpcSrv.OnDisconnect(s.onClientDisconnect)

	// General
	s.rpcSrv.Handle(rpcHandleAuth, s.handleAuth)

	// Objects
	s.rpcSrv.Handle(rpcHandleSendObject, s.sendObject)
	s.rpcSrv.Handle(rpcHandleGetIdentity, s.getID)

	// Statistics

	// Notifications
	s.rpcSrv.Handle(rpcHandleSubscribeMessages, s.subscribeMessages)
	s.rpcSrv.Handle(rpcHandleSubscribeBroadcasts, s.subscribeBroadcasts)
	s.rpcSrv.Handle(rpcHandleSubscribeGetpubkeys, s.subscribeGetpubkeys)
	s.rpcSrv.Handle(rpcHandleSubscribePubkeys, s.subscribePubkeys)
	s.rpcSrv.Handle(rpcHandleSubscribeUnknownObjs, s.subscribeUnknownObjects)

}

// onClientConnect is run for each client that connects to the RPC server.
func (s *rpcServer) onClientConnect(client *rpc2.Client) {
	s.mutex.Lock()
	s.clients[client] = struct{}{}
	s.mutex.Unlock()

	// Enforce no authentication timeout.
	go func() {
		timer := time.NewTimer(time.Second * rpcAuthTimeoutSeconds)
		<-timer.C

		if isAuth, _ := client.State.Get(rpcStateIsAuthenticated); !isAuth.(bool) {
			client.Close() // bad client
		}
	}()

	state := rpcConstructState(client)
	rpcLog.Infof("Client %s connected", state.remoteAddr)
}

// onClientDisconnect is run for each client that disconnects from the RPC server.
func (s *rpcServer) onClientDisconnect(client *rpc2.Client) {
	s.mutex.Lock()
	delete(s.clients, client)
	s.mutex.Unlock()

	state := rpcConstructState(client)

	// De-register all event handlers.
	id := state.eventsID

	s.evtMgr.RemoveListener(rpcEvtNewMessage, id)
	s.evtMgr.RemoveListener(rpcEvtNewBroadcast, id)
	s.evtMgr.RemoveListener(rpcEvtNewGetpubkey, id)
	s.evtMgr.RemoveListener(rpcEvtNewPubkey, id)
	s.evtMgr.RemoveListener(rpcEvtNewUnknownObj, id)

	rpcLog.Infof("Client %s disconnected", state.remoteAddr)
}

// genCertPair generates a key/cert pair to the paths provided.
func genCertPair(certFile, keyFile string) error {
	rpcLog.Infof("Generating TLS certificates...")

	org := "bmd autogenerated cert"
	validUntil := time.Now().Add(10 * 365 * 24 * time.Hour)
	cert, key, err := btcutil.NewTLSCertPair(org, validUntil, nil)
	if err != nil {
		return err
	}

	// Write cert and key files.
	if err = ioutil.WriteFile(certFile, cert, 0666); err != nil {
		return err
	}
	if err = ioutil.WriteFile(keyFile, key, 0600); err != nil {
		os.Remove(certFile)
		return err
	}

	rpcLog.Infof("Done generating TLS certificates")
	return nil
}

// limitConnections responds with a 503 service unavailable and returns true if
// adding another client would exceed the maximum allowed RPC clients.
//
// This function is safe for concurrent access.
func (s *rpcServer) limitConnections(w http.ResponseWriter, remoteAddr string) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if int(len(s.clients)+1) > cfg.RPCMaxClients {
		rpcLog.Infof("Max RPC clients exceeded [%d] - disconnecting client %s",
			cfg.RPCMaxClients, remoteAddr)
		http.Error(w, "503 Too busy. Try again later.",
			http.StatusServiceUnavailable)
		return true
	}
	return false
}

// restrictAuth restricts access of the client, returning an error if the client
// is not already authenticated.
func (s *rpcServer) restrictAuth(client *rpc2.Client) error {
	state := rpcConstructState(client)
	if !state.isAuthenticated {
		return errAccessDenied
	}
	return nil
}

// restrictAdmin restricts access of the client, returning an error if the
// client is not already authenticated as an admin.
func (s *rpcServer) restrictAdmin(client *rpc2.Client) error {
	state := rpcConstructState(client)
	if !state.isAdmin {
		return errAccessDenied
	}
	return nil
}

// NotifyObject is used to notify the RPC server of any new objects so that it
// can send those onwards to the client.
func (s *rpcServer) NotifyObject(msg *wire.MsgObject, counter uint64) {
	out := &RPCReceiveArgs{
		Object:  base64.StdEncoding.EncodeToString(wire.EncodeMessage(msg)),
		Counter: counter,
	}

	switch msg.ObjectType {
	case wire.ObjectTypeBroadcast:
		s.evtMgr.Emit(rpcEvtNewBroadcast, out)
	case wire.ObjectTypeGetPubKey:
		s.evtMgr.Emit(rpcEvtNewGetpubkey, out)
	case wire.ObjectTypeMsg:
		s.evtMgr.Emit(rpcEvtNewMessage, out)
	case wire.ObjectTypePubKey:
		s.evtMgr.Emit(rpcEvtNewPubkey, out)
	default:
		s.evtMgr.Emit(rpcEvtNewUnknownObj, out)
	}
}

// newRPCServer returns a new instance of the rpcServer struct.
func newRPCServer(listenAddrs []string, s *server) (*rpcServer, error) {
	rpc := rpcServer{
		server:  s,
		rpcSrv:  rpc2.NewServer(),   // Create the underlying RPC server.
		evtMgr:  eventemitter.New(), // Event manager.
		quit:    make(chan int),
		clients: make(map[*rpc2.Client]struct{}),
	}

	if cfg.RPCUser != "" && cfg.RPCPass != "" {
		login := cfg.RPCUser + ":" + cfg.RPCPass
		rpc.authsha = sha256.Sum256([]byte(login))
	}
	if cfg.RPCLimitUser != "" && cfg.RPCLimitPass != "" {
		login := cfg.RPCLimitUser + ":" + cfg.RPCLimitPass
		rpc.limitauthsha = sha256.Sum256([]byte(login))
	}

	// Setup TLS if not disabled.
	listenFunc := net.Listen
	if !cfg.DisableTLS {
		// Generate the TLS cert and key file if both don't already
		// exist.
		if !fileExists(cfg.RPCKey) && !fileExists(cfg.RPCCert) {
			err := genCertPair(cfg.RPCCert, cfg.RPCKey)
			if err != nil {
				return nil, err
			}
		}
		keypair, err := tls.LoadX509KeyPair(cfg.RPCCert, cfg.RPCKey)
		if err != nil {
			return nil, err
		}

		tlsConfig := tls.Config{
			Certificates: []tls.Certificate{keypair},
			MinVersion:   tls.VersionTLS12,
		}

		// Change the standard net.Listen function to the tls one.
		listenFunc = func(net string, laddr string) (net.Listener, error) {
			return tls.Listen(net, laddr, &tlsConfig)
		}
	}

	ipv4ListenAddrs, ipv6ListenAddrs, err := parseListeners(listenAddrs)
	if err != nil {
		return nil, err
	}
	listeners := make([]net.Listener, 0,
		len(ipv6ListenAddrs)+len(ipv4ListenAddrs))
	for _, addr := range ipv4ListenAddrs {
		listener, err := listenFunc("tcp4", addr)
		if err != nil {
			rpcLog.Warnf("Can't listen on %s: %v", addr, err)
			continue
		}
		listeners = append(listeners, listener)
	}

	for _, addr := range ipv6ListenAddrs {
		listener, err := listenFunc("tcp6", addr)
		if err != nil {
			rpcLog.Warnf("Can't listen on %s: %v", addr, err)
			continue
		}
		listeners = append(listeners, listener)
	}
	if len(listeners) == 0 {
		return nil, errors.New("RPC: No valid listen address")
	}

	rpc.listeners = listeners
	rpc.addHandlers()

	return &rpc, nil
}

// Stop is used by server.go to stop the rpc listener.
func (s *rpcServer) Stop() error {
	if atomic.AddInt32(&s.shutdown, 1) != 1 {
		rpcLog.Infof("RPC server is already in the process of shutting down")
		return nil
	}
	rpcLog.Warnf("RPC server shutting down")

	for _, listener := range s.listeners {
		err := listener.Close()
		if err != nil {
			rpcLog.Errorf("Problem shutting down rpc: %v", err)
			return err
		}
	}

	// These events are for the RPC server to forward objects received from
	// bmd.server to the client. They are emitted by NotifyObject because
	// bmd.server sends objects in the format most convenient to it (byte slice).
	// This then needs to be converted into a format that's appropriate for
	// transfer by RPC server to the clients, base64.
	s.evtMgr.RemoveListeners(rpcEvtNewMessage)
	s.evtMgr.RemoveListeners(rpcEvtNewBroadcast)
	s.evtMgr.RemoveListeners(rpcEvtNewGetpubkey)
	s.evtMgr.RemoveListeners(rpcEvtNewPubkey)
	s.evtMgr.RemoveListeners(rpcEvtNewUnknownObj)

	close(s.quit)
	s.wg.Wait()
	rpcLog.Infof("RPC server shutdown complete")
	return nil
}

// Start is used by server.go to start the rpc listener.
func (s *rpcServer) Start() {
	if atomic.AddInt32(&s.started, 1) != 1 {
		return
	}

	rpcLog.Trace("Starting RPC server")

	// Create mux for handling requests. This is necessary because if profiling
	// is enabled, then both profiler and RPC would be accessible from both
	// servers.
	rpcServeMux := http.NewServeMux()
	httpServer := &http.Server{
		Handler: rpcServeMux,
	}

	rpcServeMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Enforce cfg.RPCMaxClients
		if s.limitConnections(w, r.RemoteAddr) {
			return
		}

		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			if _, ok := err.(websocket.HandshakeError); !ok {
				rpcLog.Errorf("Unexpected websocket error: %v", err)
			}

			http.Error(w, "400 Bad Request.", http.StatusBadRequest)
			return
		}
		// Initialize state for client.
		state := rpc2.NewState()
		state.Set(rpcStateRemoteAddr, r.RemoteAddr)
		state.Set(rpcStateEventsID, prand.Int())
		state.Set(rpcStateIsAdmin, false)
		state.Set(rpcStateIsAuthenticated, false)

		s.rpcSrv.ServeCodecWithState(jsonrpc.NewJSONCodec(ws.UnderlyingConn()),
			state)
	})

	// Start listening on the listeners.
	for _, listener := range s.listeners {
		s.wg.Add(1)
		go func(listener net.Listener) {
			rpcLog.Infof("RPC server listening on %s", listener.Addr())
			httpServer.Serve(listener)
			rpcLog.Tracef("RPC listener done for %s", listener.Addr())
			s.wg.Done()
		}(listener)
	}
}

func init() {
	prand.Seed(time.Now().UnixNano())
}
