package wire

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// ProtocolVersion is the latest protocol version this package supports.
	ProtocolVersion uint32 = 3
)

// ServiceFlag identifies services supported by a bitmessage peer.
type ServiceFlag uint64

const (
	// SFNodeNetwork is a flag used to indicate a peer is a full node.
	SFNodeNetwork ServiceFlag = 1 << iota
)

// Map of service flags back to their constant names for pretty printing.
var sfStrings = map[ServiceFlag]string{
	SFNodeNetwork: "SFNodeNetwork",
}

// String returns the ServiceFlag in human-readable form.
func (f ServiceFlag) String() string {
	// No flags are set.
	if f == 0 {
		return "0x0"
	}

	// Add individual bit flags.
	s := ""
	for flag, name := range sfStrings {
		if f&flag == flag {
			s += name + "|"
			f -= flag
		}
	}

	// Add any remaining flags which aren't accounted for as hex.
	s = strings.TrimRight(s, "|")
	if f != 0 {
		s += "|0x" + strconv.FormatUint(uint64(f), 16)
	}
	s = strings.TrimLeft(s, "|")
	return s
}

// BitmessageNet represents which bitmessage network a message belongs to.
type BitmessageNet uint32

// Constants used to indicate the message bitmessage network.  They can also be
// used to seek to the next message when a stream's state is unknown, but
// this package does not provide that functionality since it's generally a
// better idea to simply disconnect clients that are misbehaving over TCP.
const (
	// MainNet represents the main bitmessage network.
	MainNet BitmessageNet = 0xe9beb4d9
)

// bnStrings is a map of bitmessage networks back to their constant names for
// pretty printing.
var bnStrings = map[BitmessageNet]string{
	MainNet: "MainNet",
}

// String returns the BitmessageNet in human-readable form.
func (n BitmessageNet) String() string {
	if s, ok := bnStrings[n]; ok {
		return s
	}

	return fmt.Sprintf("Unknown BitmessageNet (%d)", uint32(n))
}
