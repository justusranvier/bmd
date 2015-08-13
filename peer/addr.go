// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

import (
	"github.com/monetas/bmd/addrmgr"
	"github.com/monetas/bmutil/wire"
)

// Addr is an address of a peer. It's distinct from net.TCPAddr so that Tor
// addresses can be correctly represented as strings using addrmgr.NetAddressKey.
type Addr wire.NetAddress

// Network returns the address's network name.
func (a *Addr) Network() string {
	return "tcp"
}

// String returns a string representation of the address.
func (a *Addr) String() string {
	return addrmgr.NetAddressKey((*wire.NetAddress)(a))
}
