package wire

import (
	"fmt"
	"io"
	"time"
)

const (
	// Starting in version 4, Ripe is derived from the tag and not
	// sent directly
	TagBasedRipeVersion = 4
)

type MsgGetPubKey struct {
	Nonce        uint64
	ExpiresTime  time.Time
	ObjectType   ObjectType
	Version      uint64
	StreamNumber uint64
	Ripe         *RipeHash
	Tag          *ShaHash
}

// Decode decodes r using the bitmessage protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgGetPubKey) Decode(r io.Reader) error {
	var sec int64
	err := readElements(r, &msg.Nonce, &sec, &msg.ObjectType)
	if err != nil {
		return err
	}

	if msg.ObjectType != ObjectTypeGetPubKey {
		str := fmt.Sprintf("Object Type should be %d, but is %d",
			ObjectTypeGetPubKey, msg.ObjectType)
		return messageError("Decode", str)
	}

	msg.ExpiresTime = time.Unix(sec, 0)
	if msg.Version, err = readVarInt(r); err != nil {
		return err
	}

	if msg.StreamNumber, err = readVarInt(r); err != nil {
		return err
	}

	if msg.Version >= TagBasedRipeVersion {
		msg.Tag, _ = NewShaHash(make([]byte, 32))
		if err = readElement(r, msg.Tag); err != nil {
			return err
		}
	} else {
		msg.Ripe, _ = NewRipeHash(make([]byte, 20))
		if err = readElement(r, msg.Ripe); err != nil {
			return err
		}
	}

	return err
}

// Encode encodes the receiver to w using the bitmessage protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetPubKey) Encode(w io.Writer) error {
	err := writeElements(w, msg.Nonce, msg.ExpiresTime.Unix(), msg.ObjectType)
	if err != nil {
		return err
	}

	if err = writeVarInt(w, msg.Version); err != nil {
		return err
	}

	if err = writeVarInt(w, msg.StreamNumber); err != nil {
		return err
	}

	if msg.Version >= TagBasedRipeVersion {
		if err = writeElement(w, msg.Tag); err != nil {
			return err
		}
	} else {
		if err = writeElement(w, msg.Ripe); err != nil {
			return err
		}
	}

	return err
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgGetPubKey) Command() string {
	return CmdObject
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgGetPubKey) MaxPayloadLength() uint32 {
	return 1 << 18
}

func (msg *MsgGetPubKey) String() string {
	return fmt.Sprintf("getpubkey: v%d %d %s %d %x %x", msg.Version, msg.Nonce, msg.ExpiresTime, msg.StreamNumber, msg.Ripe, msg.Tag)
}

// NewMsgGetPubKey returns a new object message that conforms to the
// Message interface using the passed parameters and defaults for the remaining
// fields.
func NewMsgGetPubKey(nonce uint64, expires time.Time, version, streamNumber uint64, ripe *RipeHash, tag *ShaHash) *MsgGetPubKey {

	// Limit the timestamp to one second precision since the protocol
	// doesn't support better.
	return &MsgGetPubKey{
		Nonce:        nonce,
		ExpiresTime:  expires,
		ObjectType:   ObjectTypeGetPubKey,
		Version:      version,
		StreamNumber: streamNumber,
		Ripe:         ripe,
		Tag:          tag,
	}
}
