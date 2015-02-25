package wire

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// MaxUserAgentLen is the maximum allowed length for the user agent field in a
// version message (MsgVersion).
const MaxUserAgentLen = 2000

// DefaultUserAgent for wire.in the stack
const DefaultUserAgent = "/wire:0.2.0/"

// MsgVersion implements the Message interface and represents a bitmessage version
// message.  It is used for a peer to advertise itself as soon as an outbound
// connection is made.  The remote peer then uses this information along with
// its own to negotiate.  The remote peer must then respond with a version
// message of its own containing the negotiated values followed by a verack
// message (MsgVerAck).  This exchange must take place before any further
// communication is allowed to proceed.
type MsgVersion struct {
	// Version of the protocol the node is using.
	ProtocolVersion int32

	// Bitfield which identifies the enabled services.
	Services ServiceFlag

	// Time the message was generated.  This is encoded as an int64 on the wire.
	Timestamp time.Time

	// Address of the remote peer.
	AddrYou NetAddress

	// Address of the local peer.
	AddrMe NetAddress

	// Unique value associated with message that is used to detect self
	// connections.
	Nonce uint64

	// The user agent that generated messsage.  This is a encoded as a varString
	// on the wire.  This has a max length of MaxUserAgentLen.
	UserAgent string

	// The stream numbers of interest.
	StreamNumbers []uint64
}

// HasService returns whether the specified service is supported by the peer
// that generated the message.
func (msg *MsgVersion) HasService(service ServiceFlag) bool {
	if msg.Services&service == service {
		return true
	}
	return false
}

// AddService adds service as a supported service by the peer generating the
// message.
func (msg *MsgVersion) AddService(service ServiceFlag) {
	msg.Services |= service
}

// Decode decodes r using the bitmessage protocol encoding into the receiver.
//
// This is part of the Message interface implementation.
func (msg *MsgVersion) Decode(r io.Reader) error {
	buf, ok := r.(*bytes.Buffer)
	if !ok {
		return fmt.Errorf("MsgVersion.Decode reader is not a " +
			"*bytes.Buffer")
	}

	var sec int64
	err := readElements(buf, &msg.ProtocolVersion, &msg.Services, &sec)
	if err != nil {
		return err
	}
	msg.Timestamp = time.Unix(sec, 0)

	err = readNetAddress(buf, &msg.AddrYou, false)
	if err != nil {
		return err
	}

	err = readNetAddress(buf, &msg.AddrMe, false)
	if err != nil {
		return err
	}
	err = readElement(buf, &msg.Nonce)
	if err != nil {
		return err
	}
	userAgent, err := readVarString(buf)
	if err != nil {
		return err
	}
	err = validateUserAgent(userAgent)
	if err != nil {
		return err
	}
	msg.UserAgent = userAgent

	streamLen, err := readVarInt(buf)
	if err != nil {
		return err
	}
	msg.StreamNumbers = make([]uint64, int(streamLen))
	for i := uint64(0); i < streamLen; i++ {
		msg.StreamNumbers[i], err = readVarInt(buf)
		if err != nil {
			return err
		}
	}

	return nil
}

// Encode encodes the receiver to w using the bitmessage protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgVersion) Encode(w io.Writer) error {
	err := validateUserAgent(msg.UserAgent)
	if err != nil {
		return err
	}

	err = writeElements(w, msg.ProtocolVersion, msg.Services,
		msg.Timestamp.Unix())
	if err != nil {
		return err
	}

	err = writeNetAddress(w, &msg.AddrYou, false)
	if err != nil {
		return err
	}

	err = writeNetAddress(w, &msg.AddrMe, false)
	if err != nil {
		return err
	}

	err = writeElement(w, msg.Nonce)
	if err != nil {
		return err
	}

	err = writeVarString(w, msg.UserAgent)
	if err != nil {
		return err
	}

	err = writeVarInt(w, uint64(len(msg.StreamNumbers)))
	if err != nil {
		return err
	}
	for _, stream := range msg.StreamNumbers {
		err = writeVarInt(w, stream)
		if err != nil {
			return err
		}
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgVersion) Command() string {
	return CmdVersion
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgVersion) MaxPayloadLength() uint32 {
	// XXX: <= 106 different

	// Protocol version 4 bytes + services 8 bytes + timestamp 8 bytes +
	// remote and local net addresses + nonce 8 bytes + length of user
	// agent (varInt) + max allowed useragent length + last block 4 bytes +
	// relay transactions flag 1 byte.
	return 33 + (maxNetAddressPayload() * 2) + MaxVarIntPayload +
		MaxUserAgentLen
}

// NewMsgVersion returns a new bitmessage version message that conforms to the
// Message interface using the passed parameters and defaults for the remaining
// fields.
func NewMsgVersion(me *NetAddress, you *NetAddress, nonce uint64, streams []uint64) *MsgVersion {

	// Limit the timestamp to one second precision since the protocol
	// doesn't support better.
	return &MsgVersion{
		ProtocolVersion: int32(3),
		Services:        0,
		Timestamp:       time.Unix(time.Now().Unix(), 0),
		AddrYou:         *you,
		AddrMe:          *me,
		Nonce:           nonce,
		UserAgent:       DefaultUserAgent,
		StreamNumbers:   streams[:],
	}
}

// NewMsgVersionFromConn is a convenience function that extracts the remote
// and local address from conn and returns a new bitmessage version message that
// conforms to the Message interface.  See NewMsgVersion.
func NewMsgVersionFromConn(conn net.Conn, nonce uint64, streams []uint64) (*MsgVersion, error) {

	// Don't assume any services until we know otherwise.
	lna, err := NewNetAddress(conn.LocalAddr(), 0)
	if err != nil {
		return nil, err
	}

	// Don't assume any services until we know otherwise.
	rna, err := NewNetAddress(conn.RemoteAddr(), 0)
	if err != nil {
		return nil, err
	}

	return NewMsgVersion(lna, rna, nonce, streams), nil
}

// validateUserAgent checks userAgent length against MaxUserAgentLen
func validateUserAgent(userAgent string) error {
	if len(userAgent) > MaxUserAgentLen {
		str := fmt.Sprintf("user agent too long [len %v, max %v]",
			len(userAgent), MaxUserAgentLen)
		return messageError("MsgVersion", str)
	}
	return nil
}

// AddUserAgent adds a user agent to the user agent string for the version
// message.  The version string is not defined to any strict format, although
// it is recommended to use the form "major.minor.revision" e.g. "2.6.41".
func (msg *MsgVersion) AddUserAgent(name string, version string,
	comments ...string) error {

	newUserAgent := fmt.Sprintf("%s:%s", name, version)
	if len(comments) != 0 {
		newUserAgent = fmt.Sprintf("%s(%s)", newUserAgent,
			strings.Join(comments, "; "))
	}
	newUserAgent = fmt.Sprintf("%s%s/", msg.UserAgent, newUserAgent)
	err := validateUserAgent(newUserAgent)
	if err != nil {
		return err
	}
	msg.UserAgent = newUserAgent
	return nil
}
