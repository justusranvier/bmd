package wire

import (
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// MaxUserAgentLen is the maximum allowed length for the user agent field in a
// version message (MsgVersion).
const MaxUserAgentLen = 5000

// MaxStreams is the maximum number of allowed streams to request according
// to the bitmessage protocol. Keeping it at 1 for now.
const MaxStreams = 1

// DefaultUserAgent for wire
const DefaultUserAgent = "/wire:0.1.0/"

// MsgVersion implements the Message interface and represents a bitmessage version
// message. It is used for a peer to advertise itself as soon as an outbound
// connection is made. The remote peer then uses this information along with
// its own to negotiate. The remote peer must then respond with a version
// message of its own containing the negotiated values followed by a verack
// message (MsgVerAck). This exchange must take place before any further
// communication is allowed to proceed.
type MsgVersion struct {
	// Version of the protocol the node is using.
	ProtocolVersion int32

	// Bitfield which identifies the enabled services.
	Services ServiceFlag

	// Time the message was generated. This is encoded as an int64 on the wire.
	Timestamp time.Time

	// Address of the remote peer.
	AddrYou *NetAddress

	// Address of the local peer.
	AddrMe *NetAddress

	// Unique value associated with message that is used to detect self
	// connections.
	Nonce uint64

	// The user agent that generated messsage. This is a encoded as a varString
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
// This is part of the Message interface implementation.
func (msg *MsgVersion) Decode(r io.Reader) error {
	var sec int64
	err := readElements(r, &msg.ProtocolVersion, &msg.Services, &sec)
	if err != nil {
		return err
	}
	msg.Timestamp = time.Unix(sec, 0)

	msg.AddrYou = new(NetAddress)
	err = readNetAddress(r, msg.AddrYou, false)
	if err != nil {
		return err
	}

	msg.AddrMe = new(NetAddress)
	err = readNetAddress(r, msg.AddrMe, false)
	if err != nil {
		return err
	}
	err = readElement(r, &msg.Nonce)
	if err != nil {
		return err
	}
	userAgent, err := readVarString(r)
	if err != nil {
		return err
	}
	err = validateUserAgent(userAgent)
	if err != nil {
		return err
	}
	msg.UserAgent = userAgent

	streamLen, err := readVarInt(r)
	if err != nil {
		return err
	}

	if streamLen > MaxStreams {
		return fmt.Errorf("number of streams is too large: %v", streamLen)
	}

	msg.StreamNumbers = make([]uint64, int(streamLen))
	for i := uint64(0); i < streamLen; i++ {
		msg.StreamNumbers[i], err = readVarInt(r)
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

	err = writeNetAddress(w, msg.AddrYou, false)
	if err != nil {
		return err
	}

	err = writeNetAddress(w, msg.AddrMe, false)
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

	if len(msg.StreamNumbers) > MaxStreams {
		return fmt.Errorf("number of streams is too large: %v", len(msg.StreamNumbers))
	}

	for _, stream := range msg.StreamNumbers {
		err = writeVarInt(w, stream)
		if err != nil {
			return err
		}
	}

	return nil
}

// Command returns the protocol command string for the message. This is part
// of the Message interface implementation.
func (msg *MsgVersion) Command() string {
	return CmdVersion
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver. This is part of the Message interface implementation.
func (msg *MsgVersion) MaxPayloadLength() int {
	// Protocol version 4 bytes + services 8 bytes + timestamp 8 bytes +
	// remote and local net addresses (26*2) + nonce 8 bytes + length of user
	// agent (varInt) + max allowed useragent length + number of streams
	// (varInt) + list of streams
	return 4 + 8 + 8 + 26*2 + 8 + VarIntSerializeSize(MaxUserAgentLen) +
		MaxUserAgentLen + MaxStreams + VarIntSerializeSize(MaxStreams)*MaxStreams
	// The last calculation is slightly invalid because initial numbers would
	// take up less space than larger ones. However, this serves as a quick and
	// easy to calculate upperbound.
}

// NewMsgVersion returns a new bitmessage version message that conforms to the
// Message interface using the passed parameters and defaults for the remaining
// fields.
func NewMsgVersion(me *NetAddress, you *NetAddress, nonce uint64, streams []uint64) *MsgVersion {
	return &MsgVersion{
		ProtocolVersion: int32(3),
		Services:        0,
		Timestamp:       time.Unix(time.Now().Unix(), 0),
		AddrYou:         you,
		AddrMe:          me,
		Nonce:           nonce,
		UserAgent:       DefaultUserAgent,
		StreamNumbers:   streams[:],
	}
}

// NewMsgVersionFromConn is a convenience function that extracts the remote
// and local address from conn and returns a new bitmessage version message that
// conforms to the Message interface.  See NewMsgVersion.
func NewMsgVersionFromConn(conn net.Conn, nonce uint64, currentStream uint32, allStreams []uint64) (*MsgVersion, error) {

	// Don't assume any services until we know otherwise.
	lna, err := NewNetAddress(conn.LocalAddr(), currentStream, 0)
	if err != nil {
		return nil, err
	}

	// Don't assume any services until we know otherwise.
	rna, err := NewNetAddress(conn.RemoteAddr(), currentStream, 0)
	if err != nil {
		return nil, err
	}

	return NewMsgVersion(lna, rna, nonce, allStreams), nil
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
