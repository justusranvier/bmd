package bmpeer

import (
	"sync"

	"github.com/monetas/bmd/database"
	"github.com/monetas/bmutil/wire"
)

// Inventory is the part of a peer that manages object hashes and remembers
// which are known to the peer and which have been requested from it.
type Inventory struct {
	db        database.Db
	known     *MruInventoryMap
	requested *MruInventoryMap
	mutex     sync.Mutex
}

// IsKnownInventory returns whether or not the peer is known to have the passed
// inventory. It is safe for concurrent access.
func (I *Inventory) IsKnownInventory(invVect *wire.InvVect) bool {
	I.mutex.Lock()
	defer I.mutex.Unlock()

	if I.known.Exists(invVect) {
		return true
	}
	return false
}

// AddKnownInventory adds the passed inventory to the cache of known inventory
// for the peer. It is safe for concurrent access.
func (I *Inventory) AddKnownInventory(invVect *wire.InvVect) {
	I.mutex.Lock()
	defer I.mutex.Unlock()

	I.known.Add(invVect)
}

// AddRequest adds a request to the set of requested inventory.
func (I *Inventory) AddRequest(invVect *wire.InvVect) {
	I.mutex.Lock()
	defer I.mutex.Unlock()

	I.requested.Add(invVect)
}

// DeleteRequest removes an entry from the set of requested inventory. It returns
// true if the inventory was really removed.
func (I *Inventory) DeleteRequest(invVect *wire.InvVect) (ok bool) {
	I.mutex.Lock()
	defer I.mutex.Unlock()

	if I.requested.Exists(invVect) {
		I.requested.Delete(invVect)
		return true
	}
	return false
}

// RetrieveObject retrieves an object from the database and decodes it.
// TODO we actually end up decoding the message and then encoding it again when
// it is sent. That is not necessary.
func (I *Inventory) RetrieveObject(inv *wire.InvVect) wire.Message {
	obj, err := I.db.FetchObjectByHash(&inv.Hash)
	if err != nil {
		return nil
	}

	msg, err := wire.DecodeMsgObject(obj)
	if err != nil {
		return nil
	}

	return msg
}

// RetrieveData retrieves an object from the database and returns it as a stream
func (I *Inventory) RetrieveData(inv *wire.InvVect) []byte {
	obj, err := I.db.FetchObjectByHash(&inv.Hash)
	if err != nil {
		return nil
	}

	return obj
}

// FilterKnown takes a list of InvVects, adds them to the list of known
// inventory, and returns those which were not already in the list. It is used
// to insure that data is not sent to the peer that it already is known to have.
func (I *Inventory) FilterKnown(inv []*wire.InvVect) []*wire.InvVect {
	I.mutex.Lock()
	defer I.mutex.Unlock()

	return I.known.Filter(inv)
}

// FilterRequested takes a list of InvVects, adds them to the list of requested
// data, and returns those which were not already in the list. It is used to
// insure that data is not requested twice.
func (I *Inventory) FilterRequested(inv []*wire.InvVect) []*wire.InvVect {
	I.mutex.Lock()
	defer I.mutex.Unlock()

	return I.requested.Filter(inv)
}

// NewInventory returns a new Inventory object.
func NewInventory(db database.Db) *Inventory {
	return &Inventory{
		db:        db,
		known:     NewMruInventoryMap(maxKnownInventory),
		requested: NewMruInventoryMap(maxKnownInventory),
	}
}
