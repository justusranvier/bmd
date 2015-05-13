package bmpeer

import (
	"net"
)

// TstNewConnection is used to create a new connection with a mock conn instead 
// of a real one for testing purposes. 
func TstNewConnection(conn net.Conn) Connection {
	return &connection {
		conn : conn, 
	}
}

// TstNewListneter returns a new listener with a user defined net.Listener, which
// can be a mock object for testing purposes. 
func TstNewListener(netListen net.Listener) Listener {
	return &listener{
		netListener : netListen, 
	}
}

// SwapDialDial swaps out the dialConnection function to mock it for testing
// purposes. It returns the original function so that it can be swapped back in
// at the end of the test.
func TstSwapDial(f func(string, string) (net.Conn, error)) func(string, string) (net.Conn, error) {
	g := dial
	dial = f
	return g
}

func TstSwapListen(f func(string, string) (net.Listener, error)) func(string, string) (net.Listener, error) {
	g := listen
	listen = f
	return g
}