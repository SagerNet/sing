package socks

import (
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

var _ N.PacketReadWaitCreator = (*AssociatePacketConn)(nil)

func (c *AssociatePacketConn) CreateReadWaiter() (N.PacketReadWaiter, bool) {
	readWaiter, isReadWaiter := bufio.CreatePacketReadWaiter(c.NetPacketConn)
	if !isReadWaiter {
		return nil, false
	}
	return &AssociatePacketReadWaiter{c, readWaiter}, true
}

var _ N.PacketReadWaiter = (*AssociatePacketReadWaiter)(nil)

type AssociatePacketReadWaiter struct {
	conn       *AssociatePacketConn
	readWaiter N.PacketReadWaiter
}

func (w *AssociatePacketReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	return w.readWaiter.InitializeReadWaiter(options)
}

func (w *AssociatePacketReadWaiter) WaitReadPacket() (buffer *buf.Buffer, destination M.Socksaddr, err error) {
	buffer, destination, err = w.readWaiter.WaitReadPacket()
	if err != nil {
		return
	}
	if buffer.Len() < 3 {
		buffer.Release()
		return nil, M.Socksaddr{}, ErrInvalidPacket
	}
	w.conn.remoteAddr = destination
	buffer.Advance(3)
	destination, err = M.SocksaddrSerializer.ReadAddrPort(buffer)
	if err != nil {
		buffer.Release()
		return nil, M.Socksaddr{}, err
	}
	return
}
