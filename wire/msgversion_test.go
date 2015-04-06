package wire_test

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/monetas/bmd/wire"
)

// TestVersion tests the MsgVersion API.
func TestVersion(t *testing.T) {
	pver := wire.ProtocolVersion

	// Create version message data.
	tcpAddrMe := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8333}
	me, err := wire.NewNetAddress(tcpAddrMe, 1, wire.SFNodeNetwork)
	if err != nil {
		t.Errorf("NewNetAddress: %v", err)
	}
	tcpAddrYou := &net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 8333}
	you, err := wire.NewNetAddress(tcpAddrYou, 1, wire.SFNodeNetwork)
	if err != nil {
		t.Errorf("NewNetAddress: %v", err)
	}
	nonce, err := wire.RandomUint64()
	if err != nil {
		t.Errorf("RandomUint64: error generating nonce: %v", err)
	}

	// Ensure we get the correct data back out.
	msg := wire.NewMsgVersion(me, you, nonce, []uint32{1})
	if msg.ProtocolVersion != int32(pver) {
		t.Errorf("NewMsgVersion: wrong protocol version - got %v, want %v",
			msg.ProtocolVersion, pver)
	}
	if !reflect.DeepEqual(msg.AddrMe, me) {
		t.Errorf("NewMsgVersion: wrong me address - got %v, want %v",
			spew.Sdump(&msg.AddrMe), spew.Sdump(me))
	}
	if !reflect.DeepEqual(msg.AddrYou, you) {
		t.Errorf("NewMsgVersion: wrong you address - got %v, want %v",
			spew.Sdump(&msg.AddrYou), spew.Sdump(you))
	}
	if msg.Nonce != nonce {
		t.Errorf("NewMsgVersion: wrong nonce - got %v, want %v",
			msg.Nonce, nonce)
	}
	if msg.UserAgent != wire.DefaultUserAgent {
		t.Errorf("NewMsgVersion: wrong user agent - got %v, want %v",
			msg.UserAgent, wire.DefaultUserAgent)
	}
	if len(msg.StreamNumbers) != 1 {
		t.Errorf("NewMsgVersion: wrong number of streams - got %v, want %v",
			len(msg.StreamNumbers), 1)
	}

	if msg.StreamNumbers[0] != 1 {
		t.Errorf("NewMsgVersion: wrong streams - got %v, want %v",
			msg.StreamNumbers[0], 1)
	}

	msg.AddUserAgent("myclient", "1.2.3", "optional", "comments")
	customUserAgent := wire.DefaultUserAgent + "myclient:1.2.3(optional; comments)/"
	if msg.UserAgent != customUserAgent {
		t.Errorf("AddUserAgent: wrong user agent - got %s, want %s",
			msg.UserAgent, customUserAgent)
	}

	msg.AddUserAgent("mygui", "3.4.5")
	customUserAgent += "mygui:3.4.5/"
	if msg.UserAgent != customUserAgent {
		t.Errorf("AddUserAgent: wrong user agent - got %s, want %s",
			msg.UserAgent, customUserAgent)
	}

	// accounting for ":", "/"
	err = msg.AddUserAgent(strings.Repeat("t",
		wire.MaxUserAgentLen-len(customUserAgent)-2+1), "")
	if _, ok := err.(*wire.MessageError); !ok {
		t.Errorf("AddUserAgent: expected error not received "+
			"- got %v, want %T", err, wire.MessageError{})

	}

	// Version message should not have any services set by default.
	if msg.Services != 0 {
		t.Errorf("NewMsgVersion: wrong default services - got %v, want %v",
			msg.Services, 0)

	}
	if msg.HasService(wire.SFNodeNetwork) {
		t.Errorf("HasService: SFNodeNetwork service is not set")
	}
	msg.AddService(wire.SFNodeNetwork)
	if !msg.HasService(wire.SFNodeNetwork) {
		t.Errorf("HasService: SFNodeNetwork service is not set")
	}

	// Ensure the command is expected value.
	wantCmd := "version"
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgVersion: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value.
	// Protocol version 4 bytes + services 8 bytes + timestamp 8 bytes +
	// remote and local net addresses + nonce 8 bytes + length of user agent
	// (varInt) + max allowed user agent length + 1 stream number
	wantPayload := 5085
	maxPayload := msg.MaxPayloadLength()
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length "+
			"got %v, want %v", maxPayload, wantPayload)
	}

	// Ensure adding the full service node flag works.
	msg.AddService(wire.SFNodeNetwork)
	if msg.Services != wire.SFNodeNetwork {
		t.Errorf("AddService: wrong services - got %v, want %v",
			msg.Services, wire.SFNodeNetwork)
	}
	if !msg.HasService(wire.SFNodeNetwork) {
		t.Errorf("HasService: SFNodeNetwork service not set")
	}

	// Use a fake connection.
	conn := &fakeConn{localAddr: tcpAddrMe, remoteAddr: tcpAddrYou}
	msg, err = wire.NewMsgVersionFromConn(conn, nonce, 1, []uint32{1})
	if err != nil {
		t.Errorf("NewMsgVersionFromConn: %v", err)
	}

	// Ensure we get the correct connection data back out.
	if !msg.AddrMe.IP.Equal(tcpAddrMe.IP) {
		t.Errorf("NewMsgVersionFromConn: wrong me ip - got %v, want %v",
			msg.AddrMe.IP, tcpAddrMe.IP)
	}
	if !msg.AddrYou.IP.Equal(tcpAddrYou.IP) {
		t.Errorf("NewMsgVersionFromConn: wrong you ip - got %v, want %v",
			msg.AddrYou.IP, tcpAddrYou.IP)
	}

	// Use a fake connection with local UDP addresses to force a failure.
	conn = &fakeConn{
		localAddr:  &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8333},
		remoteAddr: tcpAddrYou,
	}
	msg, err = wire.NewMsgVersionFromConn(conn, nonce, 1, []uint32{1})
	if err != wire.ErrInvalidNetAddr {
		t.Errorf("NewMsgVersionFromConn: expected error not received "+
			"- got %v, want %v", err, wire.ErrInvalidNetAddr)
	}

	// Use a fake connection with remote UDP addresses to force a failure.
	conn = &fakeConn{
		localAddr:  tcpAddrMe,
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("192.168.0.1"), Port: 8333},
	}
	msg, err = wire.NewMsgVersionFromConn(conn, nonce, 1, []uint32{1})
	if err != wire.ErrInvalidNetAddr {
		t.Errorf("NewMsgVersionFromConn: expected error not received "+
			"- got %v, want %v", err, wire.ErrInvalidNetAddr)
	}

	return
}

// TestVersionWire tests the MsgVersion wire.encode and decode for various
// protocol versions.
func TestVersionWire(t *testing.T) {
	tests := []struct {
		in  *wire.MsgVersion // Message to encode
		out *wire.MsgVersion // Expected decoded message
		buf []byte           // Wire encoding
	}{
		// Latest protocol version.
		{
			baseVersion,
			baseVersion,
			baseVersionEncoded,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode the message to wire.format.
		var buf bytes.Buffer
		err := test.in.Encode(&buf)
		if err != nil {
			t.Errorf("Encode #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("Encode #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Decode the message from wire.format.
		var msg wire.MsgVersion
		rbuf := bytes.NewBuffer(test.buf)
		err = msg.Decode(rbuf)
		if err != nil {
			t.Errorf("Decode #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&msg, test.out) {
			t.Errorf("Decode #%d\n got: %s want: %s", i,
				spew.Sdump(msg), spew.Sdump(test.out))
			continue
		}
	}
}

// TestVersionWireErrors performs negative tests against wire.encode and
// decode of MsgGetHeaders to confirm error paths work correctly.
func TestVersionWireErrors(t *testing.T) {
	wireErr := &wire.MessageError{}

	// Ensure calling MsgVersion.Decode with a non *bytes.Buffer returns
	// error.
	fr := newFixedReader(0, []byte{})
	if err := baseVersion.Decode(fr); err == nil {
		t.Errorf("Did not received error when calling " +
			"MsgVersion.Decode with non *bytes.Buffer")
	}

	// Copy the base version and change the user agent to exceed max limits.
	bvc := *baseVersion
	exceedUAVer := &bvc
	newUA := "/" + strings.Repeat("t", wire.MaxUserAgentLen-8+1) + ":0.0.1/"
	exceedUAVer.UserAgent = newUA

	// Encode the new UA length as a varint.
	var newUAVarIntBuf bytes.Buffer
	err := wire.TstWriteVarInt(&newUAVarIntBuf, uint64(len(newUA)))
	if err != nil {
		t.Errorf("writeVarInt: error %v", err)
	}

	// Make a new buffer big enough to hold the base version plus the new
	// bytes for the bigger varint to hold the new size of the user agent
	// and the new user agent string.  Then stich it all together.
	newLen := len(baseVersionEncoded) - len(baseVersion.UserAgent)
	newLen = newLen + len(newUAVarIntBuf.Bytes()) - 1 + len(newUA)
	exceedUAVerEncoded := make([]byte, newLen)
	copy(exceedUAVerEncoded, baseVersionEncoded[0:80])
	copy(exceedUAVerEncoded[80:], newUAVarIntBuf.Bytes())
	copy(exceedUAVerEncoded[83:], []byte(newUA))
	copy(exceedUAVerEncoded[83+len(newUA):], baseVersionEncoded[99:])

	tests := []struct {
		in       *wire.MsgVersion // Value to encode
		buf      []byte           // Wire encoding
		max      int              // Max size of fixed buffer to induce errors
		writeErr error            // Expected write error
		readErr  error            // Expected read error
	}{
		// Force error in protocol version.
		{baseVersion, baseVersionEncoded, 0, io.ErrShortWrite, io.EOF},
		// Force error in services.
		{baseVersion, baseVersionEncoded, 4, io.ErrShortWrite, io.EOF},
		// Force error in timestamp.
		{baseVersion, baseVersionEncoded, 12, io.ErrShortWrite, io.EOF},
		// Force error in remote address.
		{baseVersion, baseVersionEncoded, 20, io.ErrShortWrite, io.EOF},
		// Force error in local address.
		{baseVersion, baseVersionEncoded, 47, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in nonce.
		{baseVersion, baseVersionEncoded, 73, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in user agent length.
		{baseVersion, baseVersionEncoded, 81, io.ErrShortWrite, io.EOF},
		// Force error in user agent.
		{baseVersion, baseVersionEncoded, 82, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in validating user agent
		{exceedUAVer, exceedUAVerEncoded, newLen, wireErr, wireErr},
		// Force error in stream numbers length.
		{baseVersion, baseVersionEncoded, 97, io.ErrShortWrite, io.EOF},
		// Force error in stream number.
		{baseVersion, baseVersionEncoded, 98, io.ErrShortWrite, io.EOF},
		// Force error for too many streams.
		{tooManyStreamsVersion, tooManyStreamsVersionEncoded, 300,
			fmt.Errorf("number of streams is too large: %v", 2),
			fmt.Errorf("number of streams is too large: %v", 2)},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire.format.
		w := newFixedWriter(test.max)
		err := test.in.Encode(w)
		if reflect.TypeOf(err) != reflect.TypeOf(test.writeErr) {
			t.Errorf("Encode #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// For errors which are not of type wire.MessageError, check
		// them for equality.
		if _, ok := err.(*wire.MessageError); !ok {
			//if !reflect.DeepEqual(err, test.readErr) {
			if err != test.writeErr && !reflect.DeepEqual(err, test.readErr) {
				t.Errorf("Encode #%d wrong error got: %v, "+
					"want: %v", i, err, test.writeErr)
				continue
			}
		}

		// Decode from wire.format.
		var msg wire.MsgVersion
		var buf io.Reader
		if test.max > len(test.buf) {
			buf = bytes.NewBuffer(test.buf[:])
		} else {
			buf = bytes.NewBuffer(test.buf[0:test.max])
		}
		err = msg.Decode(buf)
		if reflect.TypeOf(err) != reflect.TypeOf(test.readErr) {
			t.Errorf("Decode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}

		// For errors which are not of type wire.MessageError, check
		// them for equality.
		if _, ok := err.(*wire.MessageError); !ok {
			if !reflect.DeepEqual(err, test.readErr) {
				t.Errorf("Decode #%d wrong error got: %v, "+
					"want: %v", i, err, test.readErr)
				continue
			}
		}
	}
}

// baseVersion is used in the various tests as a baseline MsgVersion.
var baseVersion = &wire.MsgVersion{
	ProtocolVersion: 3,
	Services:        wire.SFNodeNetwork,
	Timestamp:       time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST)
	AddrYou: &wire.NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Stream:    0,           // Zero value -- no stream in version
		Services:  wire.SFNodeNetwork,
		IP:        net.ParseIP("192.168.0.1"),
		Port:      8333,
	},
	AddrMe: &wire.NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Stream:    0,           // Zero value -- no stream in version
		Services:  wire.SFNodeNetwork,
		IP:        net.ParseIP("127.0.0.1"),
		Port:      8333,
	},
	Nonce:         123123, // 0x1e0f3
	UserAgent:     "/wiretest:0.0.1/",
	StreamNumbers: []uint32{1},
}

// baseVersionEncoded is the wire.encoded bytes for baseVersion
var baseVersionEncoded = []byte{
	0x00, 0x00, 0x00, 0x03, // Protocol version 3
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, // SFNodeNetwork
	0x00, 0x00, 0x00, 0x00, 0x49, 0x5f, 0xab, 0x29, // 64-bit Timestamp
	// AddrYou -- No timestamp for NetAddress in version message
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, // SFNodeNetwork
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0xc0, 0xa8, 0x00, 0x01, // IP 192.168.0.1
	0x20, 0x8d, // Port 8333 in big-endian
	// AddrMe -- No timestamp for NetAddress in version message
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, // SFNodeNetwork
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x01, // IP 127.0.0.1
	0x20, 0x8d, // Port 8333 in big-endian
	0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0xe0, 0xf3, // Nonce
	0x10, // Varint for user agent length
	0x2f, 0x77, 0x69, 0x72, 0x65, 0x74, 0x65, 0x73,
	0x74, 0x3a, 0x30, 0x2e, 0x30, 0x2e, 0x31, 0x2f, // User agent
	0x01, 0x01, // Stream Numbers
}

// tooManyStreamsVersion is used to test a version message with too many streams.
var tooManyStreamsVersion = &wire.MsgVersion{
	ProtocolVersion: 3,
	Services:        wire.SFNodeNetwork,
	Timestamp:       time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST)
	AddrYou: &wire.NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Stream:    0,           // Zero value -- no stream in version
		Services:  wire.SFNodeNetwork,
		IP:        net.ParseIP("192.168.0.1"),
		Port:      8333,
	},
	AddrMe: &wire.NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Stream:    0,           // Zero value -- no stream in version
		Services:  wire.SFNodeNetwork,
		IP:        net.ParseIP("127.0.0.1"),
		Port:      8333,
	},
	Nonce:         123123, // 0x1e0f3
	UserAgent:     "/wiretest:0.0.1/",
	StreamNumbers: []uint32{1, 2},
}

// tooManyStreamsVersionEncoded is the wire.encoded bytes for tooManyStreamsVersion
var tooManyStreamsVersionEncoded = []byte{
	0x00, 0x00, 0x00, 0x03, // Protocol version 3
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, // SFNodeNetwork
	0x00, 0x00, 0x00, 0x00, 0x49, 0x5f, 0xab, 0x29, // 64-bit Timestamp
	// AddrYou -- No timestamp for NetAddress in version message
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, // SFNodeNetwork
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0xc0, 0xa8, 0x00, 0x01, // IP 192.168.0.1
	0x20, 0x8d, // Port 8333 in big-endian
	// AddrMe -- No timestamp for NetAddress in version message
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, // SFNodeNetwork
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x01, // IP 127.0.0.1
	0x20, 0x8d, // Port 8333 in big-endian
	0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0xe0, 0xf3, // Nonce
	0x10, // Varint for user agent length
	0x2f, 0x77, 0x69, 0x72, 0x65, 0x74, 0x65, 0x73,
	0x74, 0x3a, 0x30, 0x2e, 0x30, 0x2e, 0x31, 0x2f, // User agent
	0x02, 0x01, 0x02, // Stream Numbers
}
