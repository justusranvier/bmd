// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/rpc2"
	"github.com/monetas/bmd/database"
	"github.com/monetas/bmutil"
	"github.com/monetas/bmutil/pow"
	"github.com/monetas/bmutil/wire"
)

// RPCAuthArgs contains arguments for Authenticate.
type RPCAuthArgs struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleAuth authenticates a websocket client using the supplied username and
// password. If the supplied authentication does not match the username and
// password expected, an error is returned.
//
// This check is time-constant.
//
// The function sets the values of isAuthenticated and isAdmin for the client.
// The first bool return value signifies auth success (true if successful) and
// the second bool return value specifies whether the user can change the state
// of the server (true) or whether the user is limited (false). The second is
// always false if the first is.
func (s *rpcServer) handleAuth(client *rpc2.Client, in *RPCAuthArgs, success *bool) error {
	login := in.Username + ":" + in.Password
	authsha := sha256.Sum256([]byte(login))
	c := client.State

	// Check for limited auth first as in environments with limited users, those
	// are probably expected to have a higher volume of calls
	limitcmp := subtle.ConstantTimeCompare(authsha[:], s.limitauthsha[:])
	if limitcmp == 1 {
		c.Set(rpcStateIsAuthenticated, true)
		c.Set(rpcStateIsAdmin, false)
		*success = true
		return nil
	}

	// Check for admin-level auth
	cmp := subtle.ConstantTimeCompare(authsha[:], s.authsha[:])
	if cmp == 1 {
		c.Set(rpcStateIsAuthenticated, true)
		c.Set(rpcStateIsAdmin, true)
		*success = true
		return nil
	}

	*success = false
	state := rpcConstructState(client)
	rpcLog.Warnf("RPC authentication failure from %s.", state.remoteAddr)

	return nil
}

// objectsSend sends the requested object into the Bitmessage network. in is
// a base64 representation of the object.
func (s *rpcServer) sendObject(client *rpc2.Client, in string, counter *uint64) error {
	if err := s.restrictAuth(client); err != nil {
		return err
	}
	data, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return errors.New("base64 decode failed")
	}

	// Check whether the object is valid.
	obj, err := wire.DecodeMsgObject(data)
	if err != nil {
		return fmt.Errorf("invalid object: %v", err)
	}
	if time.Now().After(obj.ExpiresTime) { // already expired
		return errors.New("object already expired")
	}
	if obj.StreamNumber != 1 { // TODO improve
		return errors.New("invalid stream")
	}

	// Check whether the PoW is valid.
	if !pow.Check(obj, pow.DefaultExtraBytes, pow.DefaultNonceTrialsPerByte,
		time.Now()) {
		return errors.New("invalid proof of work")
	}

	// Relay object to object manager which will handle insertion and
	// advertisement.
	*counter = s.server.objectManager.handleInsert(obj)
	if *counter == 0 {
		return errors.New("failed to insert and advertise object")
	}
	return nil
}

// RPCGetIDOut contains the output of GetIdentity.
type RPCGetIDOut struct {
	Address            string `json:"address"`
	NonceTrialsPerByte uint64 `json:"nonceTrialsPerByte"`
	ExtraBytes         uint64 `json:"extraBytes"`
	// base64 encoded bytes
	SigningKey    string `json:"signingKey"`
	EncryptionKey string `json:"encryptionKey"`
}

// handleGetId returns the stored public key associated with the given
// Bitmessage address.
func (s *rpcServer) getID(client *rpc2.Client, addr string, id *RPCGetIDOut) error {
	if err := s.restrictAuth(client); err != nil {
		return err
	}

	address, err := bmutil.DecodeAddress(addr)
	if err != nil {
		return fmt.Errorf("address decode failed: %v", err)
	}

	pubID, err := s.server.db.FetchIdentityByAddress(address)
	if err == database.ErrNonexistentObject {
		return errors.New("identity not found")
	} else if err != nil {
		rpcLog.Errorf("FetchIdentityByAddress, database error: %v", err)
		return errors.New("database error")
	}
	id.Address = addr
	id.NonceTrialsPerByte = pubID.NonceTrialsPerByte
	id.ExtraBytes = pubID.ExtraBytes
	id.EncryptionKey = base64.StdEncoding.EncodeToString(
		pubID.EncryptionKey.SerializeUncompressed())
	id.SigningKey = base64.StdEncoding.EncodeToString(
		pubID.SigningKey.SerializeUncompressed())
	return nil
}

// RPCSubscribeArgs contains the input for Subscribe methods.
type RPCSubscribeArgs struct {
	FromCounter uint64 `json:"fromCounter"`
}

// RPCReceiveArgs contains the input for Receive methods on the client side.
type RPCReceiveArgs struct {
	Object  string `json:"object"`
	Counter uint64 `json:"counter"`
}

// subscribeMessages subscribes the client to receiving objects of type message
// as soon as they are received by bmd. On the client side, ReceiveMessage RPC
// method is called.
func (s *rpcServer) subscribeMessages(client *rpc2.Client, args *RPCSubscribeArgs,
	_ *struct{}) error {
	return s.handleSubscribe(client, wire.ObjectTypeMsg, args, rpcEvtNewMessage,
		rpcClientHandleMessage)
}

// subscribeBroadcasts subscribes the client to receiving objects of type
// broadcast as soon as they are received by bmd. On the client side,
// ReceiveBroadcast RPC method is called.
func (s *rpcServer) subscribeBroadcasts(client *rpc2.Client, args *RPCSubscribeArgs,
	_ *struct{}) error {
	return s.handleSubscribe(client, wire.ObjectTypeBroadcast, args,
		rpcEvtNewBroadcast, rpcClientHandleBroadcast)
}

// subscribeGetpubkeys subscribes the client to receiving objects of type
// getpubkey as soon as they are received by bmd. On the client side,
// ReceiveGetpubkey RPC method is called.
func (s *rpcServer) subscribeGetpubkeys(client *rpc2.Client, args *RPCSubscribeArgs,
	_ *struct{}) error {
	return s.handleSubscribe(client, wire.ObjectTypeGetPubKey, args,
		rpcEvtNewGetpubkey, rpcClientHandleGetpubkey)
}

// subscribePubkeys subscribes the client to receiving objects of type
// pubkey as soon as they are received by bmd. On the client side,
// ReceivePubkey RPC method is called.
func (s *rpcServer) subscribePubkeys(client *rpc2.Client, args *RPCSubscribeArgs,
	_ *struct{}) error {
	return s.handleSubscribe(client, wire.ObjectTypePubKey, args,
		rpcEvtNewPubkey, rpcClientHandlePubkey)
}

// subscribeUnknownObjects subscribes the client to receiving objects of unknown
// type as soon as they are received by bmd. On the client side,
// ReceiveUnknownObject RPC method is called.
func (s *rpcServer) subscribeUnknownObjects(client *rpc2.Client, args *RPCSubscribeArgs,
	_ *struct{}) error {
	// XXX just a hack
	return s.handleSubscribe(client, wire.ObjectType(999999), args,
		rpcEvtNewUnknownObj, rpcClientHandleUnknownObj)
}

func (s *rpcServer) handleSubscribe(client *rpc2.Client, objType wire.ObjectType,
	args *RPCSubscribeArgs, evt string, clientHandler string) error {
	// Make sure only authenticated users can subscribe to objects.
	if err := s.restrictAuth(client); err != nil {
		return err
	}
	state := rpcConstructState(client)

	s.evtMgr.On(evt, func(out *RPCReceiveArgs) {
		s.mutex.RLock()
		defer s.mutex.RUnlock()

		s.clients[client].NotifyObject(out, objType)
	}, state.eventsID)

	// We subscribe to event before sending old objects because otherwise there
	// might be misses because of race conditions. Duplication >> Misses.
	return s.sendOldObjects(client, objType, args.FromCounter, clientHandler)
}

// sendOldObjects is used to send objects of a particular type starting from a
// fixed counter value to the client.
func (s *rpcServer) sendOldObjects(client *rpc2.Client, objType wire.ObjectType,
	fromCounter uint64, clientHandler string) error {
	objs, lastCount, err := s.server.db.FetchObjectsFromCounter(objType,
		fromCounter, rpcCounterObjectsSize)
	if err != nil {
		rpcLog.Errorf("FetchObjectsFromCounter, database error: %v", err)
		return errors.New("database error")
	}

	s.mutex.RLock()
	c := s.clients[client]
	s.mutex.RUnlock()

	// Send objects to client. Terminate all requests if one fails.
	for counter, msg := range objs {
		out := &RPCReceiveArgs{
			Object:  base64.StdEncoding.EncodeToString(wire.EncodeMessage(msg)),
			Counter: counter,
		}
		c.NotifyObject(out, objType)
	}

	// We might have more objects to send.
	if len(objs) == rpcCounterObjectsSize {
		return s.sendOldObjects(client, objType, lastCount, clientHandler)
	}
	return nil
}
