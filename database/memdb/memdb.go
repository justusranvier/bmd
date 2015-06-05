// Originally derived from: btcsuite/btcd/database/memdb/memdb.go
// Copyright (c) 2013-2015 Conformal Systems LLC.

// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package memdb

import (
	"bytes"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/monetas/bmd/database"
	"github.com/monetas/bmutil"
	"github.com/monetas/bmutil/identity"
	"github.com/monetas/bmutil/wire"
)

const (
	panicMessage = "ALIEN INVASION IN PROGRESS. (bug)"
)

// counters type serves to enable sorting of uint64 slices using sort.Sort
// function. Implements sort.Interface.
type counters []uint64

func (c counters) Len() int {
	return len(c)
}

func (c counters) Less(i, j int) bool {
	return c[i] < c[j]
}

func (c counters) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

// counter includes a map to a kind of object and the counter value of the last
// element added.
type counter struct {
	// Holds a mapping from counter to shahash for some object types.
	ByCounter map[uint64]*wire.ShaHash
	// Keep track of current counter positions (last element added)
	CounterPos uint64
}

func (cmap *counter) Insert(hash *wire.ShaHash) {
	cmap.CounterPos++                      // increment, new item.
	cmap.ByCounter[cmap.CounterPos] = hash // insert to counter map
}

// MemDb is a concrete implementation of the database.Db interface which
// provides a memory-only database. Since it is memory-only, it is obviously not
// persistent and is mostly only useful for testing purposes.
type MemDb struct {
	// Embed a mutex for safe concurrent access.
	sync.RWMutex

	// objectsByHash keeps track of unexpired objects by their inventory hash.
	objectsByHash map[wire.ShaHash][]byte

	// pubkeyByTag keeps track of all public keys (even expired) by their
	// tag (which can be calculated from the address).
	pubKeyByTag map[wire.ShaHash][]byte

	// counters for respective object types.
	msgCounter       *counter
	broadcastCounter *counter
	pubKeyCounter    *counter
	getPubKeyCounter *counter

	// counters for unknown objects.
	unknownObjCounter *counter

	// closed indicates whether or not the database has been closed and is
	// therefore invalidated.
	closed bool
}

// getCounterMap is a helper function used to get the map which maps counter to
// object hash based on `objType'.
func (db *MemDb) getCounter(objType wire.ObjectType) *counter {
	switch objType {
	case wire.ObjectTypeBroadcast:
		return db.broadcastCounter
	case wire.ObjectTypeMsg:
		return db.msgCounter
	case wire.ObjectTypePubKey:
		return db.pubKeyCounter
	case wire.ObjectTypeGetPubKey:
		return db.getPubKeyCounter
	default:
		return db.unknownObjCounter
	}
}

// Close cleanly shuts down database. This is part of the database.Db interface
// implementation.
//
// All data is purged upon close with this implementation since it is a
// memory-only database.
func (db *MemDb) Close() error {
	db.Lock()
	defer db.Unlock()

	if db.closed {
		return database.ErrDbClosed
	}

	db.objectsByHash = nil
	db.pubKeyByTag = nil
	db.msgCounter = nil
	db.broadcastCounter = nil
	db.pubKeyCounter = nil
	db.getPubKeyCounter = nil
	db.unknownObjCounter = nil
	db.closed = true
	return nil
}

// ExistsObject returns whether or not an object with the given inventory hash
// exists in the database. This is part of the database.Db interface
// implementation.
func (db *MemDb) ExistsObject(hash *wire.ShaHash) (bool, error) {
	db.RLock()
	defer db.RUnlock()

	if db.closed {
		return false, database.ErrDbClosed
	}

	if _, exists := db.objectsByHash[*hash]; exists {
		return true, nil
	}

	return false, nil
}

// No locks here, meant to be used inside public facing functions.
func (db *MemDb) fetchObjectByHash(hash *wire.ShaHash) ([]byte, error) {
	if object, exists := db.objectsByHash[*hash]; exists {
		return object, nil
	}

	return nil, database.ErrNonexistentObject
}

// FetchObjectByHash returns an object from the database as a byte array.
// It is upto the implementation to decode the byte array. This is part of the
// database.Db interface implementation.
//
// This implementation does not use any additional cache since the entire
// database is already in memory.
func (db *MemDb) FetchObjectByHash(hash *wire.ShaHash) ([]byte, error) {
	db.RLock()
	defer db.RUnlock()

	if db.closed {
		return nil, database.ErrDbClosed
	}

	return db.fetchObjectByHash(hash)
}

// FetchObjectByCounter returns an object from the database as a byte array
// based on the object type and counter value. The implementation may cache the
// underlying data if desired. This is part of the database.Db interface
// implementation.
//
// This implementation does not use any additional cache since the entire
// database is already in memory.
func (db *MemDb) FetchObjectByCounter(objType wire.ObjectType,
	counter uint64) ([]byte, error) {
	db.RLock()
	defer db.RUnlock()
	if db.closed {
		return nil, database.ErrDbClosed
	}

	counterMap := db.getCounter(objType)
	hash, ok := counterMap.ByCounter[counter]
	if !ok {
		return nil, database.ErrNonexistentObject
	}
	obj, err := db.fetchObjectByHash(hash)
	if err != nil {
		panic(panicMessage)
	}
	return obj, nil
}

// FetchObjectsFromCounter returns a map of `count' objects which have a
// counter position starting from `counter'. Key is the value of counter and
// value is a byte slice containing the object. It also returns the counter
// value of the last object, which could be useful for more queries to the
// function. The implementation may cache the underlying data if desired.
// This is part of the database.Db interface implementation.
//
// This implementation does not use any additional cache since the entire
// database is already in memory.
func (db *MemDb) FetchObjectsFromCounter(objType wire.ObjectType, counter uint64,
	count uint64) (map[uint64][]byte, uint64, error) {
	db.RLock()
	defer db.RUnlock()
	if db.closed {
		return nil, 0, database.ErrDbClosed
	}

	counterMap := db.getCounter(objType)

	var c uint64 // count

	keys := make([]uint64, 0, count)

	// make a slice of keys to retrieve
	for k := range counterMap.ByCounter {
		if k < counter { // discard this element
			continue
		}
		keys = append(keys, k)
		c++
	}
	sort.Sort(counters(keys)) // sort retrieved keys
	var newCounter uint64
	if len(keys) == 0 {
		newCounter = 0
	} else if uint64(len(keys)) <= count {
		newCounter = keys[len(keys)-1] // counter value of last element
	} else { // more keys than required
		newCounter = keys[count-1] // Get counter'th element
		keys = keys[:count]        // we don't need excess elements
	}
	objects := make(map[uint64][]byte)

	// start fetching objects in ascending order
	for _, v := range keys {
		hash := counterMap.ByCounter[v]
		obj, err := db.fetchObjectByHash(hash)
		// error checking
		if err != nil {
			panic(panicMessage)
		}
		// ensure that database and returned byte arrays are separate
		objCopy := make([]byte, len(obj))
		copy(objCopy, obj)

		objects[v] = objCopy
	}

	return objects, newCounter, nil
}

// FetchIdentityByAddress returns identity.Public stored in the form
// of a PubKey message in the pubkey database. It needs to go through all the
// public keys in the database to find this. The implementation must thus cache
// results, if needed. This is part of the database.Db interface implementation.
func (db *MemDb) FetchIdentityByAddress(addr *bmutil.Address) (*identity.Public,
	error) {

	db.RLock()
	defer db.RUnlock()
	if db.closed {
		return nil, database.ErrDbClosed
	}

	for tag, obj := range db.pubKeyByTag {
		msg := new(wire.MsgPubKey)
		err := msg.Decode(bytes.NewReader(obj))
		if err != nil {
			return nil, fmt.Errorf("while decoding pubkey with tag %x,"+
				" got error %v", tag, err)
		}

		switch msg.Version {
		case wire.SimplePubKeyVersion:
			fallthrough
		case wire.ExtendedPubKeyVersion:
			id, err := identity.FromPubKeyMsg(msg)
			if err != nil { // invalid encryption/signing keys
				return nil, err
			}
			if bytes.Equal(id.Address.Tag(), addr.Tag()) { // we have our match
				return id, nil
			}
		case wire.EncryptedPubKeyVersion:
			if bytes.Equal(msg.Tag.Bytes(), addr.Tag()) { // decrypt this key
				// TODO
				return nil, database.ErrNotImplemented
			}
		default:
			continue // unknown pubkey version
		}
	}

	return nil, database.ErrNonexistentObject
}

// FilterObjects returns a map of objects that return true when passed to
// the filter function. It could be used for grabbing objects of a certain
// type, like getpubkey requests. This is an expensive operation as it
// copies data corresponding to anything that matches the filter. Use
// sparingly and ensure that only a few objects can match.
//
// WARNING: filter must not mutate the object and/or its inventory hash.
//
// This is part of the database.Db interface implementation.
func (db *MemDb) FilterObjects(filter func(hash *wire.ShaHash,
	obj []byte) bool) (map[wire.ShaHash][]byte, error) {

	db.RLock()
	defer db.RUnlock()
	if db.closed {
		return nil, database.ErrDbClosed
	}

	res := make(map[wire.ShaHash][]byte)
	for hash, obj := range db.objectsByHash {
		if filter(&hash, obj) { // we need this
			objCopy := make([]byte, len(obj))
			copy(objCopy, obj)

			res[hash] = objCopy
		}
	}

	return res, nil
}

// FetchRandomInvHashes returns the specified number of inventory hashes
// corresponding to random unexpired objects from the database and filtering
// them by calling filter(invHash, objectData) on each object. A return
// value of true from filter means that the object would be returned.
//
// Useful for creating inv message, with filter being used to filter out
// inventory hashes that have already been sent out to a particular node.
//
// WARNING: filter must not mutate the object and/or its inventory hash.
//
// This is part of the database.Db interface implementation.
func (db *MemDb) FetchRandomInvHashes(count uint64,
	filter func(*wire.ShaHash, []byte) bool) ([]wire.ShaHash, error) {

	db.RLock()
	defer db.RUnlock()
	if db.closed {
		return nil, database.ErrDbClosed
	}
	// number of objects to be returned
	counter := uint64(0)
	// filtered hashes to be returned
	res := make([]wire.ShaHash, 0, count)

	// golang ensures that iteration over maps is psuedorandom
	for hash, obj := range db.objectsByHash {
		if counter >= count { // we have all we need
			break
		}
		if filter(&hash, obj) { // we need this item
			res = append(res, hash)
			counter++
		}
	}

	return res, nil
}

// GetCounter returns the highest value of counter that exists for objects
// of the given type. This is part of the database.Db interface implementation.
func (db *MemDb) GetCounter(objType wire.ObjectType) (uint64, error) {
	db.RLock()
	defer db.RUnlock()
	if db.closed {
		return 0, database.ErrDbClosed
	}

	c := db.getCounter(objType)
	return c.CounterPos, nil
}

// InsertObject inserts data of the given type and hash into the database.
// It returns the calculated/stored inventory hash as well as the counter
// position (if object type has a counter associated with it). If the object
// is a PubKey, it inserts it into a separate place where it isn't touched
// by RemoveObject or RemoveExpiredObjects and has to be removed using
// RemovePubKey. This is part of the database.Db interface implementation.
func (db *MemDb) InsertObject(data []byte) (uint64, error) {
	db.Lock()
	defer db.Unlock()
	if db.closed {
		return 0, database.ErrDbClosed
	}

	_, _, objType, _, _, err := wire.DecodeMsgObjectHeader(bytes.NewReader(data))
	if err != nil { // all objects must be valid
		return 0, err
	}

	hash, _ := wire.NewShaHash(bmutil.CalcInventoryHash(data))

	// ensure that modifying input args doesn't change contents in database
	dataInsert := make([]byte, len(data))
	copy(dataInsert, data)

	// handle pubkeys
	if objType == wire.ObjectTypePubKey {
		msg := new(wire.MsgPubKey)
		err = msg.Decode(bytes.NewReader(data))
		if err != nil {
			goto doneInsert // fail silently
		}

		var tag []byte

		switch msg.Version {
		case wire.SimplePubKeyVersion:
			fallthrough
		case wire.ExtendedPubKeyVersion:
			id, err := identity.FromPubKeyMsg(msg)
			if err != nil { // invalid encryption/signing keys
				goto doneInsert
			}
			tag = id.Address.Tag()
		case wire.EncryptedPubKeyVersion:
			tag = msg.Tag.Bytes() // directly included
		}
		tagH, err := wire.NewShaHash(tag)
		if err != nil {
			goto doneInsert
		}
		db.pubKeyByTag[*tagH] = dataInsert // insert pubkey
	}

doneInsert: // label used because normal insertion should still succeed
	// insert object into the object hash table
	db.objectsByHash[*hash] = dataInsert

	// increment counter
	counterMap := db.getCounter(objType)
	counterMap.Insert(hash)
	return counterMap.CounterPos, nil
}

// RemoveObject removes the object with the specified hash from the database.
// This is part of the database.Db interface implementation.
func (db *MemDb) RemoveObject(hash *wire.ShaHash) error {
	db.Lock()
	defer db.Unlock()
	if db.closed {
		return database.ErrDbClosed
	}

	obj, ok := db.objectsByHash[*hash]
	if !ok {
		return database.ErrNonexistentObject
	}

	// check and remove object from counter maps
	_, _, objType, _, _, err := wire.DecodeMsgObjectHeader(bytes.NewReader(obj))
	counterMap := db.getCounter(objType)
	if err != nil { // impossible, all objects must be valid
		panic(panicMessage)
	}
	for k, v := range counterMap.ByCounter { // go through each element
		if v.IsEqual(hash) { // we got a match, so delete
			delete(counterMap.ByCounter, k)
			break
		}
	}

	// remove object from object map
	delete(db.objectsByHash, *hash) // done!

	return nil
}

// RemoveObjectByCounter removes the object with the specified counter value
// from the database. This is part of the database.Db interface implementation.
func (db *MemDb) RemoveObjectByCounter(objType wire.ObjectType,
	counter uint64) error {

	db.Lock()
	defer db.Unlock()
	if db.closed {
		return database.ErrDbClosed
	}

	counterMap := db.getCounter(objType)
	hash, ok := counterMap.ByCounter[counter]
	if !ok {
		return database.ErrNonexistentObject
	}

	delete(counterMap.ByCounter, counter) // delete counter reference
	delete(db.objectsByHash, *hash)       // delete object itself
	return nil
}

// RemoveExpiredObjects prunes all objects in the main circulation store
// whose expiry time has passed (along with a margin of 3 hours). This does
// not touch the pubkeys stored in the public key collection. This is part of
// the database.Db interface implementation.
func (db *MemDb) RemoveExpiredObjects() error {
	db.Lock()
	defer db.Unlock()
	if db.closed {
		return database.ErrDbClosed
	}

	for hash, obj := range db.objectsByHash {
		_, expiresTime, objType, _, _, err := wire.DecodeMsgObjectHeader(bytes.NewReader(obj))
		if err != nil { // impossible, all objects must be valid
			panic(panicMessage)
		}
		// current time - 3 hours
		if time.Now().Add(-time.Hour * 3).After(expiresTime) { // expired
			// remove from counter map
			counterMap := db.getCounter(objType)

			for k, v := range counterMap.ByCounter { // go through each element
				if v.IsEqual(&hash) { // we got a match, so delete
					delete(counterMap.ByCounter, k)
					break
				}
			}

			// remove object from object map
			delete(db.objectsByHash, hash)
		}
	}
	return nil
}

// RemovePubKey removes a PubKey from the PubKey store with the specified
// tag. Note that it doesn't touch the general object store and won't remove
// the public key from there. This is part of the database.Db interface
// implementation.
func (db *MemDb) RemovePubKey(tag *wire.ShaHash) error {
	db.Lock()
	defer db.Unlock()
	if db.closed {
		return database.ErrDbClosed
	}

	_, ok := db.pubKeyByTag[*tag]
	if !ok {
		return database.ErrNonexistentObject
	}

	delete(db.pubKeyByTag, *tag) // remove
	return nil
}

// RollbackClose discards the recent database changes to the previously saved
// data at last Sync and closes the database. This is part of the database.Db
// interface implementation.
//
// The database is completely purged on close with this implementation since the
// entire database is only in memory. As a result, this function behaves no
// differently than Close.
func (db *MemDb) RollbackClose() error {
	// Rollback doesn't apply to a memory database, so just call Close.
	// Close handles the mutex locks.
	return db.Close()
}

// Sync verifies that the database is coherent on disk and no outstanding
// transactions are in flight. This is part of the database.Db interface
// implementation.
//
// This implementation does not write any data to disk, so this function only
// grabs a lock to ensure it doesn't return until other operations are complete.
func (db *MemDb) Sync() error {
	db.Lock()
	defer db.Unlock()

	if db.closed {
		return database.ErrDbClosed
	}

	// There is nothing extra to do to sync the memory database. However,
	// the lock is still grabbed to ensure the function does not return
	// until other operations are complete.
	return nil
}

// newMemDb returns a new memory-only database ready for block inserts.
func newMemDb() *MemDb {
	db := MemDb{
		objectsByHash:     make(map[wire.ShaHash][]byte),
		pubKeyByTag:       make(map[wire.ShaHash][]byte),
		msgCounter:        &counter{make(map[uint64]*wire.ShaHash), 0},
		broadcastCounter:  &counter{make(map[uint64]*wire.ShaHash), 0},
		pubKeyCounter:     &counter{make(map[uint64]*wire.ShaHash), 0},
		getPubKeyCounter:  &counter{make(map[uint64]*wire.ShaHash), 0},
		unknownObjCounter: &counter{make(map[uint64]*wire.ShaHash), 0},
	}
	return &db
}
