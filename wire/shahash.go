package wire

import (
	"bytes"
	"encoding/hex"
	"fmt"
)

// HashSize is the size of the array used to store SHA hashes.
const HashSize = 32

// HashStringSize is the maximum length of a ShaHash hash string.
const HashStringSize = HashSize * 2

// ErrHashStrSize describes an error that indicates the caller specified a hash
// string that does not have the right number of characters.
var ErrHashStrSize = fmt.Errorf("string length must be %v chars", HashStringSize)

// ShaHash is used in several of the bitmessage messages and common structures.
// It typically represents a half of the double SHA512 of data.
type ShaHash [HashSize]byte

// String returns the ShaHash as the hexadecimal string of the byte-reversed
// hash.
func (hash ShaHash) String() string {
	return hex.EncodeToString(hash[:])
}

// Bytes returns the bytes which represent the hash as a byte slice.
func (hash *ShaHash) Bytes() []byte {
	newHash := make([]byte, HashSize)
	copy(newHash, hash[:])

	return newHash
}

// SetBytes sets the bytes which represent the hash.  An error is returned if
// the number of bytes passed in is not HashSize.
func (hash *ShaHash) SetBytes(newHash []byte) error {
	nhlen := len(newHash)
	if nhlen != HashSize {
		return fmt.Errorf("invalid sha length of %v, want %v", nhlen,
			HashSize)
	}
	copy(hash[:], newHash)

	return nil
}

// IsEqual returns true if target is the same as hash.
func (hash *ShaHash) IsEqual(target *ShaHash) bool {
	return bytes.Equal(hash[:], target[:])
}

// NewShaHash returns a new ShaHash from a byte slice.  An error is returned if
// the number of bytes passed in is not HashSize.
func NewShaHash(newHash []byte) (*ShaHash, error) {
	var sh ShaHash
	err := sh.SetBytes(newHash)
	if err != nil {
		return nil, err
	}
	return &sh, err
}

// NewShaHashFromStr creates a ShaHash from a hash string.  The string should be
// the hexadecimal string of a hash.
func NewShaHashFromStr(hash string) (*ShaHash, error) {
	// Return error if hash string is not the right size.
	if len(hash) != HashStringSize {
		return nil, ErrHashStrSize
	}

	// Convert string hash to bytes.
	buf, err := hex.DecodeString(hash)
	if err != nil {
		return nil, err
	}

	return NewShaHash(buf)
}
