// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package peer provides objects for managing a connection to a remote peer that obeys
the bitmessage protocol.

Connection abstracts a tcp connection to a remote bitmessage peer so that the rest
of the peer does not have to deal with byte arrays. It also prevents timeout by
regularly sending pong messages.

Listener listens for incoming tcp connections and creates Connection objects
for them when a connection is opened.

Send manages everything that is to be sent to the remote peer eventually. Data
requests, inv trickles, and other messages.

Peer manages the peer object overall and routes incoming messages.

Logic is an interface that has functions such as HandleAddrMsg that handles the
different kinds of messages in the bitmessage protocol.

Inventory can be used to store the known inventory and the requested inventory
for a peer.

*/
package peer
