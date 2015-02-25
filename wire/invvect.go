package wire

import (
	"io"
)

const (
	// MaxInvPerMsg is the maximum number of inventory vectors that can be in a
	// single bitmessage inv message.
	MaxInvPerMsg = 50000

	// Maximum payload size for an inventory vector.
	maxInvVectPayload = HashSize
)

// InvVect defines a bitmessage inventory vector which is used to describe data,
// as specified by the Type field, that a peer wants, has, or does not have to
// another peer.
type InvVect struct {
	Hash ShaHash // Hash of the data
}

// NewInvVect returns a new InvVect using the provided type and hash.
func NewInvVect(hash *ShaHash) *InvVect {
	return &InvVect{
		Hash: *hash,
	}
}

// readInvVect reads an encoded InvVect from r depending on the protocol
// version.
func readInvVect(r io.Reader, iv *InvVect) error {
	err := readElements(r, &iv.Hash)
	if err != nil {
		return err
	}
	return nil
}

// writeInvVect serializes an InvVect to w depending on the protocol version.
func writeInvVect(w io.Writer, iv *InvVect) error {
	err := writeElements(w, iv.Hash)
	if err != nil {
		return err
	}
	return nil
}
