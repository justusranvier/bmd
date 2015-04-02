package wire

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"time"
)

// ErrInvalidNetAddr describes an error that indicates the caller didn't specify
// a TCP address as required.
var ErrInvalidNetAddr = errors.New("provided net.Addr is not a net.TCPAddr")

// maxNetAddressPayload returns the max payload size for a bitmessage NetAddress
// based on the protocol version.
func maxNetAddressPayload() int {
	// Time 8 bytes + stream 4 bytes + services 8 bytes + IP 16 bytes + port 2
	// bytes.
	return 8 + 4 + 8 + 16 + 2
}

// NetAddress defines information about a peer on the network including the time
// it was last seen, the services it supports, its IP address, and port.
type NetAddress struct {
	// Last time the address was seen.
	Timestamp time.Time

	// Stream that the address is a member of.
	Stream uint32

	// Bitfield which identifies the services supported by the address.
	Services ServiceFlag

	// IP address of the peer.
	IP net.IP

	// Port the peer is using.  This is encoded in big endian on the wire
	// which differs from most everything else.
	Port uint16
}

// HasService returns whether the specified service is supported by the address.
func (na *NetAddress) HasService(service ServiceFlag) bool {
	if na.Services&service == service {
		return true
	}
	return false
}

// AddService adds service as a supported service by the peer generating the
// message.
func (na *NetAddress) AddService(service ServiceFlag) {
	na.Services |= service
}

// SetAddress is a convenience function to set the IP address and port in one
// call.
func (na *NetAddress) SetAddress(ip net.IP, port uint16) {
	na.IP = ip
	na.Port = port
}

// NewNetAddressIPPort returns a new NetAddress using the provided IP, port,
// stream and supported services with Timestamp being time.Now().
func NewNetAddressIPPort(ip net.IP, port uint16, stream uint32, services ServiceFlag) *NetAddress {
	// Limit the timestamp to one second precision since the protocol
	// doesn't support better.
	na := NetAddress{
		Timestamp: time.Unix(time.Now().Unix(), 0),
		Stream:    stream,
		Services:  services,
		IP:        ip,
		Port:      port,
	}
	return &na
}

// NewNetAddress returns a new NetAddress using the provided TCP address and
// supported services with defaults for the remaining fields.
//
// Note that addr must be a net.TCPAddr.  An ErrInvalidNetAddr is returned
// if it is not.
func NewNetAddress(addr net.Addr, stream uint32, services ServiceFlag) (*NetAddress, error) {
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return nil, ErrInvalidNetAddr
	}

	na := NewNetAddressIPPort(tcpAddr.IP, uint16(tcpAddr.Port), stream, services)
	return na, nil
}

// readNetAddress reads an encoded NetAddress from r depending on the protocol
// version and whether or not the timestamp and stream are included as per big.
// Some messages like version do not include them.
func readNetAddress(r io.Reader, na *NetAddress, big bool) error {
	var timestamp time.Time
	var stream uint32
	var services ServiceFlag
	var ip [16]byte
	var port uint16

	if big {
		var stamp uint64
		err := readElements(r, &stamp, &stream)
		if err != nil {
			return err
		}
		timestamp = time.Unix(int64(stamp), 0)
	}

	err := readElements(r, &services, &ip)
	if err != nil {
		return err
	}
	err = binary.Read(r, binary.BigEndian, &port)
	if err != nil {
		return err
	}

	na.Timestamp = timestamp
	na.Stream = stream
	na.Services = services
	na.SetAddress(net.IP(ip[:]), port)
	return nil
}

// writeNetAddress serializes a NetAddress to w depending on the protocol
// version and whether or not the timestamp and stream are included as per big.
// Some messages like version do not include them.
func writeNetAddress(w io.Writer, na *NetAddress, big bool) error {
	if big {
		err := writeElements(w, uint64(na.Timestamp.Unix()), uint32(na.Stream))
		if err != nil {
			return err
		}
	}

	// Ensure to always write 16 bytes even if the ip is nil.
	var ip [16]byte
	if na.IP != nil {
		copy(ip[:], na.IP.To16())
	}
	err := writeElements(w, na.Services, ip)
	if err != nil {
		return err
	}

	err = binary.Write(w, binary.BigEndian, na.Port)
	if err != nil {
		return err
	}

	return nil
}
