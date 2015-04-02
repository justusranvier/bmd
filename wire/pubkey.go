package wire

import (
	"bytes"
	"encoding/hex"
	"fmt"
)

// PubKeySize is the size of array used to store uncompressed public keys. Note
// that the first byte (0x04) is excluded when storing them.
const PubKeySize = 64

// MaxPubKeyStringSize is the maximum length of a PubKey string.
const MaxPubKeyStringSize = PubKeySize * 2

// ErrPubKeyStrSize describes an error that indicates the caller specified
// a PubKey string that does not have the right number of characters.
var ErrPubKeyStrSize = fmt.Errorf("string length must be %v chars", MaxPubKeyStringSize)

// PubKey is used in several of the bitmessage messages and common structures.
// The first 32 bytes contain the X value and the other 32 contain the Y value.
type PubKey [PubKeySize]byte

// String returns the PubKey as a hexadecimal string.
func (hash PubKey) String() string {
	return hex.EncodeToString(hash[:])
}

// Bytes returns the bytes which represent the hash as a byte slice.
func (hash *PubKey) Bytes() []byte {
	newHash := make([]byte, PubKeySize)
	copy(newHash, hash[:])

	return newHash
}

// SetBytes sets the bytes which represent the hash. An error is returned if
// the number of bytes passed in is not PubKeySize.
func (hash *PubKey) SetBytes(newHash []byte) error {
	nhlen := len(newHash)
	if nhlen != PubKeySize {
		return fmt.Errorf("invalid pub key length of %v, want %v", nhlen,
			PubKeySize)
	}
	copy(hash[:], newHash[0:PubKeySize])

	return nil
}

// IsEqual returns true if target is the same as hash.
func (hash *PubKey) IsEqual(target *PubKey) bool {
	return bytes.Equal(hash[:], target[:])
}

// NewPubKey returns a new PubKey from a byte slice. An error is returned if
// the number of bytes passed in is not PubKeySize.
func NewPubKey(newHash []byte) (*PubKey, error) {
	var pubkey PubKey
	err := pubkey.SetBytes(newHash)
	if err != nil {
		return nil, err
	}
	return &pubkey, err
}

// NewPubKeyFromStr creates a PubKey from a hash string. The string should be
// the hexadecimal string of the PubKey.
func NewPubKeyFromStr(pubkey string) (*PubKey, error) {
	// Return error if PubKey string is not the right size.
	if len(pubkey) != MaxPubKeyStringSize {
		return nil, ErrPubKeyStrSize
	}

	// Convert string hash to bytes.
	buf, err := hex.DecodeString(pubkey)
	if err != nil {
		return nil, err
	}

	return NewPubKey(buf)
}
