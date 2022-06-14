package socks

import (
	"net"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

//+----+------+------+----------+----------+----------+
//|RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
//+----+------+------+----------+----------+----------+
//| 2  |  1   |  1   | Variable |    2     | Variable |
//+----+------+------+----------+----------+----------+

type AssociatePacketConn struct {
	N.PacketConn
	addr       net.Addr
	remoteAddr M.Socksaddr
	underlying net.Conn
}

func NewAssociatePacketConn(conn net.PacketConn, remoteAddr M.Socksaddr, underlying net.Conn) *AssociatePacketConn {
	return &AssociatePacketConn{
		PacketConn: bufio.NewPacketConn(conn),
		remoteAddr: remoteAddr,
		underlying: underlying,
	}
}

func NewAssociateConn(conn net.Conn, remoteAddr M.Socksaddr, underlying net.Conn) *AssociatePacketConn {
	return &AssociatePacketConn{
		PacketConn: bufio.NewUnbindPacketConn(conn),
		remoteAddr: remoteAddr,
		underlying: underlying,
	}
}

func (c *AssociatePacketConn) RemoteAddr() net.Addr {
	return c.addr
}

func (c *AssociatePacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	buffer := buf.With(p)
	n, _, err = bufio.ReadFrom(c.PacketConn, buffer)
	if err != nil {
		return
	}
	buffer.Advance(3)
	destination, err := M.SocksaddrSerializer.ReadAddrPort(buffer)
	if err != nil {
		return
	}
	addr = destination.UDPAddr()
	n = copy(p, buffer.Bytes())
	return
}

func (c *AssociatePacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	destination := M.SocksaddrFromNet(addr)
	_buffer := buf.StackNewSize(3 + M.SocksaddrSerializer.AddrPortLen(destination) + len(p))
	defer common.KeepAlive(_buffer)
	buffer := common.Dup(_buffer)
	defer buffer.Release()
	common.Must(buffer.WriteZeroN(3))
	err = M.SocksaddrSerializer.WriteAddrPort(buffer, destination)
	if err != nil {
		return
	}
	_, err = buffer.Write(p)
	if err != nil {
		return
	}
	return bufio.WriteTo(c.PacketConn, buffer, c.addr)
}

func (c *AssociatePacketConn) Read(b []byte) (n int, err error) {
	n, _, err = c.ReadFrom(b)
	return
}

func (c *AssociatePacketConn) Write(b []byte) (n int, err error) {
	return c.WriteTo(b, c.addr)
}

func (c *AssociatePacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	_, addr, err := bufio.ReadFrom(c.PacketConn, buffer)
	if err != nil {
		return M.Socksaddr{}, err
	}
	c.addr = addr
	buffer.Advance(3)
	dest, err := M.SocksaddrSerializer.ReadAddrPort(buffer)
	return dest, err
}

func (c *AssociatePacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	defer buffer.Release()
	header := buf.With(buffer.ExtendHeader(3 + M.SocksaddrSerializer.AddrPortLen(destination)))
	common.Must(header.WriteZeroN(3))
	common.Must(M.SocksaddrSerializer.WriteAddrPort(header, destination))
	return common.Error(bufio.WriteTo(c.PacketConn, buffer, c.addr))
}

func (c *AssociatePacketConn) Upstream() any {
	return c.PacketConn
}

func (c *AssociatePacketConn) Close() error {
	return common.Close(
		c.PacketConn,
		c.underlying,
	)
}
