package wire

import (
	"bytes"
	"encoding/hex"
	"fmt"
)

// RipeHashSize is the size of array used to store ripe hashes.
const RipeHashSize = 20

// HashStringSize is the maximum length of a Ripe hash string.
const RipeHashStringSize = RipeHashSize * 2

// ErrRipeHashStrSize describes an error that indicates the caller specified
// a hash string that does not have the right number of characters.
var ErrRipeHashStrSize = fmt.Errorf("string length must be %v chars", RipeHashStringSize)

// RipeHash is used in several of the bitmessage messages and common
// structures. It typically represents the double sha512 of ripemd160
// of data.
type RipeHash [RipeHashSize]byte

// String returns the RipeHash as the hexadecimal string of the byte-reversed
// hash.
func (hash RipeHash) String() string {
	return hex.EncodeToString(hash[:])
}

// Bytes returns the bytes which represent the hash as a byte slice.
func (hash *RipeHash) Bytes() []byte {
	newHash := make([]byte, RipeHashSize)
	copy(newHash, hash[:])

	return newHash
}

// SetBytes sets the bytes which represent the hash. An error is returned if
// the number of bytes passed in is not RipeHashSize.
func (hash *RipeHash) SetBytes(newHash []byte) error {
	nhlen := len(newHash)
	if nhlen != RipeHashSize {
		return fmt.Errorf("invalid ripe length of %v, want %v", nhlen,
			RipeHashSize)
	}
	copy(hash[:], newHash)

	return nil
}

// IsEqual returns true if target is the same as hash.
func (hash *RipeHash) IsEqual(target *RipeHash) bool {
	return bytes.Equal(hash[:], target[:])
}

// NewRipeHash returns a new RipeHash from a byte slice. An error is returned if
// the number of bytes passed in is not RipeHashSize.
func NewRipeHash(newHash []byte) (*RipeHash, error) {
	var ripe RipeHash
	err := ripe.SetBytes(newHash)
	if err != nil {
		return nil, err
	}
	return &ripe, err
}

// NewRipeHashFromStr creates a RipeHash from a hash string. The string should
// be the hexadecimal string of a byte hash.
func NewRipeHashFromStr(hash string) (*RipeHash, error) {
	// Return error if hash string is not the right size.
	if len(hash) != RipeHashStringSize {
		return nil, ErrRipeHashStrSize
	}

	// Convert string hash to bytes.
	buf, err := hex.DecodeString(hash)
	if err != nil {
		return nil, err
	}
	return NewRipeHash(buf)
}
