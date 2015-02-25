package wire

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Maximum payload size for a variable length integer.
const MaxVarIntPayload = 9

// readElement reads the next sequence of bytes from r using little endian
// depending on the concrete type of element pointed to.
func readElement(r io.Reader, element interface{}) error {
	var scratch [8]byte

	// Attempt to read the element based on the concrete type via fast
	// type assertions first.
	switch e := element.(type) {
	case *int32:
		b := scratch[0:4]
		_, err := io.ReadFull(r, b)
		if err != nil {
			return err
		}
		*e = int32(binary.BigEndian.Uint32(b))
		return nil

	case *uint32:
		b := scratch[0:4]
		_, err := io.ReadFull(r, b)
		if err != nil {
			return err
		}
		*e = binary.BigEndian.Uint32(b)
		return nil

	case *int64:
		b := scratch[0:8]
		_, err := io.ReadFull(r, b)
		if err != nil {
			return err
		}
		*e = int64(binary.BigEndian.Uint64(b))
		return nil

	case *uint64:
		b := scratch[0:8]
		_, err := io.ReadFull(r, b)
		if err != nil {
			return err
		}
		*e = binary.BigEndian.Uint64(b)
		return nil

	case *bool:
		b := scratch[0:1]
		_, err := io.ReadFull(r, b)
		if err != nil {
			return err
		}
		if b[0] == 0x00 {
			*e = false
		} else {
			*e = true
		}
		return nil

	// Message header checksum.
	case *[4]byte:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	// Message header command.
	case *[CommandSize]uint8:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	// IP address.
	case *[16]byte:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	case *ShaHash:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	case *RipeHash:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	case *PubKey:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	case *ServiceFlag:
		b := scratch[0:8]
		_, err := io.ReadFull(r, b)
		if err != nil {
			return err
		}
		*e = ServiceFlag(binary.BigEndian.Uint64(b))
		return nil

	case *BitmessageNet:
		b := scratch[0:4]
		_, err := io.ReadFull(r, b)
		if err != nil {
			return err
		}
		*e = BitmessageNet(binary.BigEndian.Uint32(b))
		return nil
	}

	// Fall back to the slower binary.Read if a fast path was not available
	// above.
	return binary.Read(r, binary.BigEndian, element)
}

// readElements reads multiple items from r.  It is equivalent to multiple
// calls to readElement.
func readElements(r io.Reader, elements ...interface{}) error {
	for _, element := range elements {
		err := readElement(r, element)
		if err != nil {
			return err
		}
	}
	return nil
}

// writeElement writes the little endian representation of element to w.
func writeElement(w io.Writer, element interface{}) error {
	var scratch [8]byte

	// Attempt to write the element based on the concrete type via fast
	// type assertions first.
	switch e := element.(type) {
	case int32:
		b := scratch[0:4]
		binary.BigEndian.PutUint32(b, uint32(e))
		_, err := w.Write(b)
		if err != nil {
			return err
		}
		return nil

	case uint32:
		b := scratch[0:4]
		binary.BigEndian.PutUint32(b, e)
		_, err := w.Write(b)
		if err != nil {
			return err
		}
		return nil

	case int64:
		b := scratch[0:8]
		binary.BigEndian.PutUint64(b, uint64(e))
		_, err := w.Write(b)
		if err != nil {
			return err
		}
		return nil

	case uint64:
		b := scratch[0:8]
		binary.BigEndian.PutUint64(b, e)
		_, err := w.Write(b)
		if err != nil {
			return err
		}
		return nil

	case bool:
		b := scratch[0:1]
		if e == true {
			b[0] = 0x01
		} else {
			b[0] = 0x00
		}
		_, err := w.Write(b)
		if err != nil {
			return err
		}
		return nil

	// Message header checksum.
	case [4]byte:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	// Message header command.
	case [CommandSize]uint8:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	// IP address.
	case [16]byte:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	case *ShaHash:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	case *RipeHash:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	case *PubKey:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	case ServiceFlag:
		b := scratch[0:8]
		binary.BigEndian.PutUint64(b, uint64(e))
		_, err := w.Write(b)
		if err != nil {
			return err
		}
		return nil

	case BitmessageNet:
		b := scratch[0:4]
		binary.BigEndian.PutUint32(b, uint32(e))
		_, err := w.Write(b)
		if err != nil {
			return err
		}
		return nil
	}

	// Fall back to the slower binary.Write if a fast path was not available
	// above.
	return binary.Write(w, binary.BigEndian, element)
}

// writeElements writes multiple items to w.  It is equivalent to multiple
// calls to writeElement.
func writeElements(w io.Writer, elements ...interface{}) error {
	for _, element := range elements {
		err := writeElement(w, element)
		if err != nil {
			return err
		}
	}
	return nil
}

// readVarInt reads a variable length integer from r and returns it as a uint64.
func readVarInt(r io.Reader) (uint64, error) {
	var b [8]byte
	_, err := io.ReadFull(r, b[0:1])
	if err != nil {
		return 0, err
	}

	var rv uint64
	discriminant := uint8(b[0])
	switch discriminant {
	case 0xff:
		_, err := io.ReadFull(r, b[:])
		if err != nil {
			return 0, err
		}
		rv = binary.BigEndian.Uint64(b[:])

	case 0xfe:
		_, err := io.ReadFull(r, b[0:4])
		if err != nil {
			return 0, err
		}
		rv = uint64(binary.BigEndian.Uint32(b[:]))

	case 0xfd:
		_, err := io.ReadFull(r, b[0:2])
		if err != nil {
			return 0, err
		}
		rv = uint64(binary.BigEndian.Uint16(b[:]))

	default:
		rv = uint64(discriminant)
	}

	return rv, nil
}

// writeVarInt serializes val to w using a variable number of bytes depending
// on its value.
func writeVarInt(w io.Writer, val uint64) error {
	if val < 0xfd {
		_, err := w.Write([]byte{uint8(val)})
		return err
	}

	if val <= math.MaxUint16 {
		var buf [3]byte
		buf[0] = 0xfd
		binary.BigEndian.PutUint16(buf[1:], uint16(val))
		_, err := w.Write(buf[:])
		return err
	}

	if val <= math.MaxUint32 {
		var buf [5]byte
		buf[0] = 0xfe
		binary.BigEndian.PutUint32(buf[1:], uint32(val))
		_, err := w.Write(buf[:])
		return err
	}

	var buf [9]byte
	buf[0] = 0xff
	binary.BigEndian.PutUint64(buf[1:], val)
	_, err := w.Write(buf[:])
	return err
}

// VarIntSerializeSize returns the number of bytes it would take to serialize
// val as a variable length integer.
func VarIntSerializeSize(val uint64) int {
	// The value is small enough to be represented by itself, so it's
	// just 1 byte.
	if val < 0xfd {
		return 1
	}

	// Discriminant 1 byte plus 2 bytes for the uint16.
	if val <= math.MaxUint16 {
		return 3
	}

	// Discriminant 1 byte plus 4 bytes for the uint32.
	if val <= math.MaxUint32 {
		return 5
	}

	// Discriminant 1 byte plus 8 bytes for the uint64.
	return 9
}

// readVarString reads a variable length string from r and returns it as a Go
// string.  A varString is encoded as a varInt containing the length of the
// string, and the bytes that represent the string itself.  An error is returned
// if the length is greater than the maximum block payload size, since it would
// not be possible to put a varString of that size into a block anyways and it
// also helps protect against memory exhaustion attacks and forced panics
// through malformed messages.
func readVarString(r io.Reader) (string, error) {
	count, err := readVarInt(r)
	if err != nil {
		return "", err
	}

	// Prevent variable length strings that are larger than the maximum
	// message size.  It would be possible to cause memory exhaustion and
	// panics without a sane upper bound on this count.
	if count > MaxMessagePayload {
		str := fmt.Sprintf("variable length string is too long "+
			"[count %d, max %d]", count, MaxMessagePayload)
		return "", messageError("readVarString", str)
	}

	buf := make([]byte, count)
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// writeVarString serializes str to w as a varInt containing the length of the
// string followed by the bytes that represent the string itself.
func writeVarString(w io.Writer, str string) error {
	err := writeVarInt(w, uint64(len(str)))
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(str))
	if err != nil {
		return err
	}
	return nil
}

// readVarBytes reads a variable length byte array.  A byte array is encoded
// as a varInt containing the length of the array followed by the bytes
// themselves.  An error is returned if the length is greater than the
// passed maxAllowed parameter which helps protect against memory exhuastion
// attacks and forced panics thorugh malformed messages.  The fieldName
// parameter is only used for the error message so it provides more context in
// the error.
func readVarBytes(r io.Reader, maxAllowed uint32,
	fieldName string) ([]byte, error) {

	count, err := readVarInt(r)
	if err != nil {
		return nil, err
	}

	// Prevent byte array larger than the max message size.  It would
	// be possible to cause memory exhaustion and panics without a sane
	// upper bound on this count.
	if count > uint64(maxAllowed) {
		str := fmt.Sprintf("%s is larger than the max allowed size "+
			"[count %d, max %d]", fieldName, count, maxAllowed)
		return nil, messageError("readVarBytes", str)
	}

	b := make([]byte, count)
	_, err = io.ReadFull(r, b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// writeVarInt serializes a variable length byte array to w as a varInt
// containing the number of bytes, followed by the bytes themselves.
func writeVarBytes(w io.Writer, bytes []byte) error {
	slen := uint64(len(bytes))
	err := writeVarInt(w, slen)
	if err != nil {
		return err
	}

	_, err = w.Write(bytes)
	if err != nil {
		return err
	}
	return nil
}

// randomUint64 returns a cryptographically random uint64 value.  This
// unexported version takes a reader primarily to ensure the error paths
// can be properly tested by passing a fake reader in the tests.
func randomUint64(r io.Reader) (uint64, error) {
	var b [8]byte
	_, err := io.ReadFull(r, b[:])
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b[:]), nil
}

// RandomUint64 returns a cryptographically random uint64 value.
func RandomUint64() (uint64, error) {
	return randomUint64(rand.Reader)
}

// Sha512 returns the sha512 of the bytes
func Sha512(b []byte) []byte {
	t := sha512.Sum512(b)
	sha := make([]byte, 512/8)
	copy(sha, t[:])
	return sha
}

// DoubleSha512 returns the sha512^2 of the bytes
func DoubleSha512(b []byte) []byte {
	return Sha512(Sha512(b))
}
