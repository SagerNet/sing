package bufio

import (
	"net"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type BindPacketConn interface {
	N.NetPacketConn
	net.Conn
}

type bindPacketConn struct {
	N.NetPacketConn
	addr net.Addr
}

func NewBindPacketConn(conn net.PacketConn, addr net.Addr) BindPacketConn {
	return &bindPacketConn{
		NewPacketConn(conn),
		addr,
	}
}

func (c *bindPacketConn) Read(b []byte) (n int, err error) {
	n, _, err = c.ReadFrom(b)
	return
}

func (c *bindPacketConn) Write(b []byte) (n int, err error) {
	return c.WriteTo(b, c.addr)
}

func (c *bindPacketConn) CreateReadWaiter() (N.ReadWaiter, bool) {
	readWaiter, isReadWaiter := CreatePacketReadWaiter(c.NetPacketConn)
	if !isReadWaiter {
		return nil, false
	}
	return &BindPacketReadWaiter{readWaiter}, true
}

func (c *bindPacketConn) RemoteAddr() net.Addr {
	return c.addr
}

func (c *bindPacketConn) Upstream() any {
	return c.NetPacketConn
}

var (
	_ N.NetPacketConn         = (*UnbindPacketConn)(nil)
	_ N.PacketReadWaitCreator = (*UnbindPacketConn)(nil)
)

type UnbindPacketConn struct {
	N.ExtendedConn
	addr M.Socksaddr
}

func NewUnbindPacketConn(conn net.Conn) N.NetPacketConn {
	return &UnbindPacketConn{
		NewExtendedConn(conn),
		M.SocksaddrFromNet(conn.RemoteAddr()),
	}
}

func NewUnbindPacketConnWithAddr(conn net.Conn, addr M.Socksaddr) N.NetPacketConn {
	return &UnbindPacketConn{
		NewExtendedConn(conn),
		addr,
	}
}

func (c *UnbindPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, err = c.ExtendedConn.Read(p)
	if err == nil {
		addr = c.addr.UDPAddr()
	}
	return
}

func (c *UnbindPacketConn) WriteTo(p []byte, _ net.Addr) (n int, err error) {
	return c.ExtendedConn.Write(p)
}

func (c *UnbindPacketConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	err = c.ExtendedConn.ReadBuffer(buffer)
	if err != nil {
		return
	}
	destination = c.addr
	return
}

func (c *UnbindPacketConn) WritePacket(buffer *buf.Buffer, _ M.Socksaddr) error {
	return c.ExtendedConn.WriteBuffer(buffer)
}

func (c *UnbindPacketConn) CreateReadWaiter() (N.PacketReadWaiter, bool) {
	readWaiter, isReadWaiter := CreateReadWaiter(c.ExtendedConn)
	if !isReadWaiter {
		return nil, false
	}
	return &UnbindPacketReadWaiter{readWaiter, c.addr}, true
}

func (c *UnbindPacketConn) Upstream() any {
	return c.ExtendedConn
}
