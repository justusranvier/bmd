package wire

import (
	"bytes"
	"encoding/hex"
	"fmt"
)

// Size of array used to store Public Keys
const PubKeySize = 64

// MaxHashStringSize is the maximum length of a Ripe hash string.
const MaxPubKeyStringSize = PubKeySize * 2

// ErrPubKeyStrSize describes an error that indicates the caller specified
// a hash string that has too many characters.
var ErrPubKeyStrSize = fmt.Errorf("max hash string length is %v bytes", MaxPubKeyStringSize)

// PubKey is used in several of the bitmessage messages and common
// structures.  It typically represents the double sha512 and ripemd160
// of data.
type PubKey [PubKeySize]byte

// String returns the PubKey as the hexadecimal string of the byte-reversed
// hash.
func (hash PubKey) String() string {
	for i := 0; i < PubKeySize/2; i++ {
		hash[i], hash[PubKeySize-1-i] = hash[PubKeySize-1-i], hash[i]
	}
	return hex.EncodeToString(hash[:])
}

// Bytes returns the bytes which represent the hash as a byte slice.
func (hash *PubKey) Bytes() []byte {
	newHash := make([]byte, PubKeySize)
	copy(newHash, hash[:])

	return newHash
}

// SetBytes sets the bytes which represent the hash.  An error is returned if
// the number of bytes passed in is not PubKeySize.
func (hash *PubKey) SetBytes(newHash []byte) error {
	nhlen := len(newHash)
	if nhlen != PubKeySize {
		return fmt.Errorf("invalid sha length of %v, want %v", nhlen,
			PubKeySize)
	}
	copy(hash[:], newHash[0:PubKeySize])

	return nil
}

// IsEqual returns true if target is the same as hash.
func (hash *PubKey) IsEqual(target *PubKey) bool {
	return bytes.Equal(hash[:], target[:])
}

// NewPubKey returns a new PubKey from a byte slice.  An error is returned if
// the number of bytes passed in is not PubKeySize.
func NewPubKey(newHash []byte) (*PubKey, error) {
	var ripe PubKey
	err := ripe.SetBytes(newHash)
	if err != nil {
		return nil, err
	}
	return &ripe, err
}

// NewPubKeyFromStr creates a PubKey from a hash string.  The string
// should be the hexadecimal string of a byte hash, but any missing
// characters result in zero padding at the end of the PubKey.
func NewPubKeyFromStr(hash string) (*PubKey, error) {
	// Return error if hash string is too long.
	if len(hash) > MaxPubKeyStringSize {
		return nil, ErrPubKeyStrSize
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
	// PubKey.  There is no need to explicitly pad the result as any
	// missing (when len(buf) < PubKeySize) bytes from the decoded hex string
	// will remain zeros at the end of the PubKey.
	var ret PubKey
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
