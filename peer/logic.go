// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

import (
	"github.com/monetas/bmutil/wire"
)

// Logic is an interface that represents the behavior of a peer object
// excluding the parts that must be continually running.
type Logic interface {
	ProtocolVersion() uint32
	Stop()

	HandleVersionMsg(*wire.MsgVersion) error
	HandleVerAckMsg() error
	HandleAddrMsg(*wire.MsgAddr) error
	HandleInvMsg(*wire.MsgInv) error
	HandleGetDataMsg(*wire.MsgGetData) error
	HandleObjectMsg(wire.Message) error

	PushVersionMsg()
	PushVerAckMsg()
	PushAddrMsg(addresses []*wire.NetAddress) error
	PushInvMsg(invVect []*wire.InvVect)
	PushGetDataMsg(invVect []*wire.InvVect)
	PushObjectMsg(sha *wire.ShaHash)
}
