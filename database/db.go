// Originally derived from: btcsuite/btcd/database/db.go
// Copyright (c) 2013-2015 Conformal Systems LLC.

// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"errors"

	"github.com/monetas/bmutil"
	"github.com/monetas/bmutil/identity"
	"github.com/monetas/bmutil/wire"
)

// Errors that the various database functions may return.
var (
	ErrDbClosed          = errors.New("database is closed")
	ErrDuplicateObject   = errors.New("duplicate insert attempted")
	ErrDbDoesNotExist    = errors.New("non-existent database")
	ErrDbUnknownType     = errors.New("non-existent database type")
	ErrNotImplemented    = errors.New("method has not yet been implemented")
	ErrNonexistentObject = errors.New("object doesn't exist in database")
)

// Db defines a generic interface that is used to request and insert data into
// the database. This interface is intended to be agnostic to actual mechanism
// used for backend data storage. The AddDBDriver function can be used to add a
// new backend data storage method.
type Db interface {
	// Close cleanly shuts down the database and syncs all data.
	Close() error

	// ExistsObject returns whether or not an object with the given inventory
	// hash exists in the database.
	ExistsObject(*wire.ShaHash) (bool, error)

	// FetchObjectByHash returns an object from the database as a byte array.
	// It is upto the implementation to decode the byte array.
	FetchObjectByHash(*wire.ShaHash) ([]byte, error)

	// FetchObjectByCounter returns the corresponding object based on the
	// counter. Note that each object type has a different counter, with unknown
	// objects being consolidated into one counter. Counters are meant for use
	// as a convenience method for fetching new data from database since last
	// check.
	FetchObjectByCounter(wire.ObjectType, uint64) ([]byte, error)

	// FetchObjectsFromCounter returns a map of `count' objects which have a
	// counter position starting from `counter'. Key is the value of counter and
	// value is a byte slice containing the object. It also returns the counter
	// value of the last object, which could be useful for more queries to the
	// function.
	FetchObjectsFromCounter(objType wire.ObjectType, counter uint64,
		count uint64) (map[uint64][]byte, uint64, error)

	// FetchIdentityByAddress returns identity.Public stored in the form
	// of a PubKey message in the pubkey database.
	FetchIdentityByAddress(*bmutil.Address) (*identity.Public, error)

	// FilterObjects returns a map of objects that return true when passed to
	// the filter function. It could be used for grabbing objects of a certain
	// type, like getpubkey requests. This is an expensive operation as it
	// copies data corresponding to anything that matches the filter. Use
	// sparingly and ensure that only a few objects can match.
	//
	// WARNING: filter must not mutate the object and/or its inventory hash.
	FilterObjects(func(hash *wire.ShaHash,
		obj []byte) bool) (map[wire.ShaHash][]byte, error)

	// FetchRandomInvHashes returns the specified number of inventory hashes
	// corresponding to random unexpired objects from the database and filtering
	// them by calling filter(invHash, objectData) on each object. A return
	// value of true from filter means that the object would be returned.
	//
	// Useful for creating inv message, with filter being used to filter out
	// inventory hashes that have already been sent out to a particular node.
	//
	// WARNING: filter must not mutate the object and/or its inventory hash.
	FetchRandomInvHashes(count uint64,
		filter func(*wire.ShaHash, []byte) bool) ([]wire.ShaHash, error)

	// GetCounter returns the highest value of counter that exists for objects
	// of the given type.
	GetCounter(wire.ObjectType) (uint64, error)

	// InsertObject inserts data of the given type and hash into the database.
	// It returns the counter position. If the object is a PubKey, it inserts it
	// into a separate place where it isn't touched by RemoveObject or
	// RemoveExpiredObjects and has to be removed using RemovePubKey.
	InsertObject([]byte) (uint64, error)

	// RemoveObject removes the object with the specified hash from the
	// database. Does not remove PubKeys.
	RemoveObject(*wire.ShaHash) error

	// RemoveObjectByCounter removes the object with the specified counter value
	// from the database.
	RemoveObjectByCounter(wire.ObjectType, uint64) error

	// RemoveExpiredObjects prunes all objects in the main circulation store
	// whose expiry time has passed (along with a margin of 3 hours). This does
	// not touch the pubkeys stored in the public key collection.
	RemoveExpiredObjects() error

	// RemovePubKey removes a PubKey from the PubKey store with the specified
	// tag. Note that it doesn't touch the general object store and won't remove
	// the public key from there.
	RemovePubKey(*wire.ShaHash) error

	// RollbackClose discards the recent database changes to the previously
	// saved data at last Sync and closes the database.
	RollbackClose() (err error)

	// Sync verifies that the database is coherent on disk and no
	// outstanding transactions are in flight.
	Sync() (err error)
}

// DriverDB defines a structure for backend drivers to use when they registered
// themselves as a backend which implements the Db interface.
type DriverDB struct {
	DbType   string
	CreateDB func(args ...interface{}) (pbdb Db, err error)
	OpenDB   func(args ...interface{}) (pbdb Db, err error)
}

// driverList holds all of the registered database backends.
var driverList []DriverDB

// AddDBDriver adds a back end database driver to available interfaces.
func AddDBDriver(instance DriverDB) {
	for _, drv := range driverList {
		if drv.DbType == instance.DbType {
			return
		}
	}
	driverList = append(driverList, instance)
}

// CreateDB intializes and opens a database.
func CreateDB(dbtype string, args ...interface{}) (pbdb Db, err error) {
	for _, drv := range driverList {
		if drv.DbType == dbtype {
			return drv.CreateDB(args...)
		}
	}
	return nil, ErrDbUnknownType
}

// OpenDB opens an existing database.
func OpenDB(dbtype string, args ...interface{}) (pbdb Db, err error) {
	for _, drv := range driverList {
		if drv.DbType == dbtype {
			return drv.OpenDB(args...)
		}
	}
	return nil, ErrDbUnknownType
}

// SupportedDBs returns a slice of strings that represent the database drivers
// that have been registered and are therefore supported.
func SupportedDBs() []string {
	var supportedDBs []string
	for _, drv := range driverList {
		supportedDBs = append(supportedDBs, drv.DbType)
	}
	return supportedDBs
}
