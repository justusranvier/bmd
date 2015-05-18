package bmpeer_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/monetas/bmd/bmpeer"
	"github.com/monetas/bmutil"
	"github.com/monetas/bmutil/wire"
)

func TestNewInventory(t *testing.T) {
	if bmpeer.NewInventory(NewMockDb()) == nil {
		t.Error("Nil inventory returned.")
	}
}

func TestIsAddKnownInventory(t *testing.T) {
	inventory := bmpeer.NewInventory(NewMockDb())

	a := &wire.InvVect{Hash: *randomShaHash()}

	if inventory.IsKnownInventory(a) {
		t.Error("Map should be empty.")
	}

	inventory.AddKnownInventory(a)
	if !inventory.IsKnownInventory(a) {
		t.Error("Map should not be empty..")
	}
}

func TestAddDeleteRequest(t *testing.T) {
	inventory := bmpeer.NewInventory(NewMockDb())

	a := &wire.InvVect{Hash: *randomShaHash()}

	if inventory.DeleteRequest(a) {
		t.Error("Map should be empty.")
	}

	inventory.AddRequest(a)
	if !inventory.DeleteRequest(a) {
		t.Error("Map should not be empty..")
	}
}

func TestRetrieveObjectAndData(t *testing.T) {
	db := NewMockDb()
	inventory := bmpeer.NewInventory(db)

	// An object that is not in the database.
	notThere := &wire.InvVect{Hash: *randomShaHash()}

	// An invalid object that will be in the database (normally this should not happen).
	badData := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}
	hash, _ := wire.NewShaHash(bmutil.CalcInventoryHash(badData))
	badInv := &wire.InvVect{Hash: *hash}
	db.InsertObject(badData)

	// A valid object that will be in the database.
	message := wire.NewMsgUnknownObject(345, time.Now(), wire.ObjectType(4), 1, 1, []byte{77, 82, 53, 48, 96, 1})
	goodData := wire.EncodeMessage(message)
	goodInv := &wire.InvVect{Hash: *wire.MessageHash(message)}
	db.InsertObject(goodData)

	// Retrieve objects that are not in the database.
	if inventory.RetrieveObject(notThere) != nil {
		t.Error("Object returned that should not have been in the database.")
	}
	if inventory.RetrieveData(notThere) != nil {
		t.Error("Data returned that should not have been in the database.")
	}

	// Retrieve invalid objects from the database.
	if inventory.RetrieveObject(badInv) != nil {
		t.Error("Object returned that should have been detected to be invalid.")
	}
	if !bytes.Equal(inventory.RetrieveData(badInv), badData) {
		t.Error("No data returned that should have been in the database.")
	}

	// Retrieve good objects from the database.
	if inventory.RetrieveObject(goodInv) == nil {
		t.Error("No object returned.")
	}
	if !bytes.Equal(inventory.RetrieveData(goodInv), goodData) {
		t.Error("No data returned that should have been in the database.")
	}
}

func TestFilterKnown(t *testing.T) {
	inventory := bmpeer.NewInventory(NewMockDb())

	a := &wire.InvVect{Hash: *randomShaHash()}
	b := &wire.InvVect{Hash: *randomShaHash()}
	c := &wire.InvVect{Hash: *randomShaHash()}

	inventory.AddKnownInventory(a)

	ret := inventory.FilterKnown([]*wire.InvVect{a, b, c})

	if len(ret) != 2 {
		t.Errorf("Filtered list has the wrong size. Got %d expected %d.", len(ret), 2)
	}
	if ret[0] != b {
		t.Error("Wrong filtered list returned.")
	}

	if !inventory.IsKnownInventory(a) {
		t.Error("Element should be present.")
	}
	if !inventory.IsKnownInventory(b) {
		t.Error("Element should be present.")
	}
	if !inventory.IsKnownInventory(c) {
		t.Error("Element should be present.")
	}
}

func TestFilterRequested(t *testing.T) {
	inventory := bmpeer.NewInventory(NewMockDb())

	a := &wire.InvVect{Hash: *randomShaHash()}
	b := &wire.InvVect{Hash: *randomShaHash()}
	c := &wire.InvVect{Hash: *randomShaHash()}

	inventory.AddRequest(a)

	ret := inventory.FilterRequested([]*wire.InvVect{a, b, c})

	if len(ret) != 2 {
		t.Errorf("Filtered list has the wrong size. Got %d expected %d.", len(ret), 2)
	}
	if ret[0] != b {
		t.Error("Wrong filtered list returned.")
	}

	if !inventory.DeleteRequest(a) {
		t.Error("Element should be present.")
	}
	if !inventory.DeleteRequest(b) {
		t.Error("Element should be present.")
	}
	if !inventory.DeleteRequest(c) {
		t.Error("Element should be present.")
	}
}
