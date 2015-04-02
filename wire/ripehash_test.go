package wire_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/monetas/bmd/wire"
)

// TestRipeHash tests the RipeHash API.
func TestRipeHash(t *testing.T) {

	ripeStr := "385e17e3f2047ca81f71ac604c6da1c2a311f384"
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
	wantStr := "06e533fd1ada86391f3f6c343204b0d278d4aaec"
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
			wire.ErrRipeHashStrSize,
		},

		// Single digit hash.
		{
			"1",
			wire.RipeHash([wire.RipeHashSize]byte{ // Make go vet happy.
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
			}),
			wire.ErrRipeHashStrSize,
		},

		{
			"65eaa9561128a9fd5df1995bd9e20bde38546a59",
			wire.RipeHash([wire.RipeHashSize]byte{ // Make go vet happy.
				0x65, 0xEA, 0xA9, 0x56, 0x11, 0x28, 0xA9, 0xFD,
				0x5D, 0xF1, 0x99, 0x5B, 0xD9, 0xE2, 0x0B, 0xDE,
				0x38, 0x54, 0x6A, 0x59,
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
			"65gaa9561128a9fd5df1995bd9e20bde38546a59",
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
