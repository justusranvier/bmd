package bmpeer

import (
	"github.com/monetas/bmutil/wire"
)

type PeerState uint32
const (
	PeerStateNew                 PeerState = iota
	PeerStateVersionKnown        PeerState = iota
	PeerStateHandshakeComplete   PeerState = iota
)

type Logic interface {
	State() PeerState
	ProtocolVersion() uint32

	HandleVersionMsg(*wire.MsgVersion) error
	HandleVerAckMsg() error
	HandleAddrMsg(*wire.MsgAddr) error
	HandleInvMsg(*wire.MsgInv) error
	HandleGetDataMsg(*wire.MsgGetData) error
	HandleObjectMsg(wire.Message) error

	PushVersionMsg() error
	PushVerAckMsg() error
	PushAddrMsg(addresses []*wire.NetAddress)
	PushInvMsg(invVect []*wire.InvVect)
	PushGetDataMsg(invVect []*wire.InvVect)
	PushObjectMsg(sha *wire.ShaHash)
}

