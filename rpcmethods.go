// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"time"

	"github.com/monetas/bmd/database"
	pb "github.com/monetas/bmd/rpcproto"
	"github.com/monetas/bmutil"
	"github.com/monetas/bmutil/pow"
	"github.com/monetas/bmutil/wire"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// SendObject inserts the object into bmd's database and sends it out to the
// Bitmessage network.
func (s *rpcServer) SendObject(ctx context.Context, in *pb.Object) (*pb.SendObjectReply, error) {
	if code := s.restrictAuth(ctx); code != codes.OK {
		return nil, grpc.Errorf(code, "auth failure")
	}
	if len(in.Contents) == 0 {
		return nil, grpc.Errorf(codes.InvalidArgument, "contents must not be empty")
	}

	objMsg, err := wire.DecodeMsgObject(in.Contents)
	if err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "error decoding object: %v",
			err)
	}

	// Check if object is already in database.
	exists, err := s.server.db.ExistsObject(objMsg.InventoryHash())
	if err != nil {
		rpcLog.Errorf("ExistsObject, database error: %v", err)
		return nil, grpc.Errorf(codes.Internal, "database error")
	}
	if exists {
		return nil, grpc.Errorf(codes.AlreadyExists, "object already in database")
	}

	// Check whether the object is valid.
	if time.Now().After(objMsg.ExpiresTime) { // already expired
		return nil, grpc.Errorf(codes.InvalidArgument, "object already expired")
	}
	if objMsg.StreamNumber != 1 { // TODO improve
		return nil, grpc.Errorf(codes.InvalidArgument, "invalid stream")
	}

	// Check whether the PoW is valid.
	if !pow.Check(objMsg, pow.DefaultExtraBytes, pow.DefaultNonceTrialsPerByte,
		time.Now()) {
		return nil, grpc.Errorf(codes.InvalidArgument, "invalid proof of work")
	}

	// Relay object to object manager which will handle insertion and
	// advertisement.
	counter := s.server.objectManager.handleInsert(objMsg)
	if counter == 0 {
		return nil, grpc.Errorf(codes.Internal, "failed to insert and advertise object")
	}

	return &pb.SendObjectReply{
		Counter: counter,
	}, nil
}

// GetIdentity returns the stored public key associated with the given
// Bitmessage address.
func (s *rpcServer) GetIdentity(ctx context.Context, in *pb.GetIdentityRequest) (*pb.GetIdentityReply, error) {
	if code := s.restrictAuth(ctx); code != codes.OK {
		return nil, grpc.Errorf(code, "auth failure")
	}

	address, err := bmutil.DecodeAddress(in.Address)
	if err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "address decode failed: %v",
			err)
	}

	pubID, err := s.server.db.FetchIdentityByAddress(address)
	if err == database.ErrNonexistentObject {
		return nil, grpc.Errorf(codes.NotFound, "identity not found")
	} else if err != nil {
		rpcLog.Errorf("FetchIdentityByAddress, database error: %v", err)
		return nil, grpc.Errorf(codes.Internal, "database error")
	}

	return &pb.GetIdentityReply{
		NonceTrials:   pubID.NonceTrialsPerByte,
		ExtraBytes:    pubID.ExtraBytes,
		SigningKey:    pubID.SigningKey.SerializeUncompressed(),
		EncryptionKey: pubID.EncryptionKey.SerializeUncompressed(),
	}, nil
}

// GetObjects retrieves objects of a particular type starting from a particular
// counter value from the database and streams them to the client.
func (s *rpcServer) GetObjects(in *pb.GetObjectsRequest, stream pb.Bmd_GetObjectsServer) error {
	ctx := stream.Context()
	if code := s.restrictAuth(ctx); code != codes.OK {
		return grpc.Errorf(code, "auth failure")
	}

	if in.FromCounter == 0 {
		return grpc.Errorf(codes.InvalidArgument, "from_counter cannot be 0")
	}

	// fromCounter is updated after each iteration of the loop and set to the
	// value of the last element+1.
	fromCounter := in.FromCounter
	objType := wire.ObjectType(in.ObjectType)
	cond := s.objConds[objType.String()]

	for {
		objs, lastCount, err := s.server.db.FetchObjectsFromCounter(objType,
			fromCounter, rpcCounterObjectsSize)
		if err != nil {
			rpcLog.Errorf("FetchObjectsFromCounter, database error: %v", err)
			return grpc.Errorf(codes.Internal, "database error")
		}

		// We ran out of more objects to send to the client, so wait until we have
		// more.
		if len(objs) == 0 {
			cond.L.Lock()
			cond.Wait()
			cond.L.Unlock()
			continue
		}

		// For next iteration.
		fromCounter = lastCount + 1

		// Send objects to client.
		for counter, msg := range objs {
			out := &pb.Object{
				Contents: wire.EncodeMessage(msg),
				Counter:  counter,
			}
			err = stream.Send(out)
			if err != nil {
				return grpc.Errorf(codes.DataLoss, "failed to send object: %v", err)
			}
		}
	}
}
