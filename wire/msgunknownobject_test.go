package wire_test

import (
	"bytes"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/monetas/bmd/wire"
)

// TestUnknownObject tests the MsgUnknownObject API.
func TestUnknownObject(t *testing.T) {

	// Ensure the command is expected value.
	wantCmd := "object"
	now := time.Now()
	payload := make([]byte, 99)
	msg := wire.NewMsgUnknownObject(83928, now, wire.ObjectType(17), 2, 1, payload)
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgUnknownObject: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	wantPayload := wire.MaxPayloadOfMsgObject
	maxPayload := msg.MaxPayloadLength()
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"- got %v, want %v", maxPayload, wantPayload)
	}

	str := msg.String()
	if str[:14] != "unknown object" {
		t.Errorf("String representation: got %v, want %v", str[:9], "unknown object")
	}

	return
}

// TestUnknownObjectWire tests the MsgUnknownObject wire.encode and decode for
// various versions.
func TestUnknownObjectWire(t *testing.T) {
	expires := time.Unix(0x495fab29, 0) // 2009-01-03 12:15:05 -0600 CST
	payload := make([]byte, 128)
	msgBase := wire.NewMsgUnknownObject(83928, expires, wire.ObjectType(22), 2, 1, payload)

	tests := []struct {
		in  *wire.MsgUnknownObject // Message to encode
		out *wire.MsgUnknownObject // Expected decoded message
		buf []byte                 // Wire encoding
	}{
		// Latest protocol version with multiple object vectors.
		{
			msgBase,
			msgBase,
			baseUnknownObjectEncoded,
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
		var msg wire.MsgUnknownObject
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

// TestUnknownObjectWireError tests the MsgUnknownObject error paths.
func TestUnknownObjectWireError(t *testing.T) {
	wireErr := &wire.MessageError{}

	// Ensure calling MsgVersion.Decode with a non *bytes.Buffer returns
	// error.
	fr := newFixedReader(0, []byte{})
	if err := baseMsg.Decode(fr); err == nil {
		t.Errorf("Did not receive error when calling " +
			"MsgVersion.Decode with non *bytes.Buffer")
	}

	wrongObjectTypeEncoded := make([]byte, len(baseMsgEncoded))
	copy(wrongObjectTypeEncoded, baseMsgEncoded)
	wrongObjectTypeEncoded[19] = 0

	tests := []struct {
		in       *wire.MsgUnknownObject // Value to encode
		buf      []byte                 // Wire encoding
		max      int                    // Max size of fixed buffer to induce errors
		writeErr error                  // Expected write error
		readErr  error                  // Expected read error
	}{
		// Force error in nonce
		{baseUnknownObject, baseUnknownObjectEncoded, 0, io.ErrShortWrite, io.EOF},
		// Force error in expirestime.
		{baseUnknownObject, baseUnknownObjectEncoded, 8, io.ErrShortWrite, io.EOF},
		// Force error in object type.
		{baseUnknownObject, baseUnknownObjectEncoded, 16, io.ErrShortWrite, io.EOF},
		// Force error in version.
		{baseUnknownObject, baseUnknownObjectEncoded, 20, io.ErrShortWrite, io.EOF},
		// Force error in stream number.
		{baseUnknownObject, baseUnknownObjectEncoded, 21, io.ErrShortWrite, io.EOF},
		// Force error object type validation.
		{baseUnknownObject, wrongObjectTypeEncoded, 52, io.ErrShortWrite, wireErr},
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
		var msg wire.MsgUnknownObject
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

// baseUnknownObject is used in the various tests as a baseline MsgUnknownObject.
var baseUnknownObject = &wire.MsgUnknownObject{
	Nonce:        123123,                   // 0x1e0f3
	ExpiresTime:  time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST)
	ObjectType:   wire.ObjectType(22),
	Version:      2,
	StreamNumber: 1,
	Payload: []byte{
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

// baseUnknownObjectEncoded is the wire.encoded bytes for baseUnknownObject
var baseUnknownObjectEncoded = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x47, 0xd8, // 83928 nonce
	0x00, 0x00, 0x00, 0x00, 0x49, 0x5f, 0xab, 0x29, // 64-bit Timestamp
	0x00, 0x00, 0x00, 0x16, // Object Type
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
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}
