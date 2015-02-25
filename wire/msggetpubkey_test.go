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

// TestGetPubKey tests the MsgGetPubKey API.
func TestGetPubKey(t *testing.T) {
	// Ensure the command is expected value.
	wantCmd := "object"
	now := time.Now()
	// ripe-based getpubkey message
	ripeBytes := make([]byte, 20)
	ripeBytes[0] = 1
	ripe, err := wire.NewRipeHash(ripeBytes)
	if err != nil {
		t.Fatalf("could not make a ripe hash %s", err)
	}
	msg := wire.NewMsgGetPubKey(83928, now, 2, 1, ripe, nil)
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgGetPubKey: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Num objectentory vectors (varInt) + max allowed objectentory vectors.
	wantPayload := uint32(1 << 18)
	maxPayload := msg.MaxPayloadLength()
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"got %v, want %v", maxPayload, wantPayload)
	}

	str := msg.String()
	if str[:9] != "getpubkey" {
		t.Errorf("String representation: got %v, want %v", str[:9], "getpubkey")
	}

	return
}

// TestGetPubKeyWire tests the MsgGetPubKey wire.encode and decode for various numbers
// of objectentory vectors and protocol versions.
func TestGetPubKeyWire(t *testing.T) {

	ripeBytes := make([]byte, 20)
	ripeBytes[0] = 1
	ripe, err := wire.NewRipeHash(ripeBytes)
	if err != nil {
		t.Fatalf("could not make a ripe hash %s", err)
	}

	expires := time.Unix(0x495fab29, 0) // 2009-01-03 12:15:05 -0600 CST)

	// empty tag, something in ripe
	msgRipe := wire.NewMsgGetPubKey(83928, expires, 2, 1, ripe, nil)

	// empty ripe, something in tag
	tagBytes := make([]byte, 32)
	tagBytes[0] = 1
	tag, err := wire.NewShaHash(tagBytes)
	if err != nil {
		t.Fatalf("could not make a tag hash %s", err)
	}
	msgTag := wire.NewMsgGetPubKey(83928, expires, 4, 1, nil, tag)

	RipeEncoded := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x47, 0xd8, // 83928 nonce
		0x00, 0x00, 0x00, 0x00, 0x49, 0x5f, 0xab, 0x29, // 64-bit timestamp
		0x00, 0x00, 0x00, 0x00, // object type (GETPUBKEY)
		0x02, // object version
		0x01, // stream number
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, // 20-byte ripemd
	}

	TagEncoded := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x47, 0xd8, // 83928 nonce
		0x00, 0x00, 0x00, 0x00, 0x49, 0x5f, 0xab, 0x29, // 64-bit timestamp
		0x00, 0x00, 0x00, 0x00, // object type (GETPUBKEY)
		0x04, // object version
		0x01, // stream number
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // 32-byte ripemd
	}

	tests := []struct {
		in  *wire.MsgGetPubKey // Message to encode
		out *wire.MsgGetPubKey // Expected decoded message
		buf []byte               // Wire encoding
	}{
		// Latest protocol version with multiple object vectors.
		{
			msgRipe,
			msgRipe,
			RipeEncoded,
		},
		{
			msgTag,
			msgTag,
			TagEncoded,
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
		var msg wire.MsgGetPubKey
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

// TestGetPubKeyWireError tests the MsgGetPubKey error paths
func TestGetPubKeyWireError(t *testing.T) {
	wireErr := &wire.MessageError{}

	// Ensure calling MsgVersion.Decode with a non *bytes.Buffer returns
	// error.
	fr := newFixedReader(0, []byte{})
	if err := baseVersion.Decode(fr); err == nil {
		t.Errorf("Did not received error when calling " +
			"MsgVersion.Decode with non *bytes.Buffer")
	}

	tests := []struct {
		in       *wire.MsgGetPubKey // Value to encode
		buf      []byte               // Wire encoding
		max      int                  // Max size of fixed buffer to induce errors
		writeErr error                // Expected write error
		readErr  error                // Expected read error
	}{
		// Force error in nonce
		{baseGetPubKey, baseGetPubKeyEncoded, 0, io.ErrShortWrite, io.EOF},
		// Force error in expirestime.
		{baseGetPubKey, baseGetPubKeyEncoded, 8, io.ErrShortWrite, io.EOF},
		// Force error in object type.
		{baseGetPubKey, baseGetPubKeyEncoded, 16, io.ErrShortWrite, io.EOF},
		// Force error in version.
		{baseGetPubKey, baseGetPubKeyEncoded, 20, io.ErrShortWrite, io.EOF},
		// Force error in stream number.
		{baseGetPubKey, baseGetPubKeyEncoded, 21, io.ErrShortWrite, io.EOF},
		// Force error in ripe.
		{baseGetPubKey, baseGetPubKeyEncoded, 22, io.ErrShortWrite, io.EOF},
		// Force error in tag.
		{tagGetPubKey, tagGetPubKeyEncoded, 22, io.ErrShortWrite, io.EOF},
		// Force error object type validation.
		{baseGetPubKey, basePubKeyEncoded, 20, io.ErrShortWrite, wireErr},
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
		var msg wire.MsgGetPubKey
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

// baseGetPubKey is used in the various tests as a baseline MsgGetPubKey.
var baseGetPubKey = &wire.MsgGetPubKey{
	Nonce:        123123,                   // 0x1e0f3
	ExpiresTime:  time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST)
	ObjectType:   wire.ObjectTypeGetPubKey,
	Version:      3,
	StreamNumber: 1,
	Ripe:         &wire.RipeHash{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	Tag:          nil,
}

// baseGetPubKeyEncoded is the wire.encoded bytes for baseGetPubKey
// using version 2 (pre-tag
var baseGetPubKeyEncoded = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0xe0, 0xf3, // Nonce
	0x00, 0x00, 0x00, 0x00, 0x49, 0x5f, 0xab, 0x29, // 64-bit Timestamp
	0x00, 0x00, 0x00, 0x00, // object type
	0x03, // Version
	0x01, // Stream Number
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, // Ripe
}

// baseGetPubKey is used in the various tests as a baseline MsgGetPubKey.
var tagGetPubKey = &wire.MsgGetPubKey{
	Nonce:        123123,                   // 0x1e0f3
	ExpiresTime:  time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST)
	ObjectType:   wire.ObjectTypeGetPubKey,
	Version:      4,
	StreamNumber: 1,
	Ripe:         nil,
	Tag: &wire.ShaHash{
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
}

// baseGetPubKeyEncoded is the wire.encoded bytes for baseGetPubKey
// using version 2 (pre-tag
var tagGetPubKeyEncoded = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0xe0, 0xf3, // Nonce
	0x00, 0x00, 0x00, 0x00, 0x49, 0x5f, 0xab, 0x29, // 64-bit Timestamp
	0x00, 0x00, 0x00, 0x00, // object type
	0x04, // Version
	0x01, // Stream Number
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Ripe
}
