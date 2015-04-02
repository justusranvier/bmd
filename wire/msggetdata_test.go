package wire_test

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/monetas/bmd/wire"
)

// TestGetData tests the MsgGetData API.
func TestGetData(t *testing.T) {
	// Ensure the command is expected value.
	wantCmd := "getdata"
	msg := wire.NewMsgGetData()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgGetData: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Num inventory vectors (varInt) + max allowed inventory vectors.
	wantPayload := 1600009
	maxPayload := msg.MaxPayloadLength()
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"got %v, want %v", maxPayload, wantPayload)
	}

	// Ensure inventory vectors are added properly.
	hash := wire.ShaHash{}
	iv := wire.NewInvVect(&hash)
	err := msg.AddInvVect(iv)
	if err != nil {
		t.Errorf("AddInvVect: %v", err)
	}
	if msg.InvList[0] != iv {
		t.Errorf("AddInvVect: wrong invvect added - got %v, want %v",
			spew.Sprint(msg.InvList[0]), spew.Sprint(iv))
	}

	// Ensure adding more than the max allowed inventory vectors per
	// message returns an error.
	for i := 0; i < wire.MaxInvPerMsg; i++ {
		err = msg.AddInvVect(iv)
	}
	if err == nil {
		t.Errorf("AddInvVect: expected error on too many inventory " +
			"vectors not received")
	}

	// Ensure creating the message with a size hint larger than the max
	// works as expected.
	msg = wire.NewMsgGetDataSizeHint(wire.MaxInvPerMsg + 1)
	wantCap := wire.MaxInvPerMsg
	if cap(msg.InvList) != wantCap {
		t.Errorf("NewMsgGetDataSizeHint: wrong cap for size hint - "+
			"got %v, want %v", cap(msg.InvList), wantCap)
	}

	return
}

// TestGetDataWire tests the MsgGetData wire.encode and decode for various
// numbers of inventory vectors and protocol versions.
func TestGetDataWire(t *testing.T) {
	hashStr := "1ee5d34b208ebe616943fcf2cc1ca0e948cc94f73fa4e94574bc105fa6174376"
	hash, err := wire.NewShaHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewShaHashFromStr: %v", err)
	}

	hashStr = "d28a3dc7392bf00a9855ee93dd9a81eff82a2c4fe57fbd42cfe71b487accfaf0"
	hash2, err := wire.NewShaHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewShaHashFromStr: %v", err)
	}

	iv := wire.NewInvVect(hash)
	iv2 := wire.NewInvVect(hash2)

	// Empty MsgGetData message.
	NoInv := wire.NewMsgGetData()
	NoInvEncoded := []byte{
		0x00, // Varint for number of inventory vectors
	}

	// MsgGetData message with multiple inventory vectors.
	MultiInv := wire.NewMsgGetData()
	MultiInv.AddInvVect(iv)
	MultiInv.AddInvVect(iv2)
	MultiInvEncoded := []byte{
		0x02, // Varint for number of inv vectors
		0x1E, 0xE5, 0xD3, 0x4B, 0x20, 0x8E, 0xBE, 0x61,
		0x69, 0x43, 0xFC, 0xF2, 0xCC, 0x1C, 0xA0, 0xE9,
		0x48, 0xCC, 0x94, 0xF7, 0x3F, 0xA4, 0xE9, 0x45,
		0x74, 0xBC, 0x10, 0x5F, 0xA6, 0x17, 0x43, 0x76,
		0xD2, 0x8A, 0x3D, 0xC7, 0x39, 0x2B, 0xF0, 0x0A,
		0x98, 0x55, 0xEE, 0x93, 0xDD, 0x9A, 0x81, 0xEF,
		0xF8, 0x2A, 0x2C, 0x4F, 0xE5, 0x7F, 0xBD, 0x42,
		0xCF, 0xE7, 0x1B, 0x48, 0x7A, 0xCC, 0xFA, 0xF0,
	}

	tests := []struct {
		in  *wire.MsgGetData // Message to encode
		out *wire.MsgGetData // Expected decoded message
		buf []byte           // Wire encoding
	}{
		// Latest protocol version with no inv vectors.
		{
			NoInv,
			NoInv,
			NoInvEncoded,
		},

		// Latest protocol version with multiple inv vectors.
		{
			MultiInv,
			MultiInv,
			MultiInvEncoded,
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
		var msg wire.MsgGetData
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

// TestGetDataWireErrors performs negative tests against wire.encode and decode
// of MsgGetData to confirm error paths work correctly.
func TestGetDataWireErrors(t *testing.T) {
	wireErr := &wire.MessageError{}

	hashStr := "1ee5d34b208ebe616943fcf2cc1ca0e948cc94f73fa4e94574bc105fa6174376"
	hash, err := wire.NewShaHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewShaHashFromStr: %v", err)
	}

	iv := wire.NewInvVect(hash)

	// Base message used to induce errors.
	baseGetData := wire.NewMsgGetData()
	baseGetData.AddInvVect(iv)
	baseGetDataEncoded := []byte{
		0x02, // Varint for number of inv vectors
		0x1E, 0xE5, 0xD3, 0x4B, 0x20, 0x8E, 0xBE, 0x61,
		0x69, 0x43, 0xFC, 0xF2, 0xCC, 0x1C, 0xA0, 0xE9,
		0x48, 0xCC, 0x94, 0xF7, 0x3F, 0xA4, 0xE9, 0x45,
		0x74, 0xBC, 0x10, 0x5F, 0xA6, 0x17, 0x43, 0x76,
	}

	// Message that forces an error by having more than the max allowed inv
	// vectors.
	maxGetData := wire.NewMsgGetData()
	for i := 0; i < wire.MaxInvPerMsg; i++ {
		maxGetData.AddInvVect(iv)
	}
	maxGetData.InvList = append(maxGetData.InvList, iv)
	maxGetDataEncoded := []byte{
		0xfd, 0xc3, 0x51, // Varint for number of inv vectors (50001)
	}

	tests := []struct {
		in       *wire.MsgGetData // Value to encode
		buf      []byte           // Wire encoding
		max      int              // Max size of fixed buffer to induce errors
		writeErr error            // Expected write error
		readErr  error            // Expected read error
	}{
		// Latest protocol version with intentional read/write errors.
		// Force error in inventory vector count
		{baseGetData, baseGetDataEncoded, 0, io.ErrShortWrite, io.EOF},
		// Force error in inventory list.
		{baseGetData, baseGetDataEncoded, 1, io.ErrShortWrite, io.EOF},
		// Force error with greater than max inventory vectors.
		{maxGetData, maxGetDataEncoded, 3, wireErr, wireErr},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire.format.
		w := newFixedWriter(test.max)
		err := test.in.Encode(w)
		if reflect.TypeOf(err) != reflect.TypeOf(test.writeErr) {
			t.Errorf("Encode #%d wrong error got: %v, want: %v",
				i, reflect.TypeOf(err), reflect.TypeOf(test.writeErr))
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
		var msg wire.MsgGetData
		r := newFixedReader(test.max, test.buf)
		err = msg.Decode(r)
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
