package wire_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/jimmysong/bmd/wire"
)

// TestPubKey tests the PubKey API.
func TestPubKeyH(t *testing.T) {

	pubKeyStr := "14a0810ac680a3eb3f82edc814a0810ac680a3eb3f82edc814a0810ac680a3eb3f82edc8473883a8"
	pubKey, err := wire.NewPubKeyFromStr(pubKeyStr)
	if err != nil {
		t.Errorf("NewPubKeyFromStr: %v", err)
	}

	buf := []byte{
		0x79, 0xa6, 0x1a, 0xdb, 0xc6, 0xe5, 0xa2, 0xe1,
		0x39, 0xd2, 0x71, 0x3a, 0x54, 0x6e, 0xc7, 0xc8,
		0x79, 0xa6, 0x1a, 0xdb, 0xc6, 0xe5, 0xa2, 0xe1,
		0x39, 0xd2, 0x71, 0x3a, 0x54, 0x6e, 0xc7, 0xc8,
		0x79, 0xa6, 0x1a, 0xdb, 0xc6, 0xe5, 0xa2, 0xe1,
		0x39, 0xd2, 0x71, 0x3a, 0x54, 0x6e, 0xc7, 0xc8,
		0x79, 0xa6, 0x1a, 0xdb, 0xc6, 0xe5, 0xa2, 0xe1,
		0x39, 0xd2, 0x71, 0x3a, 0x54, 0x6e, 0xc7, 0xc8,
	}

	hash, err := wire.NewPubKey(buf)
	if err != nil {
		t.Errorf("NewPubKey: unexpected error %v", err)
	}

	// Ensure proper size.
	if len(hash) != wire.PubKeySize {
		t.Errorf("NewPubKey: hash length mismatch - got: %v, want: %v",
			len(hash), wire.PubKeySize)
	}

	// Ensure contents match.
	if !bytes.Equal(hash[:], buf) {
		t.Errorf("NewPubKey: hash contents mismatch - got: %v, want: %v",
			hash[:], buf)
	}

	// Ensure contents of hash of block 234440 don't match 234439.
	if hash.IsEqual(pubKey) {
		t.Errorf("IsEqual: hash contents should not match - got: %v, want: %v",
			hash, pubKey)
	}

	// Set hash from byte slice and ensure contents match.
	err = hash.SetBytes(pubKey.Bytes())
	if err != nil {
		t.Errorf("SetBytes: %v", err)
	}
	if !hash.IsEqual(pubKey) {
		t.Errorf("IsEqual: hash contents mismatch - got: %v, want: %v",
			hash, pubKey)
	}

	// Invalid size for SetBytes.
	err = hash.SetBytes([]byte{0x00})
	if err == nil {
		t.Errorf("SetBytes: failed to received expected err - got: nil")
	}

	// Invalid size for NewPubKey.
	invalidHash := make([]byte, wire.PubKeySize+1)
	_, err = wire.NewPubKey(invalidHash)
	if err == nil {
		t.Errorf("NewPubKey: failed to received expected err - got: nil")
	}
}

// TestPubKeyString  tests the stringized output for sha hashes.
func TestPubKeyString(t *testing.T) {
	wantStr := "d2b00432346c3f1f3986da1afd33e506d2b00432346c3f1f3986da1afd33e506d2b00432346c3f1f3986da1afd33e506d2b00432346c3f1f3986da1afd33e506"
	hash := wire.PubKey([wire.PubKeySize]byte{ // Make go vet happy.
		0x06, 0xe5, 0x33, 0xfd, 0x1a, 0xda, 0x86, 0x39,
		0x1f, 0x3f, 0x6c, 0x34, 0x32, 0x04, 0xb0, 0xd2,
		0x06, 0xe5, 0x33, 0xfd, 0x1a, 0xda, 0x86, 0x39,
		0x1f, 0x3f, 0x6c, 0x34, 0x32, 0x04, 0xb0, 0xd2,
		0x06, 0xe5, 0x33, 0xfd, 0x1a, 0xda, 0x86, 0x39,
		0x1f, 0x3f, 0x6c, 0x34, 0x32, 0x04, 0xb0, 0xd2,
		0x06, 0xe5, 0x33, 0xfd, 0x1a, 0xda, 0x86, 0x39,
		0x1f, 0x3f, 0x6c, 0x34, 0x32, 0x04, 0xb0, 0xd2,
	})

	hashStr := hash.String()
	if hashStr != wantStr {
		t.Errorf("String: wrong hash string - got %v, want %v",
			hashStr, wantStr)
	}
}

// TestNewPubKeyFromStr executes tests against the NewPubKeyFromStr function.
func TestNewPubKeyFromStr(t *testing.T) {
	tests := []struct {
		in   string
		want wire.PubKey
		err  error
	}{
		// Empty string.
		{
			"",
			wire.PubKey{},
			nil,
		},

		// Single digit hash.
		{
			"1",
			wire.PubKey([wire.PubKeySize]byte{ // Make go vet happy.
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			}),
			nil,
		},

		{
			"a60840790ba1d475d01367e7c723da941069e9dc",
			wire.PubKey([wire.PubKeySize]byte{ // Make go vet happy.
				0xdc, 0xe9, 0x69, 0x10, 0x94, 0xda, 0x23, 0xc7,
				0xe7, 0x67, 0x13, 0xd0, 0x75, 0xd4, 0xa1, 0x0b,
				0x79, 0x40, 0x08, 0xa6,
			}),
			nil,
		},

		// Pub Key that is too long.
		{
			"012345678901234567890123456789012345678901234567890123456789123450123456789012345678901234567890123456789012345678901234567891234501234567890123456789012345678901234567890123456789012345678912345",
			wire.PubKey{},
			wire.ErrPubKeyStrSize,
		},

		// Pub Key that is contains non-hex chars.
		{
			"abcdefg",
			wire.PubKey{},
			hex.InvalidByteError('g'),
		},
	}

	unexpectedErrStr := "NewPubKeyFromStr #%d failed to detect expected error - got: %v want: %v"
	unexpectedResultStr := "NewPubKeyFromStr #%d got: %v want: %v"
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result, err := wire.NewPubKeyFromStr(test.in)
		if err != test.err {
			t.Errorf(unexpectedErrStr, i, err, test.err)
			continue
		} else if err != nil {
			// Got expected error. Move on to the next test.
			continue
		}
		if !test.want.IsEqual(result) {
			t.Errorf(unexpectedResultStr, i, result, &test.want)
			continue
		}
	}
}
