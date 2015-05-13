package bmpeer

import (
	"net"
)

// Listener represents an open port listening for bitmessage connections.
// It is given as an interface so that mock peer listeners can easily swapped
// for the genuine ones.
type Listener interface {
	Accept() (Connection, error)
	Close() error
	Addr() net.Addr
}

// listener implements the Listener interface. 
type listener struct {
	netListener net.Listener
}

func (pl *listener) Accept() (Connection, error) {
	conn, err := pl.netListener.Accept()
	if err != nil {
		return nil, err
	}
	return &connection {
		conn : conn, 
	}, nil
}

func (pl *listener) Close() error {
	return pl.netListener.Close()
}

// Addr returns the listener's network address.
func (ml *listener) Addr() net.Addr {
	return ml.netListener.Addr()
}

var listen func(service, addr string) (net.Listener, error) = net.Listen

// Listen 
// The value can be swapped out with a mock connection dialer for testing purposes.
func Listen(service, addr string) (Listener, error) {
	netListener, err := listen(service, addr)
	if err != nil {
		return nil, err
	}
	return &listener{
		netListener : netListener, 
	}, nil
}
