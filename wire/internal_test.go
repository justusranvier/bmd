/*
This test file is part of the wire.package rather than than the wire.test
package so it can bridge access to the internals to properly test cases which
are either not possible or can't reliably be tested via the public interface.
The functions are only exported while the tests are being run.
*/

package wire

import (
	"io"
)

// TstRandomUint64 makes the internal randomUint64 function available to the
// test package.
func TstRandomUint64(r io.Reader) (uint64, error) {
	return randomUint64(r)
}

// TstReadElement makes the internal readElement function available to the
// test package.
func TstReadElement(r io.Reader, element interface{}) error {
	return readElement(r, element)
}

// TstWriteElement makes the internal writeElement function available to the
// test package.
func TstWriteElement(w io.Writer, element interface{}) error {
	return writeElement(w, element)
}

// TstReadVarInt makes the internal readVarInt function available to the
// test package.
func TstReadVarInt(r io.Reader) (uint64, error) {
	return readVarInt(r)
}

// TstWriteVarInt makes the internal writeVarInt function available to the
// test package.
func TstWriteVarInt(w io.Writer, val uint64) error {
	return writeVarInt(w, val)
}

// TstReadVarString makes the internal readVarString function available to the
// test package.
func TstReadVarString(r io.Reader) (string, error) {
	return readVarString(r)
}

// TstWriteVarString makes the internal writeVarString function available to the
// test package.
func TstWriteVarString(w io.Writer, str string) error {
	return writeVarString(w, str)
}

// TstReadVarBytes makes the internal readVarBytes function available to the
// test package.
func TstReadVarBytes(r io.Reader, maxAllowed uint32, fieldName string) ([]byte, error) {
	return readVarBytes(r, maxAllowed, fieldName)
}

// TstWriteVarBytes makes the internal writeVarBytes function available to the
// test package.
func TstWriteVarBytes(w io.Writer, bytes []byte) error {
	return writeVarBytes(w, bytes)
}

// TstReadNetAddress makes the internal readNetAddress function available to
// the test package.
func TstReadNetAddress(r io.Reader, na *NetAddress, ts bool) error {
	return readNetAddress(r, na, ts)
}

// TstWriteNetAddress makes the internal writeNetAddress function available to
// the test package.
func TstWriteNetAddress(w io.Writer, na *NetAddress, ts bool) error {
	return writeNetAddress(w, na, ts)
}

// TstMaxNetAddressPayload makes the internal maxNetAddressPayload function
// available to the test package.
func TstMaxNetAddressPayload() uint32 {
	return maxNetAddressPayload()
}

// TstReadInvVect makes the internal readInvVect function available to the test
// package.
func TstReadInvVect(r io.Reader, iv *InvVect) error {
	return readInvVect(r, iv)
}

// TstWriteInvVect makes the internal writeInvVect function available to the
// test package.
func TstWriteInvVect(w io.Writer, iv *InvVect) error {
	return writeInvVect(w, iv)
}
