// Originally derived from: btcsuite/btcd/database/interface_test.go
// Copyright (c) 2013-2015 Conformal Systems LLC.

// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/monetas/bmd/database"
	"github.com/monetas/bmutil"
	"github.com/monetas/bmutil/wire"
)

// testContext is used to store context information about a running test which
// is passed into helper functions.
type testContext struct {
	t      *testing.T
	dbType string
	db     database.Db
}

// create a new database after clearing out the old one and return the teardown
// function
func (tc *testContext) newDb() func() {
	// create fresh database
	db, teardown, err := createDB(tc.dbType, "test", true)
	if err != nil {
		tc.t.Fatalf("Failed to create test database (%s) %v", tc.dbType, err)
	}
	tc.db = db
	return teardown
}

var expires = time.Now().Add(10 * time.Minute)
var expired = time.Now().Add(-10 * time.Minute).Add(-3 * time.Hour)

// A set of pub keys to create fake objects for testing the database.
var pubkey = []wire.PubKey{
	wire.PubKey([wire.PubKeySize]byte{
		23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38,
		39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54,
		55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 70,
		71, 72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85, 86}),
	wire.PubKey([wire.PubKeySize]byte{
		87, 88, 89, 90, 91, 92, 93, 94, 95, 96, 97, 98, 99, 100, 101, 102,
		103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118,
		119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 129, 130, 131, 132, 133, 134,
		135, 136, 137, 138, 139, 140, 141, 142, 143, 144, 145, 146, 147, 148, 149, 150}),
	wire.PubKey([wire.PubKeySize]byte{
		54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69,
		70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85,
		86, 87, 88, 89, 90, 91, 92, 93, 94, 95, 96, 97, 98, 99, 100, 101,
		102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117}),
	wire.PubKey([wire.PubKeySize]byte{
		118, 119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 129, 130, 131, 132, 133,
		134, 135, 136, 137, 138, 139, 140, 141, 142, 143, 144, 145, 146, 147, 148, 149,
		150, 151, 152, 153, 154, 155, 156, 157, 158, 159, 160, 161, 162, 163, 164, 165,
		166, 167, 168, 169, 170, 171, 172, 173, 174, 175, 176, 177, 178, 179, 180, 181}),
}

var shahash = []wire.ShaHash{
	wire.ShaHash([wire.HashSize]byte{
		98, 99, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113,
		114, 115, 116, 117, 118, 119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 129}),
	wire.ShaHash([wire.HashSize]byte{
		100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115,
		116, 117, 118, 119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 129, 130, 131}),
}

var ripehash = []wire.RipeHash{
	wire.RipeHash([wire.RipeHashSize]byte{
		78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 91, 92, 93, 94, 95, 96, 97}),
	wire.RipeHash([wire.RipeHashSize]byte{
		80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 91, 92, 93, 94, 95, 96, 97, 98, 99}),
}

// Some bitmessage objects that we use for testing. Two of each.
var testObj = [][]wire.Message{
	[]wire.Message{
		wire.NewMsgGetPubKey(654, expires, 4, 1, &ripehash[0], &shahash[0]),
		wire.NewMsgGetPubKey(654, expired, 4, 1, &ripehash[1], &shahash[1]),
	},
	[]wire.Message{
		wire.NewMsgPubKey(543, expires, 4, 1, 2, &pubkey[0], &pubkey[1], 3, 5,
			[]byte{4, 5, 6, 7, 8, 9, 10}, &shahash[0], []byte{11, 12, 13, 14, 15, 16, 17, 18}),
		wire.NewMsgPubKey(543, expired, 4, 1, 2, &pubkey[2], &pubkey[3], 3, 5,
			[]byte{4, 5, 6, 7, 8, 9, 10}, &shahash[1], []byte{11, 12, 13, 14, 15, 16, 17, 18}),
	},
	[]wire.Message{
		wire.NewMsgMsg(765, expires, 1, 1,
			[]byte{90, 87, 66, 45, 3, 2, 120, 101, 78, 78, 78, 7, 85, 55, 2, 23},
			1, 1, 2, &pubkey[0], &pubkey[1], 3, 5, &ripehash[0], 1,
			[]byte{21, 22, 23, 24, 25, 26, 27, 28},
			[]byte{20, 21, 22, 23, 24, 25, 26, 27},
			[]byte{19, 20, 21, 22, 23, 24, 25, 26}),
		wire.NewMsgMsg(765, expired, 1, 1,
			[]byte{90, 87, 66, 45, 3, 2, 120, 101, 78, 78, 78, 7, 85, 55},
			1, 1, 2, &pubkey[2], &pubkey[3], 3, 5, &ripehash[1], 1,
			[]byte{21, 22, 23, 24, 25, 26, 27, 28, 79},
			[]byte{20, 21, 22, 23, 24, 25, 26, 27, 79},
			[]byte{19, 20, 21, 22, 23, 24, 25, 26, 79}),
	},
	[]wire.Message{
		wire.NewMsgBroadcast(876, expires, 1, 1, &shahash[0],
			[]byte{90, 87, 66, 45, 3, 2, 120, 101, 78, 78, 78, 7, 85, 55, 2, 23},
			1, 1, 2, &pubkey[0], &pubkey[1], 3, 5, &ripehash[1], 1,
			[]byte{27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41},
			[]byte{42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56}),
		wire.NewMsgBroadcast(876, expired, 1, 1, &shahash[1],
			[]byte{90, 87, 66, 45, 3, 2, 120, 101, 78, 78, 78, 7, 85, 55},
			1, 1, 2, &pubkey[2], &pubkey[3], 3, 5, &ripehash[0], 1,
			[]byte{27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40},
			[]byte{42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55}),
	},
	[]wire.Message{
		wire.NewMsgUnknownObject(345, expires, wire.ObjectType(4), 1, 1, []byte{77, 82, 53, 48, 96, 1}),
		wire.NewMsgUnknownObject(987, expired, wire.ObjectType(4), 1, 1, []byte{1, 2, 3, 4, 5, 0, 6, 7, 8, 9, 100}),
	},
	[]wire.Message{
		wire.NewMsgUnknownObject(7288, expires, wire.ObjectType(5), 1, 1, []byte{0, 0, 0, 0, 1, 0, 0}),
		wire.NewMsgUnknownObject(7288, expired, wire.ObjectType(5), 1, 1, []byte{0, 0, 0, 0, 0, 0, 0, 99, 98, 97}),
	},
}

func msgToBytes(msg wire.Message) []byte {
	buf := &bytes.Buffer{}
	msg.Encode(buf)
	return buf.Bytes()
}

func testSync(tc *testContext) {
	teardown := tc.newDb()
	defer teardown()

	err := tc.db.Sync()
	if err != nil {
		tc.t.Errorf("Sync (%s): got error %v", tc.dbType, err)
	}
}

// testObject tests InsertObject, ExistsObject, FetchObjectByHash, and RemoveObject
func testObject(tc *testContext) {
	teardown := tc.newDb()
	defer teardown()

	// test inserting invalid object
	_, err := tc.db.InsertObject([]byte{0x00, 0x00})
	if err == nil {
		tc.t.Errorf("InsertObject (%s): inserting invalid object, expected"+
			" error got none", tc.dbType)
	}

	for i, object := range testObj {
		data := msgToBytes(object[0])
		_, err := tc.db.InsertObject(data)
		if err != nil {
			tc.t.Errorf("InsertObject (%s): object #%d,"+
				" got error %v", tc.dbType, i, err)
		}

		hash, _ := wire.NewShaHash(bmutil.CalcInventoryHash(msgToBytes(object[0])))

		exists, err := tc.db.ExistsObject(hash)
		if err != nil {
			tc.t.Errorf("ExistsObject (%s): object #%d,"+
				" got error %v", tc.dbType, i, err)
		}
		if !exists {
			tc.t.Errorf("ExistsObject (%s): object #%d should be in db"+
				" but it is not", tc.dbType, i)
		}

		testData, err := tc.db.FetchObjectByHash(hash)
		if err != nil {
			tc.t.Errorf("FetchObjectByHash (%s): object #%d, got error %v",
				tc.dbType, i, err)
		}
		if !bytes.Equal(data, testData) {
			tc.t.Errorf("FetchObjectByHash (%s): object #%d,"+
				" data does not match", tc.dbType, i)
		}

		err = tc.db.RemoveObject(hash)
		if err != nil {
			tc.t.Errorf("RemoveObject (%s): object #%d, got error %v",
				tc.dbType, i, err)
		}

		err = tc.db.RemoveObject(hash)
		if err == nil {
			tc.t.Errorf("RemoveObject (%s): object #%d, removing"+
				" nonexistent object, expected error got none", tc.dbType, i)
		}

		exists, err = tc.db.ExistsObject(hash)
		if err != nil {
			tc.t.Errorf("ExistsObject (%s): object #%d, got error %v",
				tc.dbType, i, err)
		}
		if exists {
			tc.t.Errorf("ExistsObject (%s): object #%d, should not exist"+
				" in db but does", tc.dbType, i)
		}

		_, err = tc.db.FetchObjectByHash(hash)
		if err == nil {
			tc.t.Errorf("FetchObjectByHash (%s): object #%d, fetching"+
				" nonexistent object, expected error got none", tc.dbType, i)
		}
	}
}

// testCounter tests FetchObjectByCounter, FetchObjectsFromCounter,
// RemoveObjectByCounter, and GetCounter
func testCounter(tc *testContext) {
	for i := 0; i < len(testObj); i++ { // test objects that aren't unknown
		teardown := tc.newDb()

		objType := wire.ObjectType(i)

		// Test that the counter starts at zero.
		count, err := tc.db.GetCounter(objType)
		if err != nil {
			tc.t.Errorf("GetCounter (%s): object type %s, got error %v.",
				tc.dbType, objType, err)
		}
		if count != 0 {
			tc.t.Errorf("GetCounter (%s): expected 0, got %d", tc.dbType, count)
		}

		// Try to grab an element that is not there.
		_, err = tc.db.FetchObjectByCounter(objType, 1)
		if err == nil {
			tc.t.Errorf("FetchObjectByCounter (%s): fetching nonexistent"+
				" object, expected error got none", tc.dbType)
		}

		// Try to remove an element that is not there.
		err = tc.db.RemoveObjectByCounter(objType, 1)
		if err == nil {
			tc.t.Errorf("RemoveObjectByCounter (%s): removing nonexistent"+
				" object of type %s, expected error got none",
				tc.dbType, objType)
		}

		// Insert an element and make sure the counter comes out correct.
		data := msgToBytes(testObj[i][0])
		count, err = tc.db.InsertObject(data)
		if err != nil {
			tc.t.Errorf("InsertObject (%s): object #%d #%d of type %s,"+
				" got error %v", tc.dbType, i, 0, objType, err)
		}
		if count != 1 {
			tc.t.Errorf("InsertObject (%s): for counter, expected 1 got %d",
				tc.dbType, count)
		}

		// Try to fetch an object that should be there now.
		testData, err := tc.db.FetchObjectByCounter(objType, 1)
		if err != nil {
			tc.t.Errorf("FetchObjectByCounter (%s): fetching object"+
				" of type %s, got error %v", tc.dbType, objType, err)
		}
		if !bytes.Equal(testData, data) {
			tc.t.Errorf("FetchObjectByCounter (%s): data mismatch", tc.dbType)
		}

		data1 := msgToBytes(testObj[i][1])
		count, err = tc.db.InsertObject(data1)
		if err != nil {
			tc.t.Errorf("InsertObject (%s): object #%d #%d of type %s,"+
				" got error %v", tc.dbType, i, 1, objType, err)
		}
		if count != 2 {
			tc.t.Errorf("InsertObject (%s): object #%d #%d of type %s,"+
				" count error, expected 2, got %d", tc.dbType, i, 1,
				objType, count)
		}

		// Try fetching the new object.
		testData, err = tc.db.FetchObjectByCounter(objType, 2)
		if err != nil {
			tc.t.Errorf("FetchObjectByCounter (%s): fetching existing object"+
				" of type %s, got error %v", tc.dbType, objType, err)
		}
		if !bytes.Equal(testData, data1) {
			tc.t.Errorf("FetchObjectByCounter (%s): data mismatch", tc.dbType)
		}

		// Test that the counter has incremented.
		count, err = tc.db.GetCounter(objType)
		if err != nil {
			tc.t.Errorf("GetCounter (%s): object type %s, got error %v",
				tc.dbType, objType, err)
		}
		if count != 2 {
			tc.t.Errorf("GetCounter (%s): object type %s, expected 2, got %d",
				tc.dbType, objType, count)
		}

		// Test FetchObjectsFromCounter for various input values.
		fetch, n, err := tc.db.FetchObjectsFromCounter(objType, 3, 2)
		if err != nil {
			tc.t.Errorf("FetchObjectsFromCounter (%s): object type %s, "+
				"expected empty slice got error %v", tc.dbType, objType, err)
		}
		if len(fetch) != 0 {
			tc.t.Errorf("FetchObjectsFromCounter (%s): object type %s, "+
				"incorrect slice size, expected 0, got %d", tc.dbType,
				objType, len(fetch))
		}
		if n != 0 {
			tc.t.Errorf("FetchObjectsFromCounter (%s): object type %s, "+
				"incorrect counter value, expected 0, got %d", tc.dbType,
				objType, n)
		}

		fetch, n, err = tc.db.FetchObjectsFromCounter(objType, 1, 3)
		if err != nil {
			tc.t.Errorf("FetchObjectsFromCounter (%s): object type %s,"+
				" got error %v", tc.dbType, objType, err)
		}
		if len(fetch) != 2 {
			tc.t.Errorf("FetchObjectsFromCounter (%s): object type %s,"+
				" incorrect slice size: expected 2, got %d", tc.dbType,
				objType, len(fetch))
		}
		if n != 2 {
			tc.t.Errorf("FetchObjectsFromCounter (%s): object type %s,"+
				" incorrect counter value: expected 2, got %d", tc.dbType,
				objType, n)
		}

		fetch, n, err = tc.db.FetchObjectsFromCounter(objType, 1, 1)
		if err != nil {
			tc.t.Errorf("FetchObjectsFromCounter (%s): object type %s,"+
				" got error %v", tc.dbType, objType, err)
		}
		if len(fetch) != 1 {
			tc.t.Errorf("FetchObjectsFromCounter (%s): object type %s,"+
				" incorrect slice size: expected 1, got %d", tc.dbType,
				objType, len(fetch))
		}
		if n != 1 {
			tc.t.Errorf("FetchObjectsFromCounter (%s): object type %s,"+
				" incorrect counter value: expected 1, got %d", tc.dbType,
				objType, n)
		}

		fetch, n, err = tc.db.FetchObjectsFromCounter(objType, 2, 3)
		if err != nil {
			tc.t.Errorf("FetchObjectsFromCounter (%s): object type %s,"+
				" got error %v", tc.dbType, objType, err)
		}
		if len(fetch) != 1 {
			tc.t.Errorf("FetchObjectsFromCounter (%s): object type %s,"+
				" incorrect slice size: expected 2, got %d", tc.dbType,
				objType, len(fetch))
		}
		if n != 2 {
			tc.t.Errorf("FetchObjectsFromCounter (%s): object type %s,"+
				" incorrect counter value, expected 2 got %d", tc.dbType,
				objType, n)
		}

		// Test that objects can be removed after being added.
		err = tc.db.RemoveObjectByCounter(objType, 1)
		if err != nil {
			tc.t.Errorf("RemoveObjectByCounter (%s): object type %s,"+
				" got error %v", tc.dbType, objType, err)
		}

		// Removing an object that has already been removed.
		err = tc.db.RemoveObjectByCounter(objType, 1)
		if err == nil {
			tc.t.Errorf("RemoveObjectByCounter (%s): removing already removed"+
				" object of type %s, got no error", tc.dbType, objType)
		}

		// Removing a nonexistent object
		err = tc.db.RemoveObjectByCounter(objType, 3)
		if err == nil {
			tc.t.Errorf("RemoveObjectByCounter (%s): removing nonexistent"+
				" object of type %s, got no error", tc.dbType, objType)
		}

		// Test that objects cannot be fetched after being removed.
		_, err = tc.db.FetchObjectByCounter(objType, 1)
		if err == nil {
			tc.t.Errorf("FetchObjectByCounter (%s): fetching nonexistent"+
				" object of type %s, got no error", tc.dbType, objType)
		}

		// Test that the counter values returned by FetchObjectsFromCounter are
		// correct after some objects have been removed.
		fetch, n, err = tc.db.FetchObjectsFromCounter(objType, 1, 3)
		if err != nil {
			tc.t.Errorf("FetchObjectByCounter (%s): object type %s,"+
				" got error %v", tc.dbType, objType, err)
		}
		if len(fetch) != 1 {
			tc.t.Errorf("FetchObjectByCounter (%s): object type %s,"+
				" incorrect slice size, expected 1 got %d", tc.dbType,
				objType, len(fetch))
		}
		if n != 2 {
			tc.t.Errorf("FetchObjectByCounter (%s): object type %s,"+
				" incorrect counter value, expected 2 got %d", tc.dbType,
				objType, n)
		}

		// close database
		teardown()
	}
}

// testPubKey tests inserting public key messages, FetchIdentityByAddress
// and RemovePubKey
func testPubKey(tc *testContext) {
	teardown := tc.newDb()
	defer teardown()

	// test inserting invalid public key
	invalidPubkey := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x47, 0xd8, // 83928 nonce
		0x00, 0x00, 0x00, 0x00, 0x49, 0x5f, 0xab, 0x29, // 64-bit Timestamp
		0x00, 0x00, 0x00, 0x01, // Object Type
		0x02,                   // Version
		0x01,                   // Stream Number
		0x00, 0x00, 0x00, 0x00, // Behavior
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Signing Key
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Encrypt Key
	}
	_, err := tc.db.InsertObject(invalidPubkey)
	if err != nil {
		tc.t.Errorf("InsertObject (%s): inserting invalid pubkey, got error %v",
			tc.dbType, err)
	}
	count, _ := tc.db.GetCounter(wire.ObjectTypePubKey)
	if count != 1 {
		tc.t.Errorf("GetCounter (%s): got %d expected %d", tc.dbType, count, 1)
	}

	// test FetchIdentityByAddress for an address that does not exist
	addr, err := bmutil.DecodeAddress("BM-2cV9RshwouuVKWLBoyH5cghj3kMfw5G7BJ")
	if err != nil {
		tc.t.Fatalf("DecodeAddress failed, got error %v", err)
	}
	_, err = tc.db.FetchIdentityByAddress(addr)
	if err == nil {
		tc.t.Fatalf("FetchIdentityByAddress (%s): expected error got none",
			tc.dbType)
	}

	// test RemovePubKey for an address that does not exist
	tag, _ := wire.NewShaHash(addr.Tag())
	err = tc.db.RemovePubKey(tag)
	if err == nil {
		tc.t.Fatalf("RemovePubKey (%s): expected error got none", tc.dbType)
	}

	// test inserting valid v2 public key
	// test inserting valid v3 public key
	// test inserting valid v4 public key

	// test FetchIdentityByAddress for an address that exists in the database
	// v2 address
	// v3 address
	// v4 address

	// test RemovePubKey for an address that exists in the database

	// test FetchIdentityByAddress for an address that was removed
	// v2 address
	// v3 address
	// v4 address

	// test RemovePubKey for an address that was removed
}

// tests FetchRandomInvHashes and FilterObjects
func testFilters(tc *testContext) {
	teardown := tc.newDb()
	defer teardown()

	for _, messages := range testObj {
		for _, message := range messages {
			_, _ = tc.db.InsertObject(msgToBytes(message))
		}
	}

	allFilter := func(*wire.ShaHash, []byte) bool {
		return true
	}
	noneFilter := func(*wire.ShaHash, []byte) bool {
		return false
	}
	unknownObjFilter := func(hash *wire.ShaHash, obj []byte) bool {
		msg := new(wire.MsgUnknownObject)
		msg.Decode(bytes.NewReader(obj))
		if msg.ObjectType >= wire.ObjectType(4) {
			return true
		} else {
			return false
		}
	}
	expiredFilter := func(hash *wire.ShaHash, obj []byte) bool {
		_, expiresTime, _, _, _, err := wire.DecodeMsgObjectHeader(
			bytes.NewReader(obj))
		if err != nil {
			return false
		}
		if time.Now().Add(-time.Hour * 3).After(expiresTime) { // expired
			return true
		} else {
			return false
		}
	}

	type randomInvHashesTest struct {
		count         uint64
		filterFunc    func(*wire.ShaHash, []byte) bool
		expectedCount int
	}

	randomInvHashesTests := []randomInvHashesTest{
		{2, allFilter, 2},
		{15, allFilter, 12},
		{5, noneFilter, 0},
		{0, noneFilter, 0},
		{8, unknownObjFilter, 4},
		{1, unknownObjFilter, 1},
		{7, expiredFilter, 6},
		{5, expiredFilter, 5},
	}

	for i, tst := range randomInvHashesTests {
		hashes, err := tc.db.FetchRandomInvHashes(tst.count, tst.filterFunc)
		if err != nil {
			tc.t.Fatalf("FetchRandomInvHashes (%s): test #%d, got error %v",
				tc.dbType, i, err)
		}
		if len(hashes) != tst.expectedCount {
			tc.t.Errorf("FetchRandomInvHashes (%s): test #%d, got length %d"+
				" expected %d", tc.dbType, i, len(hashes), tst.expectedCount)
		}
	}

	type filterObjectsTest struct {
		filterFunc    func(*wire.ShaHash, []byte) bool
		expectedCount int
	}

	filterObjectsTests := []filterObjectsTest{
		{allFilter, 12},
		{noneFilter, 0},
		{unknownObjFilter, 4},
		{expiredFilter, 6},
	}

	for i, tst := range filterObjectsTests {
		hashes, err := tc.db.FilterObjects(tst.filterFunc)
		if err != nil {
			tc.t.Fatalf("FilterObjects (%s): test #%d, got error %v",
				tc.dbType, i, err)
		}
		if len(hashes) != tst.expectedCount {
			tc.t.Errorf("FilterObjects (%s): test #%d, got length %d"+
				" expected %d", tc.dbType, i, len(hashes), tst.expectedCount)
		}
	}

}

func testRemoveExpiredObjects(tc *testContext) {
	teardown := tc.newDb()
	defer teardown()

	for _, messages := range testObj {
		for _, message := range messages {
			_, _ = tc.db.InsertObject(msgToBytes(message))
		}
	}

	tc.db.RemoveExpiredObjects()

	for i, messages := range testObj {
		for j, message := range messages {
			hash, _ := wire.NewShaHash(bmutil.CalcInventoryHash(msgToBytes(message)))

			exists, _ := tc.db.ExistsObject(hash)

			// The object should have been deleted
			if j == 1 {
				if exists {
					tc.t.Errorf("RemoveExpiredObjects (%s): message #%d #%d"+
						" should have been deleted, but was not",
						tc.dbType, i, j)
				}
			} else {
				if !exists {
					tc.t.Errorf("RemoveExpiredObjects (%s): message #%d #%d"+
						" should not have been deleted, but was",
						tc.dbType, i, j)
				}
			}
		}
	}
}

// testInterface tests performs tests for the various interfaces of the database
// package which require state in the database for the given database type.
func testInterface(t *testing.T, dbType string) {
	// Create a test context to pass around.
	context := &testContext{t: t, dbType: dbType}

	testObject(context)
	testPubKey(context)
	testRemoveExpiredObjects(context)
	testCounter(context)
	testFilters(context)
	testSync(context)
}
