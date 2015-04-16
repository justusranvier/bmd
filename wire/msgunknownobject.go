package wire

import (
	"fmt"
	"io"
	"io/ioutil"
	"time"
)

// MsgUnknownObject implements the Message interface and represents an unknown
// object.
type MsgUnknownObject struct {
	Nonce        uint64
	ExpiresTime  time.Time
	ObjectType   ObjectType
	Version      uint64
	StreamNumber uint64
	Payload      []byte
}

// Decode decodes r using the bitmessage protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgUnknownObject) Decode(r io.Reader) error {
	var sec int64
	var err error
	if err = readElements(r, &msg.Nonce, &sec, &msg.ObjectType); err != nil {
		return err
	}

	if msg.ObjectType < ObjectType(4) {
		str := fmt.Sprintf("Object Type should be > 3, but is %d", msg.ObjectType)
		return messageError("Decode", str)
	}

	msg.ExpiresTime = time.Unix(sec, 0)
	if msg.Version, err = readVarInt(r); err != nil {
		return err
	}

	if msg.StreamNumber, err = readVarInt(r); err != nil {
		return err
	}

	msg.Payload, err = ioutil.ReadAll(r)

	return err
}

// Encode encodes the receiver to w using the bitmessage protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgUnknownObject) Encode(w io.Writer) error {
	var err error
	if err = writeElements(w, msg.Nonce, msg.ExpiresTime.Unix(), msg.ObjectType); err != nil {
		return err
	}

	if err = writeVarInt(w, msg.Version); err != nil {
		return err
	}

	if err = writeVarInt(w, msg.StreamNumber); err != nil {
		return err
	}

	_, err = w.Write(msg.Payload)
	return err
}

// Command returns the protocol command string for the message. This is part
// of the Message interface implementation.
func (msg *MsgUnknownObject) Command() string {
	return CmdObject
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver. This is part of the Message interface implementation.
func (msg *MsgUnknownObject) MaxPayloadLength() int {
	return MaxPayloadOfMsgObject
}

func (msg *MsgUnknownObject) String() string {
	return fmt.Sprintf("unknown object: v%d %d %s %d %x", msg.Version, msg.Nonce, msg.ExpiresTime, msg.StreamNumber, msg.Payload)
}

// NewMsgUnknownObject returns a new object message that conforms to the
// Message interface using the passed parameters and defaults for the remaining
// fields.
func NewMsgUnknownObject(nonce uint64, expires time.Time, objectType ObjectType, version, streamNumber uint64, payload []byte) *MsgUnknownObject {
	return &MsgUnknownObject{
		Nonce:        nonce,
		ExpiresTime:  expires,
		ObjectType:   objectType,
		Version:      version,
		StreamNumber: streamNumber,
		Payload:      payload,
	}
}
