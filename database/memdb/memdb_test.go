package memdb_test

import (
	"bytes"
	"testing"

	"github.com/monetas/bmd/database"
	_ "github.com/monetas/bmd/database/memdb"
	"github.com/monetas/bmutil/wire"
)

// TestClosed ensures that the correct errors are returend when the public
// functions are called on a closed database.
func TestClosed(t *testing.T) {
	db, err := database.CreateDB("memdb")
	if err != nil {
		t.Fatalf("Failed to open test database %v", err)
	}

	db.Close()
	hash, _ := wire.NewShaHash(bytes.Repeat([]byte{0}, 32))

	if err := db.Sync(); err != database.ErrDbClosed {
		t.Errorf("Sync: unexpected error %v", err)
	}

	if err := db.Close(); err != database.ErrDbClosed {
		t.Errorf("Close: unexpected error %v", err)
	}

	if err := db.RollbackClose(); err != database.ErrDbClosed {
		t.Errorf("RollbackClose: unexpected error %v", err)
	}

	if _, err := db.InsertObject([]byte{0, 0}); err != database.ErrDbClosed {
		t.Errorf("InsertObject: unexpected error %v", err)
	}

	if _, err = db.ExistsObject(hash); err != database.ErrDbClosed {
		t.Errorf("ExistsObject: unexpected error %v", err)
	}

	if err := db.RemoveObject(hash); err != database.ErrDbClosed {
		t.Errorf("RemoveObject: unexpected error %v", err)
	}

	if _, err := db.FetchObjectByHash(hash); err != database.ErrDbClosed {
		t.Errorf("FetchObjectByHash: unexpected error %v", err)
	}

	if err := db.RemoveExpiredObjects(); err != database.ErrDbClosed {
		t.Errorf("RemoveExpiredObjects: unexpected error %v", err)
	}

	_, err = db.FetchObjectByCounter(wire.ObjectType(4), 1)
	if err != database.ErrDbClosed {
		t.Errorf("FetchObjectByCounter: unexpected error %v", err)
	}

	_, _, err = db.FetchObjectsFromCounter(wire.ObjectType(4), 1, 10)
	if err != database.ErrDbClosed {
		t.Errorf("FetchObjectsFromCounter: unexpected error %v", err)
	}

	if _, err := db.GetCounter(wire.ObjectType(4)); err != database.ErrDbClosed {
		t.Errorf("GetCounter: unexpected error %v", err)
	}

	if err := db.RemoveObjectByCounter(wire.ObjectType(4), 3); err !=
		database.ErrDbClosed {
		t.Errorf("RemoveObjectByCounter: unexpected error %v", err)
	}

	if err := db.RemovePubKey(hash); err != database.ErrDbClosed {
		t.Errorf("RemovePubKey: unexpected error %v", err)
	}

	if _, err := db.FetchIdentityByAddress(nil); err != database.ErrDbClosed {
		t.Errorf("FetchIdentityByAddress: unexpected error %v", err)
	}

	if _, err := db.FilterObjects(nil); err != database.ErrDbClosed {
		t.Errorf("FilterObjects: unexpected error %v", err)
	}

	if _, err := db.FetchRandomInvHashes(0, nil); err != database.ErrDbClosed {
		t.Errorf("FetchRandomInvHashes: unexpected error %v", err)
	}
}
