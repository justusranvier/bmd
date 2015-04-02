package wire_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/monetas/bmd/wire"
)

// TestShaHash tests the ShaHash API.
func TestShaHash(t *testing.T) {

	shaHashStr := "7bbee8758205fe8c8674e3ead895f7d5a144e3d00a47a20070cf0f5c4782b449"
	shaHash, err := wire.NewShaHashFromStr(shaHashStr)
	if err != nil {
		t.Errorf("NewShaHashFromStr: %v", err)
	}

	buf := []byte{
		0x79, 0xa6, 0x1a, 0xdb, 0xc6, 0xe5, 0xa2, 0xe1,
		0x39, 0xd2, 0x71, 0x3a, 0x54, 0x6e, 0xc7, 0xc8,
		0x75, 0x63, 0x2e, 0x75, 0xf1, 0xdf, 0x9c, 0x3f,
		0xa6, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	hash, err := wire.NewShaHash(buf)
	if err != nil {
		t.Errorf("NewShaHash: unexpected error %v", err)
	}

	// Ensure proper size.
	if len(hash) != wire.HashSize {
		t.Errorf("NewShaHash: hash length mismatch - got: %v, want: %v",
			len(hash), wire.HashSize)
	}

	// Ensure contents match.
	if !bytes.Equal(hash[:], buf) {
		t.Errorf("NewShaHash: hash contents mismatch - got: %v, want: %v",
			hash[:], buf)
	}

	if hash.IsEqual(shaHash) {
		t.Errorf("IsEqual: hash contents should not match - got: %v, want: %v",
			hash, shaHash)
	}

	// Set hash from byte slice and ensure contents match.
	err = hash.SetBytes(shaHash.Bytes())
	if err != nil {
		t.Errorf("SetBytes: %v", err)
	}
	if !hash.IsEqual(shaHash) {
		t.Errorf("IsEqual: hash contents mismatch - got: %v, want: %v",
			hash, shaHash)
	}

	// Invalid size for SetBytes.
	err = hash.SetBytes([]byte{0x00})
	if err == nil {
		t.Errorf("SetBytes: failed to received expected err - got: nil")
	}

	// Invalid size for NewShaHash.
	invalidHash := make([]byte, wire.HashSize+1)
	_, err = wire.NewShaHash(invalidHash)
	if err == nil {
		t.Errorf("NewShaHash: failed to received expected err - got: nil")
	}
}

// TestShaHashString  tests the stringized output for sha hashes.
func TestShaHashString(t *testing.T) {
	wantStr := "06e533fd1ada86391f3f6c343204b0d278d4aaec1c0b20aa27ba030000000000"
	hash := wire.ShaHash([wire.HashSize]byte{ // Make go vet happy.
		0x06, 0xe5, 0x33, 0xfd, 0x1a, 0xda, 0x86, 0x39,
		0x1f, 0x3f, 0x6c, 0x34, 0x32, 0x04, 0xb0, 0xd2,
		0x78, 0xd4, 0xaa, 0xec, 0x1c, 0x0b, 0x20, 0xaa,
		0x27, 0xba, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00,
	})

	hashStr := hash.String()
	if hashStr != wantStr {
		t.Errorf("String: wrong hash string - got %v, want %v",
			hashStr, wantStr)
	}
}

// TestNewShaHashFromStr executes tests against the NewShaHashFromStr function.
func TestNewShaHashFromStr(t *testing.T) {
	tests := []struct {
		in   string
		want wire.ShaHash
		err  error
	}{
		// Empty string.
		{
			"",
			wire.ShaHash{},
			wire.ErrHashStrSize,
		},

		// Single digit hash.
		{
			"1",
			wire.ShaHash([wire.HashSize]byte{ // Make go vet happy.
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			}),
			wire.ErrHashStrSize,
		},

		{
			"c478c24a50002d3191e9d87d34ce4f02c55bf83326540cee3d6c33405720f7d2",
			wire.ShaHash([wire.HashSize]byte{ // Make go vet happy.
				0xC4, 0x78, 0xC2, 0x4A, 0x50, 0x00, 0x2D, 0x31,
				0x91, 0xE9, 0xD8, 0x7D, 0x34, 0xCE, 0x4F, 0x02,
				0xC5, 0x5B, 0xF8, 0x33, 0x26, 0x54, 0x0C, 0xEE,
				0x3D, 0x6C, 0x33, 0x40, 0x57, 0x20, 0xF7, 0xD2,
			}),
			nil,
		},

		// Hash string that is too long.
		{
			"01234567890123456789012345678901234567890123456789012345678912345",
			wire.ShaHash{},
			wire.ErrHashStrSize,
		},

		// Hash string that is contains non-hex chars.
		{
			"c47gc24a50002d3191e9d87d34ce4f02c55bf83326540cee3d6c33405720f7d2",
			wire.ShaHash{},
			hex.InvalidByteError('g'),
		},
	}

	unexpectedErrStr := "NewShaHashFromStr #%d failed to detect expected error - got: %v want: %v"
	unexpectedResultStr := "NewShaHashFromStr #%d got: %v want: %v"
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result, err := wire.NewShaHashFromStr(test.in)
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
