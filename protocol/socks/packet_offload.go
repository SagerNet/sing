package socks

import (
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type associatePacketOffload struct{}

func (c *AssociatePacketConn) CreatePacketOffload() (N.PacketOffload, bool) {
	return associatePacketOffload{}, true
}

func (c *LazyAssociatePacketConn) CreatePacketOffload() (N.PacketOffload, bool) {
	return nil, false
}

func (o associatePacketOffload) EncodePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	headerLen := 3 + M.SocksaddrSerializer.AddrPortLen(destination)
	header := buf.With(buffer.ExtendHeader(headerLen))
	common.Must(header.WriteZeroN(3))
	return M.SocksaddrSerializer.WriteAddrPort(header, destination)
}

func (o associatePacketOffload) DecodePacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	if buffer.Len() < 3 {
		return M.Socksaddr{}, ErrInvalidPacket
	}
	buffer.Advance(3)
	destination, err := M.SocksaddrSerializer.ReadAddrPort(buffer)
	if err != nil {
		return M.Socksaddr{}, E.Cause1(ErrInvalidPacket, err)
	}
	return destination, nil
}

var (
	_ N.PacketOffloadCreator = (*AssociatePacketConn)(nil)
	_ N.PacketOffloadCreator = (*LazyAssociatePacketConn)(nil)
)
