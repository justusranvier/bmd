package wire

import (
	"fmt"
	"io"
	"io/ioutil"
	"time"
)

const (
	// TagBroadcastVersion is the broadcast version from which tags for light
	// clients started being added at the beginning of the broadcast message.
	TagBroadcastVersion = 5
)

// MsgBroadcast implements the Message interface and represents a broadcast
// message that can be decrypted by all the clients that know the address of the
// sender.
type MsgBroadcast struct {
	Nonce            uint64
	ExpiresTime      time.Time
	ObjectType       ObjectType
	Version          uint64
	StreamNumber     uint64
	Tag              *ShaHash
	Encrypted        []byte
	AddressVersion   uint64
	FromStreamNumber uint64
	Behavior         uint32
	SigningKey       *PubKey
	EncryptKey       *PubKey
	NonceTrials      uint64
	ExtraBytes       uint64
	Destination      *RipeHash
	Encoding         uint64
	Message          []byte
	Signature        []byte
}

// Decode decodes r using the bitmessage protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgBroadcast) Decode(r io.Reader) error {
	var sec int64
	var err error
	if err := readElements(r, &msg.Nonce, &sec, &msg.ObjectType); err != nil {
		return err
	}

	if msg.ObjectType != ObjectTypeBroadcast {
		str := fmt.Sprintf("Object Type should be %d, but is %d",
			ObjectTypeBroadcast, msg.ObjectType)
		return messageError("Decode", str)
	}

	msg.ExpiresTime = time.Unix(sec, 0)
	if msg.Version, err = readVarInt(r); err != nil {
		return err
	}

	if msg.StreamNumber, err = readVarInt(r); err != nil {
		return err
	}

	if msg.Version >= TagBroadcastVersion {
		msg.Tag, _ = NewShaHash(make([]byte, HashSize))
		if err = readElements(r, msg.Tag); err != nil {
			return err
		}
	}

	msg.Encrypted, err = ioutil.ReadAll(r)

	return err
}

// Encode encodes the receiver to w using the bitmessage protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgBroadcast) Encode(w io.Writer) error {
	var err error
	err = writeElements(w, msg.Nonce, msg.ExpiresTime.Unix(), msg.ObjectType)
	if err != nil {
		return err
	}

	if err = writeVarInt(w, msg.Version); err != nil {
		return err
	}

	if err = writeVarInt(w, msg.StreamNumber); err != nil {
		return err
	}

	if msg.Version >= TagBroadcastVersion {
		if err = writeElement(w, msg.Tag); err != nil {
			return err
		}
	}

	_, err = w.Write(msg.Encrypted)
	return err
}

// Command returns the protocol command string for the message. This is part
// of the Message interface implementation.
func (msg *MsgBroadcast) Command() string {
	return CmdObject
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver. This is part of the Message interface implementation.
func (msg *MsgBroadcast) MaxPayloadLength() int {
	return MaxPayloadOfMsgObject
}

func (msg *MsgBroadcast) String() string {
	return fmt.Sprintf("broadcast: v%d %d %s %d %x %x", msg.Version, msg.Nonce, msg.ExpiresTime, msg.StreamNumber, msg.Tag, msg.Encrypted)
}

// NewMsgBroadcast returns a new object message that conforms to the
// Message interface using the passed parameters and defaults for the remaining
// fields.
func NewMsgBroadcast(nonce uint64, expires time.Time, version, streamNumber uint64, tag *ShaHash, encrypted []byte, addressVersion, fromStreamNumber uint64, behavior uint32, signingKey, encryptKey *PubKey, nonceTrials, extraBytes uint64, destination *RipeHash, encoding uint64, message, signature []byte) *MsgBroadcast {
	return &MsgBroadcast{
		Nonce:            nonce,
		ExpiresTime:      expires,
		ObjectType:       ObjectTypeBroadcast,
		Version:          version,
		StreamNumber:     streamNumber,
		Tag:              tag,
		Encrypted:        encrypted,
		AddressVersion:   addressVersion,
		FromStreamNumber: fromStreamNumber,
		Behavior:         behavior,
		SigningKey:       signingKey,
		EncryptKey:       encryptKey,
		NonceTrials:      nonceTrials,
		ExtraBytes:       extraBytes,
		Destination:      destination,
		Encoding:         encoding,
		Message:          message,
		Signature:        signature,
	}
}
