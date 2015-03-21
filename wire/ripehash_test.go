package wire_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/monetas/bmd/wire"
)

// TestRipeHash tests the RipeHash API.
func TestRipeHash(t *testing.T) {

	ripeStr := "14a0810ac680a3eb3f82edc8"
	ripe, err := wire.NewRipeHashFromStr(ripeStr)
	if err != nil {
		t.Errorf("NewRipeHashFromStr: %v", err)
	}

	buf := []byte{
		0x79, 0xa6, 0x1a, 0xdb, 0xc6, 0xe5, 0xa2, 0xe1,
		0x39, 0xd2, 0x71, 0x3a, 0x54, 0x6e, 0xc7, 0xc8,
		0x75, 0x63, 0x2e, 0x75,
	}

	hash, err := wire.NewRipeHash(buf)
	if err != nil {
		t.Errorf("NewRipeHash: unexpected error %v", err)
	}

	// Ensure proper size.
	if len(hash) != wire.RipeHashSize {
		t.Errorf("NewRipeHash: hash length mismatch - got: %v, want: %v",
			len(hash), wire.RipeHashSize)
	}

	// Ensure contents match.
	if !bytes.Equal(hash[:], buf) {
		t.Errorf("NewRipeHash: hash contents mismatch - got: %v, want: %v",
			hash[:], buf)
	}

	if hash.IsEqual(ripe) {
		t.Errorf("IsEqual: hash contents should not match - got: %v, want: %v",
			hash, ripe)
	}

	// Set hash from byte slice and ensure contents match.
	err = hash.SetBytes(ripe.Bytes())
	if err != nil {
		t.Errorf("SetBytes: %v", err)
	}
	if !hash.IsEqual(ripe) {
		t.Errorf("IsEqual: hash contents mismatch - got: %v, want: %v",
			hash, ripe)
	}

	// Invalid size for SetBytes.
	err = hash.SetBytes([]byte{0x00})
	if err == nil {
		t.Errorf("SetBytes: failed to received expected err - got: nil")
	}

	// Invalid size for NewRipeHash.
	invalidHash := make([]byte, wire.RipeHashSize+1)
	_, err = wire.NewRipeHash(invalidHash)
	if err == nil {
		t.Errorf("NewRipeHash: failed to received expected err - got: nil")
	}
}

// TestRipeHashString  tests the stringized output for sha hashes.
func TestRipeHashString(t *testing.T) {
	wantStr := "ecaad478d2b00432346c3f1f3986da1afd33e506"
	hash := wire.RipeHash([wire.RipeHashSize]byte{ // Make go vet happy.
		0x06, 0xe5, 0x33, 0xfd, 0x1a, 0xda, 0x86, 0x39,
		0x1f, 0x3f, 0x6c, 0x34, 0x32, 0x04, 0xb0, 0xd2,
		0x78, 0xd4, 0xaa, 0xec,
	})

	hashStr := hash.String()
	if hashStr != wantStr {
		t.Errorf("String: wrong hash string - got %v, want %v",
			hashStr, wantStr)
	}
}

// TestNewRipeHashFromStr executes tests against the NewRipeHashFromStr function.
func TestNewRipeHashFromStr(t *testing.T) {
	tests := []struct {
		in   string
		want wire.RipeHash
		err  error
	}{
		// Empty string.
		{
			"",
			wire.RipeHash{},
			nil,
		},

		// Single digit hash.
		{
			"1",
			wire.RipeHash([wire.RipeHashSize]byte{ // Make go vet happy.
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
			}),
			nil,
		},

		{
			"a60840790ba1d475d01367e7c723da941069e9dc",
			wire.RipeHash([wire.RipeHashSize]byte{ // Make go vet happy.
				0xdc, 0xe9, 0x69, 0x10, 0x94, 0xda, 0x23, 0xc7,
				0xe7, 0x67, 0x13, 0xd0, 0x75, 0xd4, 0xa1, 0x0b,
				0x79, 0x40, 0x08, 0xa6,
			}),
			nil,
		},

		// Hash string that is too long.
		{
			"01234567890123456789012345678901234567890123456789012345678912345",
			wire.RipeHash{},
			wire.ErrRipeHashStrSize,
		},

		// Hash string that is contains non-hex chars.
		{
			"abcdefg",
			wire.RipeHash{},
			hex.InvalidByteError('g'),
		},
	}

	unexpectedErrStr := "NewRipeHashFromStr #%d failed to detect expected error - got: %v want: %v"
	unexpectedResultStr := "NewRipeHashFromStr #%d got: %v want: %v"
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result, err := wire.NewRipeHashFromStr(test.in)
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
