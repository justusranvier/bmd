package wire_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/monetas/bmd/wire"
)

// fakeRandReader implements the io.Reader interface and is used to force
// errors in the RandomUint64 function.
type fakeRandReader struct {
	n   int
	err error
}

// Read returns the fake reader error and the lesser of the fake reader value
// and the length of p.
func (r *fakeRandReader) Read(p []byte) (int, error) {
	n := r.n
	if n > len(p) {
		n = len(p)
	}
	return n, r.err
}

// TestElementWire tests wire.encode and decode for various element types.  This
// is mainly to test the "fast" paths in readElement and writeElement which use
// type assertions to avoid reflection when possible.
func TestElementWire(t *testing.T) {
	type writeElementReflect int32

	tests := []struct {
		in  interface{} // Value to encode
		buf []byte      // Wire encoding
	}{
		{int32(1), []byte{0x00, 0x00, 0x00, 0x01}},
		{uint32(256), []byte{0x00, 0x00, 0x01, 0x00}},
		{
			int64(65536),
			[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00},
		},
		{
			uint64(4294967296),
			[]byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00},
		},
		{
			true,
			[]byte{0x01},
		},
		{
			false,
			[]byte{0x00},
		},
		{
			[4]byte{0x01, 0x02, 0x03, 0x04},
			[]byte{0x01, 0x02, 0x03, 0x04},
		},
		{
			[wire.CommandSize]byte{
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0a, 0x0b, 0x0c,
			},
			[]byte{
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0a, 0x0b, 0x0c,
			},
		},
		{
			[16]byte{
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
			},
			[]byte{
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
			},
		},
		{
			(*wire.ShaHash)(&[wire.HashSize]byte{ // Make go vet happy.
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
				0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
				0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
			}),
			[]byte{
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
				0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
				0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
			},
		},
		{
			(*wire.RipeHash)(&[wire.RipeHashSize]byte{ // Make go vet happy.
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
				0x11, 0x12, 0x13, 0x14,
			}),
			[]byte{
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
				0x11, 0x12, 0x13, 0x14,
			},
		},
		{
			wire.ServiceFlag(wire.SFNodeNetwork),
			[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
		},
		{
			wire.BitmessageNet(wire.MainNet),
			[]byte{0xe9, 0xbe, 0xb4, 0xd9},
		},
		// Type not supported by the "fast" path and requires reflection.
		{
			writeElementReflect(1),
			[]byte{0x00, 0x00, 0x00, 0x01},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Write to wire.format.
		var buf bytes.Buffer
		err := wire.TstWriteElement(&buf, test.in)
		if err != nil {
			t.Errorf("writeElement #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("writeElement #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Read from wire.format.
		rbuf := bytes.NewReader(test.buf)
		val := test.in
		if reflect.ValueOf(test.in).Kind() != reflect.Ptr {
			val = reflect.New(reflect.TypeOf(test.in)).Interface()
		}
		err = wire.TstReadElement(rbuf, val)
		if err != nil {
			t.Errorf("readElement #%d error %v", i, err)
			continue
		}
		ival := val
		if reflect.ValueOf(test.in).Kind() != reflect.Ptr {
			ival = reflect.Indirect(reflect.ValueOf(val)).Interface()
		}
		if !reflect.DeepEqual(ival, test.in) {
			t.Errorf("readElement #%d\n got: %s want: %s", i,
				spew.Sdump(ival), spew.Sdump(test.in))
			continue
		}
	}
}

// TestElementWireErrors performs negative tests against wire.encode and decode
// of various element types to confirm error paths work correctly.
func TestElementWireErrors(t *testing.T) {
	tests := []struct {
		in       interface{} // Value to encode
		max      int         // Max size of fixed buffer to induce errors
		writeErr error       // Expected write error
		readErr  error       // Expected read error
	}{
		{int32(1), 0, io.ErrShortWrite, io.EOF},
		{uint32(256), 0, io.ErrShortWrite, io.EOF},
		{int64(65536), 0, io.ErrShortWrite, io.EOF},
		{true, 0, io.ErrShortWrite, io.EOF},
		{[4]byte{0x01, 0x02, 0x03, 0x04}, 0, io.ErrShortWrite, io.EOF},
		{
			[wire.CommandSize]byte{
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0a, 0x0b, 0x0c,
			},
			0, io.ErrShortWrite, io.EOF,
		},
		{
			[16]byte{
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
			},
			0, io.ErrShortWrite, io.EOF,
		},
		{
			(*wire.ShaHash)(&[wire.HashSize]byte{ // Make go vet happy.
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
				0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
				0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
			}),
			0, io.ErrShortWrite, io.EOF,
		},
		{
			(*wire.RipeHash)(&[wire.RipeHashSize]byte{ // Make go vet happy.
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
				0x11, 0x12, 0x13, 0x14,
			}),
			0, io.ErrShortWrite, io.EOF,
		},
		{wire.ServiceFlag(wire.SFNodeNetwork), 0, io.ErrShortWrite, io.EOF},
		{wire.BitmessageNet(wire.MainNet), 0, io.ErrShortWrite, io.EOF},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire.format.
		w := newFixedWriter(test.max)
		err := wire.TstWriteElement(w, test.in)
		if err != test.writeErr {
			t.Errorf("writeElement #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// Decode from wire.format.
		r := newFixedReader(test.max, nil)
		val := test.in
		if reflect.ValueOf(test.in).Kind() != reflect.Ptr {
			val = reflect.New(reflect.TypeOf(test.in)).Interface()
		}
		err = wire.TstReadElement(r, val)
		if err != test.readErr {
			t.Errorf("readElement #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}

// TestVarIntWire tests wire.encode and decode for variable length integers.
func TestVarIntWire(t *testing.T) {
	tests := []struct {
		in  uint64 // Value to encode
		out uint64 // Expected decoded value
		buf []byte // Wire encoding
	}{
		// Latest protocol version.
		// Single byte
		{0, 0, []byte{0x00}},
		// Max single byte
		{0xfc, 0xfc, []byte{0xfc}},
		// Min 2-byte
		{0xfd, 0xfd, []byte{0xfd, 0x00, 0xfd}},
		// Max 2-byte
		{0xffff, 0xffff, []byte{0xfd, 0xff, 0xff}},
		// Min 4-byte
		{0x10000, 0x10000, []byte{0xfe, 0x00, 0x01, 0x00, 0x00}},
		// Max 4-byte
		{0xffffffff, 0xffffffff, []byte{0xfe, 0xff, 0xff, 0xff, 0xff}},
		// Min 8-byte
		{
			0x100000000, 0x100000000,
			[]byte{0xff, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00},
		},
		// Max 8-byte
		{
			0xffffffffffffffff, 0xffffffffffffffff,
			[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire.format.
		var buf bytes.Buffer
		err := wire.TstWriteVarInt(&buf, test.in)
		if err != nil {
			t.Errorf("writeVarInt #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("writeVarInt #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Decode from wire.format.
		rbuf := bytes.NewReader(test.buf)
		val, err := wire.TstReadVarInt(rbuf)
		if err != nil {
			t.Errorf("readVarInt #%d error %v", i, err)
			continue
		}
		if val != test.out {
			t.Errorf("readVarInt #%d\n got: %d want: %d", i,
				val, test.out)
			continue
		}
	}
}

// TestVarIntWireErrors performs negative tests against wire.encode and decode
// of variable length integers to confirm error paths work correctly.
func TestVarIntWireErrors(t *testing.T) {
	tests := []struct {
		in       uint64 // Value to encode
		buf      []byte // Wire encoding
		max      int    // Max size of fixed buffer to induce errors
		writeErr error  // Expected write error
		readErr  error  // Expected read error
	}{
		// Force errors on discriminant.
		{0, []byte{0x00}, 0, io.ErrShortWrite, io.EOF},
		// Force errors on 2-byte read/write.
		{0xfd, []byte{0xfd}, 2, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force errors on 4-byte read/write.
		{0x10000, []byte{0xfe}, 2, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force errors on 8-byte read/write.
		{0x100000000, []byte{0xff}, 2, io.ErrShortWrite, io.ErrUnexpectedEOF},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire.format.
		w := newFixedWriter(test.max)
		err := wire.TstWriteVarInt(w, test.in)
		if err != test.writeErr {
			t.Errorf("writeVarInt #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// Decode from wire.format.
		r := newFixedReader(test.max, test.buf)
		_, err = wire.TstReadVarInt(r)
		if err != test.readErr {
			t.Errorf("readVarInt #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}

// TestVarIntWire tests the serialize size for variable length integers.
func TestVarIntSerializeSize(t *testing.T) {
	tests := []struct {
		val  uint64 // Value to get the serialized size for
		size int    // Expected serialized size
	}{
		// Single byte
		{0, 1},
		// Max single byte
		{0xfc, 1},
		// Min 2-byte
		{0xfd, 3},
		// Max 2-byte
		{0xffff, 3},
		// Min 4-byte
		{0x10000, 5},
		// Max 4-byte
		{0xffffffff, 5},
		// Min 8-byte
		{0x100000000, 9},
		// Max 8-byte
		{0xffffffffffffffff, 9},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		serializedSize := wire.VarIntSerializeSize(test.val)
		if serializedSize != test.size {
			t.Errorf("VarIntSerializeSize #%d got: %d, want: %d", i,
				serializedSize, test.size)
			continue
		}
	}
}

// TestVarStringWire tests wire.encode and decode for variable length strings.
func TestVarStringWire(t *testing.T) {
	// str256 is a string that takes a 2-byte varint to encode.
	str256 := strings.Repeat("test", 64)

	tests := []struct {
		in  string // String to encode
		out string // String to decoded value
		buf []byte // Wire encoding
	}{
		// Latest protocol version.
		// Empty string
		{"", "", []byte{0x00}},
		// Single byte varint + string
		{"Test", "Test", append([]byte{0x04}, []byte("Test")...)},
		// 2-byte varint + string
		{str256, str256, append([]byte{0xfd, 0x01, 0x00}, []byte(str256)...)},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire.format.
		var buf bytes.Buffer
		err := wire.TstWriteVarString(&buf, test.in)
		if err != nil {
			t.Errorf("writeVarString #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("writeVarString #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Decode from wire.format.
		rbuf := bytes.NewReader(test.buf)
		val, err := wire.TstReadVarString(rbuf)
		if err != nil {
			t.Errorf("readVarString #%d error %v", i, err)
			continue
		}
		if val != test.out {
			t.Errorf("readVarString #%d\n got: %s want: %s", i,
				val, test.out)
			continue
		}
	}
}

// TestVarStringWireErrors performs negative tests against wire.encode and
// decode of variable length strings to confirm error paths work correctly.
func TestVarStringWireErrors(t *testing.T) {
	// str256 is a string that takes a 2-byte varint to encode.
	str256 := strings.Repeat("test", 64)

	tests := []struct {
		in       string // Value to encode
		buf      []byte // Wire encoding
		max      int    // Max size of fixed buffer to induce errors
		writeErr error  // Expected write error
		readErr  error  // Expected read error
	}{
		// Latest protocol version with intentional read/write errors.
		// Force errors on empty string.
		{"", []byte{0x00}, 0, io.ErrShortWrite, io.EOF},
		// Force error on single byte varint + string.
		{"Test", []byte{0x04}, 2, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force errors on 2-byte varint + string.
		{str256, []byte{0xfd}, 2, io.ErrShortWrite, io.ErrUnexpectedEOF},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire.format.
		w := newFixedWriter(test.max)
		err := wire.TstWriteVarString(w, test.in)
		if err != test.writeErr {
			t.Errorf("writeVarString #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// Decode from wire.format.
		r := newFixedReader(test.max, test.buf)
		_, err = wire.TstReadVarString(r)
		if err != test.readErr {
			t.Errorf("readVarString #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}

// TestVarStringOverflowErrors performs tests to ensure deserializing variable
// length strings intentionally crafted to use large values for the string
// length are handled properly. This could otherwise potentially be used as an
// attack vector.
func TestVarStringOverflowErrors(t *testing.T) {
	tests := []struct {
		buf []byte // Wire encoding
		err error  // Expected error
	}{
		{[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			&wire.MessageError{}},
		{[]byte{0xff, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			&wire.MessageError{}},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Decode from wire.format.
		rbuf := bytes.NewReader(test.buf)
		_, err := wire.TstReadVarString(rbuf)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("readVarString #%d wrong error got: %v, "+
				"want: %v", i, err, reflect.TypeOf(test.err))
			continue
		}
	}

}

// TestVarBytesWire tests wire.encode and decode for variable length byte array.
func TestVarBytesWire(t *testing.T) {
	// bytes256 is a byte array that takes a 2-byte varint to encode.
	bytes256 := bytes.Repeat([]byte{0x01}, 256)

	tests := []struct {
		in  []byte // Byte Array to write
		buf []byte // Wire encoding
	}{
		// Latest protocol version.
		// Empty byte array
		{[]byte{}, []byte{0x00}},
		// Single byte varint + byte array
		{[]byte{0x01}, []byte{0x01, 0x01}},
		// 2-byte varint + byte array
		{bytes256, append([]byte{0xfd, 0x01, 0x00}, bytes256...)},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire.format.
		var buf bytes.Buffer
		err := wire.TstWriteVarBytes(&buf, test.in)
		if err != nil {
			t.Errorf("writeVarBytes #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("writeVarBytes #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Decode from wire.format.
		rbuf := bytes.NewReader(test.buf)
		val, err := wire.TstReadVarBytes(rbuf,
			wire.MaxMessagePayload, "test payload")
		if err != nil {
			t.Errorf("readVarBytes #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("readVarBytes #%d\n got: %s want: %s", i,
				val, test.buf)
			continue
		}
	}
}

// TestVarBytesWireErrors performs negative tests against wire.encode and
// decode of variable length byte arrays to confirm error paths work correctly.
func TestVarBytesWireErrors(t *testing.T) {
	// bytes256 is a byte array that takes a 2-byte varint to encode.
	bytes256 := bytes.Repeat([]byte{0x01}, 256)

	tests := []struct {
		in       []byte // Byte Array to write
		buf      []byte // Wire encoding
		max      int    // Max size of fixed buffer to induce errors
		writeErr error  // Expected write error
		readErr  error  // Expected read error
	}{
		// Latest protocol version with intentional read/write errors.
		// Force errors on empty byte array.
		{[]byte{}, []byte{0x00}, 0, io.ErrShortWrite, io.EOF},
		// Force error on single byte varint + byte array.
		{[]byte{0x01, 0x02, 0x03}, []byte{0x04}, 2, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force errors on 2-byte varint + byte array.
		{bytes256, []byte{0xfd}, 2, io.ErrShortWrite, io.ErrUnexpectedEOF},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire.format.
		w := newFixedWriter(test.max)
		err := wire.TstWriteVarBytes(w, test.in)
		if err != test.writeErr {
			t.Errorf("writeVarBytes #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// Decode from wire.format.
		r := newFixedReader(test.max, test.buf)
		_, err = wire.TstReadVarBytes(r,
			wire.MaxMessagePayload, "test payload")
		if err != test.readErr {
			t.Errorf("readVarBytes #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}

// TestVarBytesOverflowErrors performs tests to ensure deserializing variable
// length byte arrays intentionally crafted to use large values for the array
// length are handled properly. This could otherwise potentially be used as an
// attack vector.
func TestVarBytesOverflowErrors(t *testing.T) {
	tests := []struct {
		buf []byte // Wire encoding
		err error  // Expected error
	}{
		{[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			&wire.MessageError{}},
		{[]byte{0xff, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			&wire.MessageError{}},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Decode from wire.format.
		rbuf := bytes.NewReader(test.buf)
		_, err := wire.TstReadVarBytes(rbuf,
			wire.MaxMessagePayload, "test payload")
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("readVarBytes #%d wrong error got: %v, "+
				"want: %v", i, err, reflect.TypeOf(test.err))
			continue
		}
	}

}

// TestRandomUint64 exercises the randomness of the random number generator on
// the system by ensuring the probability of the generated numbers. If the RNG
// is evenly distributed as a proper cryptographic RNG should be, there really
// should only be 1 number < 2^56 in 2^8 tries for a 64-bit number. However,
// use a higher number of 5 to really ensure the test doesn't fail unless the
// RNG is just horrendous.
func TestRandomUint64(t *testing.T) {
	tries := 1 << 8              // 2^8
	watermark := uint64(1 << 56) // 2^56
	maxHits := 5
	badRNG := "The random number generator on this system is clearly " +
		"terrible since we got %d values less than %d in %d runs " +
		"when only %d was expected"

	numHits := 0
	for i := 0; i < tries; i++ {
		nonce, err := wire.RandomUint64()
		if err != nil {
			t.Errorf("RandomUint64 iteration %d failed - err %v",
				i, err)
			return
		}
		if nonce < watermark {
			numHits++
		}
		if numHits > maxHits {
			str := fmt.Sprintf(badRNG, numHits, watermark, tries, maxHits)
			t.Errorf("Random Uint64 iteration %d failed - %v %v", i,
				str, numHits)
			return
		}
	}
}

// TestRandomUint64Errors uses a fake reader to force error paths to be executed
// and checks the results accordingly.
func TestRandomUint64Errors(t *testing.T) {
	// Test short reads.
	fr := &fakeRandReader{n: 2, err: io.EOF}
	nonce, err := wire.TstRandomUint64(fr)
	if err != io.ErrUnexpectedEOF {
		t.Errorf("TestRandomUint64Fails: Error not expected value of %v [%v]",
			io.ErrShortBuffer, err)
	}
	if nonce != 0 {
		t.Errorf("TestRandomUint64Fails: nonce is not 0 [%v]", nonce)
	}
}

// TestDoubleSha512 checks some test cases for DoubleSha512.
func TestDoubleSha512(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"",
			"826df068457df5dd195b437ab7e7739ff75d2672183f02bb8e1089fabcf97bd9dc80110cf42dbc7cff41c78ecb68d8ba78abe6b5178dea3984df8c55541bf949",
		}, {
			".",
			"4d4da299f2e2b044f8bf0169c7d7141fe27b5642420a1997bc1aeafbd6b1f5502f47a53bb9801748ca332f581fe21d9d588b67f753a31fe8df27baca8e233712",
		}, {
			" ",
			"5cf0a3d5f5465e60ceea891fb3670be6b5d8d9989d811aa7844e938a93c0ea7602639d3db0ceeb63742bc2bfcc476a0b961e2d757f24e3662b3b4ad47fac34d4",
		}, {
			"Jackdaws love my big sphynx of quartz.",
			"1aa604755e9b20b10b4b997ac4d665fda90e0e813f6d6470630b6585b248e1ef6f0915cd735ecf45d12b3af8072843ce241d00a093fe029214e0ec2761fb92c1",
		}, {
			"The quick brown fox jumps over the lazy dog.",
			"363e3c576d54f8ea3d9b6810594ce734607ad04c4cef103c73548a845a183cee09658f9f8f3e44cbb940a2b7767e1002f920ae24de07451ab26263425290c94c",
		},
	}

	for _, test := range tests {
		byteSlice := []byte(test.input)
		result := wire.DoubleSha512(byteSlice)
		expected := wire.Sha512(wire.Sha512(byteSlice))
		if !bytes.Equal(expected, result) {
			t.Errorf("DoubleSha512 fails for case \"%s\" against Sha512: expected %s, got %s", byteSlice, expected, result)
		}
		expected, _ = hex.DecodeString(test.expected)
		if !bytes.Equal(expected, result) {
			t.Errorf("DoubleSha512 fails for case \"%s\" against preset string: expected %s, got %s", byteSlice, expected, result)
		}
	}
}
