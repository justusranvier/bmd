package wire_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/jimmysong/bmd/wire"
)

// TestVerAck tests the MsgVerAck API.
func TestVerAck(t *testing.T) {
	// Ensure the command is expected value.
	wantCmd := "verack"
	msg := wire.NewMsgVerAck()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgVerAck: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value.
	wantPayload := uint32(0)
	maxPayload := msg.MaxPayloadLength()
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"got %v, want %v", maxPayload, wantPayload)
	}

	return
}

// TestVerAckWire tests the MsgVerAck wire.encode and decode for various
// protocol versions.
func TestVerAckWire(t *testing.T) {
	msgVerAck := wire.NewMsgVerAck()
	msgVerAckEncoded := []byte{}

	tests := []struct {
		in  *wire.MsgVerAck // Message to encode
		out *wire.MsgVerAck // Expected decoded message
		buf []byte            // Wire encoding
	}{
		// Latest protocol version.
		{
			msgVerAck,
			msgVerAck,
			msgVerAckEncoded,
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
		var msg wire.MsgVerAck
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
