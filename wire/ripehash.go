package wire

import (
	"bytes"
	"encoding/hex"
	"fmt"
)

// Size of array used to store ripe hashes.
const RipeHashSize = 20

// MaxHashStringSize is the maximum length of a Ripe hash string.
const MaxRipeHashStringSize = RipeHashSize * 2

// ErrRipeHashStrSize describes an error that indicates the caller specified
// a hash string that has too many characters.
var ErrRipeHashStrSize = fmt.Errorf("max hash string length is %v bytes", MaxRipeHashStringSize)

// RipeHash is used in several of the bitmessage messages and common
// structures.  It typically represents the double sha512 and ripemd160
// of data.
type RipeHash [RipeHashSize]byte

// String returns the RipeHash as the hexadecimal string of the byte-reversed
// hash.
func (hash RipeHash) String() string {
	for i := 0; i < RipeHashSize/2; i++ {
		hash[i], hash[RipeHashSize-1-i] = hash[RipeHashSize-1-i], hash[i]
	}
	return hex.EncodeToString(hash[:])
}

// Bytes returns the bytes which represent the hash as a byte slice.
func (hash *RipeHash) Bytes() []byte {
	newHash := make([]byte, RipeHashSize)
	copy(newHash, hash[:])

	return newHash
}

// SetBytes sets the bytes which represent the hash.  An error is returned if
// the number of bytes passed in is not RipeHashSize.
func (hash *RipeHash) SetBytes(newHash []byte) error {
	nhlen := len(newHash)
	if nhlen != RipeHashSize {
		return fmt.Errorf("invalid sha length of %v, want %v", nhlen,
			RipeHashSize)
	}
	copy(hash[:], newHash[0:RipeHashSize])

	return nil
}

// IsEqual returns true if target is the same as hash.
func (hash *RipeHash) IsEqual(target *RipeHash) bool {
	return bytes.Equal(hash[:], target[:])
}

// NewRipeHash returns a new RipeHash from a byte slice.  An error is returned if
// the number of bytes passed in is not RipeHashSize.
func NewRipeHash(newHash []byte) (*RipeHash, error) {
	var ripe RipeHash
	err := ripe.SetBytes(newHash)
	if err != nil {
		return nil, err
	}
	return &ripe, err
}

// NewRipeHashFromStr creates a RipeHash from a hash string.  The string
// should be the hexadecimal string of a byte hash, but any missing
// characters result in zero padding at the end of the RipeHash.
func NewRipeHashFromStr(hash string) (*RipeHash, error) {
	// Return error if hash string is too long.
	if len(hash) > MaxRipeHashStringSize {
		return nil, ErrRipeHashStrSize
	}

	// Hex decoder expects the hash to be a multiple of two.
	if len(hash)%2 != 0 {
		hash = "0" + hash
	}

	// Convert string hash to bytes.
	buf, err := hex.DecodeString(hash)
	if err != nil {
		return nil, err
	}

	// Un-reverse the decoded bytes, copying into in leading bytes of a
	// RipeHash.  There is no need to explicitly pad the result as any
	// missing (when len(buf) < RipeHashSize) bytes from the decoded hex string
	// will remain zeros at the end of the RipeHash.
	var ret RipeHash
	blen := len(buf)
	mid := blen / 2
	if blen%2 != 0 {
		mid++
	}
	blen--
	for i, b := range buf[:mid] {
		ret[i], ret[blen-i] = buf[blen-i], b
	}
	return &ret, nil
}
