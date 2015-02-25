package wire

import (
	"fmt"
	"io"
	"io/ioutil"
	"time"
)

const ()

type MsgMsg struct {
	Nonce            uint64
	ExpiresTime      time.Time
	ObjectType       ObjectType
	Version          uint64
	StreamNumber     uint64
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
	Ack              []byte
	Signature        []byte
}

// Decode decodes r using the bitmessage protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgMsg) Decode(r io.Reader) error {
	var sec int64
	var err error
	if err = readElements(r, &msg.Nonce, &sec, &msg.ObjectType); err != nil {
		return err
	}

	if msg.ObjectType != ObjectTypeMsg {
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

	msg.Encrypted, err = ioutil.ReadAll(r)

	return err
}

// Encode encodes the receiver to w using the bitmessage protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgMsg) Encode(w io.Writer) error {
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

	_, err = w.Write(msg.Encrypted)
	return err
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgMsg) Command() string {
	return CmdObject
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgMsg) MaxPayloadLength() uint32 {
	return 1 << 18
}

func (msg *MsgMsg) String() string {
	return fmt.Sprintf("msg: v%d %d %s %d %x", msg.Version, msg.Nonce, msg.ExpiresTime, msg.StreamNumber, msg.Encrypted)
}

// NewMsgMsg returns a new object message that conforms to the
// Message interface using the passed parameters and defaults for the remaining
// fields.
func NewMsgMsg(nonce uint64, expires time.Time, version, streamNumber uint64, encrypted []byte, addressVersion, fromStreamNumber uint64, behavior uint32, signingKey, encryptKey *PubKey, nonceTrials, extraBytes uint64, destination *RipeHash, encoding uint64, message, ack, signature []byte) *MsgMsg {

	// Limit the timestamp to one second precision since the protocol
	// doesn't support better.
	return &MsgMsg{
		Nonce:            nonce,
		ExpiresTime:      expires,
		ObjectType:       ObjectTypeMsg,
		Version:          version,
		StreamNumber:     streamNumber,
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
		Ack:              ack,
		Signature:        signature,
	}
}
