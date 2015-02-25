package wire_test

import (
	"bytes"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/jimmysong/bmd/wire"
)

// TestBroadcast tests the MsgBroadcast API.
func TestBroadcast(t *testing.T) {

	// Ensure the command is expected value.
	wantCmd := "object"
	now := time.Now()
	enc := make([]byte, 99)
	msg := wire.NewMsgBroadcast(83928, now, 2, 1, nil, enc, 0, 0, 0, nil, nil, 0, 0, nil, 0, nil, nil)
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgBroadcast: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Num objectentory vectors (varInt) + max allowed objectentory vectors.
	wantPayload := uint32(1 << 18)
	maxPayload := msg.MaxPayloadLength()
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"- got %v, want %v", maxPayload, wantPayload)
	}

	str := msg.String()
	if str[:9] != "broadcast" {
		t.Errorf("String representation: got %v, want %v", str[:9], "broadcast")
	}

	return
}

// TestBroadcastWire tests the MsgBroadcast wire.encode and decode for
// various versions.
func TestBroadcastWire(t *testing.T) {
	expires := time.Unix(0x495fab29, 0) // 2009-01-03 12:15:05 -0600 CST)
	enc := make([]byte, 128)
	msgBase := wire.NewMsgBroadcast(83928, expires, 2, 1, nil, enc, 0, 0, 0, nil, nil, 0, 0, nil, 0, nil, nil)
	ripeBytes := make([]byte, 20)
	ripe, err := wire.NewRipeHash(ripeBytes)
	if err != nil {
		t.Fatalf("could not make a ripe hash %s", err)
	}
	m := make([]byte, 32)
	a := make([]byte, 8)
	tagBytes := make([]byte, 32)
	tag, err := wire.NewShaHash(tagBytes)
	if err != nil {
		t.Fatalf("could not make a sha hash %s", err)
	}
	msgTagged := wire.NewMsgBroadcast(83928, expires, 5, 1, tag, enc, 1, 1, 1, pubKey1, pubKey2, 512, 512, ripe, 0, m, a)
	msgBaseAndTag := wire.NewMsgBroadcast(83928, expires, 5, 1, tag, enc, 0, 0, 0, nil, nil, 0, 0, nil, 0, nil, nil)

	tests := []struct {
		in  *wire.MsgBroadcast // Message to encode
		out *wire.MsgBroadcast // Expected decoded message
		buf []byte               // Wire encoding
	}{
		// Latest protocol version with multiple object vectors.
		{
			msgBase,
			msgBase,
			baseBroadcastEncoded,
		},
		{
			msgTagged,
			msgBaseAndTag,
			tagBroadcastEncoded,
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
		var msg wire.MsgBroadcast
		rbuf := bytes.NewReader(test.buf)
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

// TestBroadcastWireError tests the MsgBroadcast error paths
func TestBroadcastWireError(t *testing.T) {
	wireErr := &wire.MessageError{}

	// Ensure calling MsgVersion.Decode with a non *bytes.Buffer returns
	// error.
	fr := newFixedReader(0, []byte{})
	if err := baseMsg.Decode(fr); err == nil {
		t.Errorf("Did not received error when calling " +
			"MsgVersion.Decode with non *bytes.Buffer")
	}

	wrongObjectTypeEncoded := make([]byte, len(baseMsgEncoded))
	copy(wrongObjectTypeEncoded, baseMsgEncoded)
	wrongObjectTypeEncoded[19] = 0

	tests := []struct {
		in       *wire.MsgBroadcast // Value to encode
		buf      []byte               // Wire encoding
		max      int                  // Max size of fixed buffer to induce errors
		writeErr error                // Expected write error
		readErr  error                // Expected read error
	}{
		// Force error in nonce
		{baseBroadcast, baseBroadcastEncoded, 0, io.ErrShortWrite, io.EOF},
		// Force error in expirestime.
		{baseBroadcast, baseBroadcastEncoded, 8, io.ErrShortWrite, io.EOF},
		// Force error in object type.
		{baseBroadcast, baseBroadcastEncoded, 16, io.ErrShortWrite, io.EOF},
		// Force error in version.
		{baseBroadcast, baseBroadcastEncoded, 20, io.ErrShortWrite, io.EOF},
		// Force error in stream number.
		{baseBroadcast, baseBroadcastEncoded, 21, io.ErrShortWrite, io.EOF},
		// Force error object type validation.
		{baseBroadcast, wrongObjectTypeEncoded, 52, io.ErrShortWrite, wireErr},
		// Force error in tag.
		{taggedBroadcast, tagBroadcastEncoded, 22, io.ErrShortWrite, io.EOF},
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
			if err != test.writeErr {
				t.Errorf("Encode #%d wrong error got: %v, "+
					"want: %v", i, err, test.writeErr)
				continue
			}
		}

		// Decode from wire.format.
		var msg wire.MsgBroadcast
		buf := bytes.NewBuffer(test.buf[0:test.max])
		err = msg.Decode(buf)
		if reflect.TypeOf(err) != reflect.TypeOf(test.readErr) {
			t.Errorf("Decode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}

		// For errors which are not of type wire.MessageError, check
		// them for equality.
		if _, ok := err.(*wire.MessageError); !ok {
			if err != test.readErr {
				t.Errorf("Decode #%d wrong error got: %v, "+
					"want: %v", i, err, test.readErr)
				continue
			}
		}
	}
}

// baseBroadcast is used in the various tests as a baseline MsgBroadcast.
var baseBroadcast = &wire.MsgBroadcast{
	Nonce:        123123,                   // 0x1e0f3
	ExpiresTime:  time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST)
	ObjectType:   wire.ObjectTypeBroadcast,
	Version:      2,
	StreamNumber: 1,
	Encrypted: []byte{
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	},
}

// baseBroadcast is used in the various tests as a baseline MsgBroadcast.
var taggedBroadcast = &wire.MsgBroadcast{
	Nonce:        123123,                   // 0x1e0f3
	ExpiresTime:  time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST)
	ObjectType:   wire.ObjectTypeBroadcast,
	Version:      5,
	StreamNumber: 1,
	Tag: &wire.ShaHash{
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	},
	Encrypted: []byte{
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	},
}

// baseBroadcastEncoded is the wire.encoded bytes for baseBroadcast (just encrypted data)
var baseBroadcastEncoded = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x47, 0xd8, // 83928 nonce
	0x00, 0x00, 0x00, 0x00, 0x49, 0x5f, 0xab, 0x29, // 64-bit Timestamp
	0x00, 0x00, 0x00, 0x03, // Object Type
	0x02, // Version
	0x01, // Stream Number
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Encrypted Data
}

// baseBroadcastEncoded is the wire.encoded bytes for baseBroadcast (just encrypted data)
var tagBroadcastEncoded = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x47, 0xd8, // 83928 nonce
	0x00, 0x00, 0x00, 0x00, 0x49, 0x5f, 0xab, 0x29, // 64-bit Timestamp
	0x00, 0x00, 0x00, 0x03, // Object Type
	0x05, // Version
	0x01, // Stream Number
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Tag
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Encrypted Data
}
