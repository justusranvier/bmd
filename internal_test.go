// Copyright (c) 2015 Monetas
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.package main

package main

import (	
	"github.com/monetas/bmd/bmpeer"
	"github.com/monetas/bmd/database"
	"github.com/monetas/bmutil/wire"
)

// TstNewServer allows for the creation of a sever with extra parameters for testing
// purposes, such as a nonstandard set of default peers.
func (s *server) TstStart(startPeers []*DefaultPeer) {
	s.start(startPeers)
}

// TstNewServer
func TstNewServer(listenAddrs []string, db database.Db, listen func(string, string) (bmpeer.Listener, error)) (*server, error) {
	return newServer(listenAddrs, db, listen)
}

// TstNewPeerHandshakeComplete creates a new peer object that has already nearly
// completed its initial handshake. You just need to send it a ver ack and it will
// run as if that was the last step necessary. It comes already running. 
func TstNewPeerHandshakeComplete(s *server, conn bmpeer.Connection, inventory *bmpeer.Inventory, sq bmpeer.SendQueue) *bmpeer.Peer{
	logic := &peer{
		server:          s,
		protocolVersion: maxProtocolVersion,
		bmnet:           wire.MainNet,
		services:        wire.SFNodeNetwork,
		inbound:         true,
		inventory:       inventory, 
		sendQueue:       sq, 
		addr:            conn.RemoteAddr(), 
		versionSent:     true,
		versionKnown:    true,
		userAgent:       wire.DefaultUserAgent,
	}
	
	p := bmpeer.NewPeer(logic, conn, sq, true, 0) 

	p.Start()	
	
	return p
}
