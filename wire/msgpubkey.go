package wire

import (
	"fmt"
	"io"
	"io/ioutil"
	"time"
)

const (
	// SimplePubKeyVersion is the version in which pubkeys are sent unencrypted
	// without any details of PoW required by the sender.
	SimplePubKeyVersion = 2
	// ExtendedPubKeyVersion is the version in which pubkeys are sent
	// unencrypted with details of PoW required by the sender.
	ExtendedPubKeyVersion = 3
	// EncryptedPubKeyVersion is the version from which pubkeys started to be
	// sent as an encrypted ExtendedPubKey, decryptable by those who had the
	// addresses of the owners of those keys.
	EncryptedPubKeyVersion = 4
	// Signature consists of 2 256-bit integers encoding using ASN.1
	// 2*256/8 + 16 (safe encoding boundary). TODO find precise number. Probably
	// 72.
	signatureMaxLength = 80
)

// MsgPubKey implements the Message interface and represents a pubkey sent in
// response to MsgGetPubKey.
type MsgPubKey struct {
	Nonce         uint64
	ExpiresTime   time.Time
	ObjectType    ObjectType
	Version       uint64
	StreamNumber  uint64
	Behavior      uint32
	SigningKey    *PubKey
	EncryptionKey *PubKey
	NonceTrials   uint64
	ExtraBytes    uint64
	Signature     []byte
	Tag           *ShaHash
	Encrypted     []byte
}

// Decode decodes r using the bitmessage protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgPubKey) Decode(r io.Reader) error {
	var sec int64
	var err error
	if err = readElements(r, &msg.Nonce, &sec, &msg.ObjectType); err != nil {
		return err
	}

	if msg.ObjectType != ObjectTypePubKey {
		str := fmt.Sprintf("Object Type should be %d, but is %d",
			ObjectTypePubKey, msg.ObjectType)
		return messageError("Decode", str)
	}

	msg.ExpiresTime = time.Unix(sec, 0)
	if msg.Version, err = readVarInt(r); err != nil {
		return err
	}

	if msg.StreamNumber, err = readVarInt(r); err != nil {
		return err
	}

	if msg.Version >= EncryptedPubKeyVersion {
		msg.Tag, _ = NewShaHash(make([]byte, HashSize))
		if err = readElement(r, msg.Tag); err != nil {
			return err
		}
		// The rest is the encrypted data, accessible only to the holder
		// of the private key to whom it's addressed.
		msg.Encrypted, err = ioutil.ReadAll(r)
		return err
	} else if msg.Version == ExtendedPubKeyVersion {
		msg.SigningKey, _ = NewPubKey(make([]byte, 64))
		msg.EncryptionKey, _ = NewPubKey(make([]byte, 64))
		var sigLength uint64
		err = readElements(r, &msg.Behavior, msg.SigningKey, msg.EncryptionKey)
		if err != nil {
			return err
		}
		if msg.NonceTrials, err = readVarInt(r); err != nil {
			return err
		}
		if msg.ExtraBytes, err = readVarInt(r); err != nil {
			return err
		}
		if sigLength, err = readVarInt(r); err != nil {
			return err
		}
		if sigLength > signatureMaxLength {
			str := fmt.Sprintf("signature length exceeds max length - "+
				"indicates %d, but max length is %d",
				sigLength, signatureMaxLength)
			return messageError("Decode", str)
		}
		msg.Signature = make([]byte, sigLength)
		_, err = io.ReadFull(r, msg.Signature)
		return err
	}

	msg.SigningKey, _ = NewPubKey(make([]byte, 64))
	msg.EncryptionKey, _ = NewPubKey(make([]byte, 64))
	return readElements(r, &msg.Behavior, msg.SigningKey, msg.EncryptionKey)
}

// Encode encodes the receiver to w using the bitmessage protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgPubKey) Encode(w io.Writer) error {
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

	if msg.Version >= EncryptedPubKeyVersion {
		if err = writeElement(w, msg.Tag); err != nil {
			return err
		}
		// The rest is the encrypted data, accessible only to the holder
		// of the private key to whom it's addressed.
		_, err = w.Write(msg.Encrypted)
		return err

	} else if msg.Version == ExtendedPubKeyVersion {
		err = writeElements(w, msg.Behavior, msg.SigningKey, msg.EncryptionKey)
		if err != nil {
			return err
		}
		if err = writeVarInt(w, msg.NonceTrials); err != nil {
			return err
		}
		if err = writeVarInt(w, msg.ExtraBytes); err != nil {
			return err
		}
		sigLength := uint64(len(msg.Signature))
		if err = writeVarInt(w, sigLength); err != nil {
			return err
		}
		_, err = w.Write(msg.Signature)
		return err
	}

	return writeElements(w, &msg.Behavior, msg.SigningKey, msg.EncryptionKey)
}

// Command returns the protocol command string for the message. This is part
// of the Message interface implementation.
func (msg *MsgPubKey) Command() string {
	return CmdObject
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver. This is part of the Message interface implementation.
func (msg *MsgPubKey) MaxPayloadLength() int {
	// TODO find a sensible value based on pubkey version
	return MaxPayloadOfMsgObject
}

func (msg *MsgPubKey) String() string {
	return fmt.Sprintf("pubkey: v%d %d %s %d %x", msg.Version, msg.Nonce, msg.ExpiresTime, msg.StreamNumber, msg.Tag)
}

// NewMsgPubKey returns a new object message that conforms to the Message
// interface using the passed parameters and defaults for the remaining fields.
func NewMsgPubKey(nonce uint64, expires time.Time,
	version, streamNumber uint64, behavior uint32,
	signingKey, encryptKey *PubKey, nonceTrials, extraBytes uint64,
	signature []byte, tag *ShaHash, encrypted []byte) *MsgPubKey {
	return &MsgPubKey{
		Nonce:         nonce,
		ExpiresTime:   expires,
		ObjectType:    ObjectTypePubKey,
		Version:       version,
		StreamNumber:  streamNumber,
		Behavior:      behavior,
		SigningKey:    signingKey,
		EncryptionKey: encryptKey,
		NonceTrials:   nonceTrials,
		ExtraBytes:    extraBytes,
		Signature:     signature,
		Tag:           tag,
		Encrypted:     encrypted,
	}
}
