package bmpeer

import (
	"sync"

	"github.com/monetas/bmd/database"
	"github.com/monetas/bmutil/wire"	
)

// 
type Inventory struct {
	db        database.Db
	known     *MruInventoryMap
	requested *MruInventoryMap
	mutex     sync.Mutex
}

// isKnownInventory returns whether or not the peer is known to have the passed
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

func (I *Inventory) Request(invVect *wire.InvVect) {
	I.mutex.Lock()
	defer I.mutex.Unlock()

	I.requested.Add(invVect)
}

func (I *Inventory) DeleteRequest(invVect *wire.InvVect) (ok bool) {
	I.mutex.Lock()
	defer I.mutex.Unlock()

	if I.requested.Exists(invVect) {
		I.requested.Delete(invVect) 
		return true
	}
	return false
}

//TODO we actually end up decoding the message and then encoding it again when
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

func (I *Inventory) FilterKnown(inv []*wire.InvVect) []*wire.InvVect {
	I.mutex.Lock()
	defer I.mutex.Unlock()

	return I.known.Filter(inv)
}

func (I *Inventory) FilterRequested(inv []*wire.InvVect) []*wire.InvVect {
	I.mutex.Lock()
	defer I.mutex.Unlock()

	return I.known.Filter(inv)
}

func NewInventory(db database.Db) *Inventory{
	return &Inventory{
		db : db, 
		known : NewMruInventoryMap(maxKnownInventory), 
		requested : NewMruInventoryMap(maxKnownInventory), 
	}
}
