package socks

import (
	"net"
	"runtime"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
)

//+----+------+------+----------+----------+----------+
//|RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
//+----+------+------+----------+----------+----------+
//| 2  |  1   |  1   | Variable |    2     | Variable |
//+----+------+------+----------+----------+----------+

type AssociateConn struct {
	net.Conn
	conn net.Conn
	addr net.Addr
	dest M.Socksaddr
}

func (c AssociateConn) Close() error {
	c.conn.Close()
	c.Conn.Close()
	return nil
}

func NewAssociateConn(conn net.Conn, packetConn net.Conn, destination M.Socksaddr) *AssociateConn {
	return &AssociateConn{
		Conn: packetConn,
		conn: conn,
		dest: destination,
	}
}

func (c *AssociateConn) RemoteAddr() net.Addr {
	return c.addr
}

func (c *AssociateConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, err = c.Conn.Read(p)
	if err != nil {
		return
	}
	reader := buf.As(p[3:n])
	destination, err := M.SocksaddrSerializer.ReadAddrPort(reader)
	if err != nil {
		return
	}
	addr = destination.UDPAddr()
	n = copy(p, reader.Bytes())
	return
}

func (c *AssociateConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	_buffer := buf.StackNew()
	defer runtime.KeepAlive(_buffer)
	buffer := common.Dup(_buffer)
	common.Must(buffer.WriteZeroN(3))
	err = M.SocksaddrSerializer.WriteAddrPort(buffer, M.SocksaddrFromNet(addr))
	if err != nil {
		return
	}
	_, err = buffer.Write(p)
	if err != nil {
		return
	}

	_, err = c.Conn.Write(buffer.Bytes())
	return
}

func (c *AssociateConn) Read(b []byte) (n int, err error) {
	n, _, err = c.ReadFrom(b)
	return
}

func (c *AssociateConn) Write(b []byte) (n int, err error) {
	_buffer := buf.StackNew()
	defer runtime.KeepAlive(_buffer)
	buffer := common.Dup(_buffer)
	common.Must(buffer.WriteZeroN(3))
	err = M.SocksaddrSerializer.WriteAddrPort(buffer, c.dest)
	if err != nil {
		return
	}
	_, err = buffer.Write(b)
	if err != nil {
		return
	}
	_, err = c.Conn.Write(buffer.Bytes())
	return
}

func (c *AssociateConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	n, err := buffer.ReadFrom(c.conn)
	if err != nil {
		return M.Socksaddr{}, err
	}
	buffer.Truncate(int(n))
	buffer.Advance(3)
	return M.SocksaddrSerializer.ReadAddrPort(buffer)
}

func (c *AssociateConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	defer buffer.Release()
	header := buf.With(buffer.ExtendHeader(3 + M.SocksaddrSerializer.AddrPortLen(destination)))
	common.Must(header.WriteZeroN(3))
	common.Must(M.SocksaddrSerializer.WriteAddrPort(header, destination))
	return common.Error(c.Conn.Write(buffer.Bytes()))
}

type AssociatePacketConn struct {
	net.PacketConn
	conn net.Conn
	addr net.Addr
	dest M.Socksaddr
}

func NewAssociatePacketConn(conn net.Conn, packetConn net.PacketConn, destination M.Socksaddr) *AssociatePacketConn {
	return &AssociatePacketConn{
		PacketConn: packetConn,
		conn:       conn,
		dest:       destination,
	}
}

func (c *AssociatePacketConn) RemoteAddr() net.Addr {
	return c.addr
}

func (c *AssociatePacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, addr, err = c.PacketConn.ReadFrom(p)
	if err != nil {
		return
	}
	reader := buf.As(p[3:n])
	destination, err := M.SocksaddrSerializer.ReadAddrPort(reader)
	if err != nil {
		return
	}
	addr = destination.UDPAddr()
	n = copy(p, reader.Bytes())
	return
}

func (c *AssociatePacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	_buffer := buf.StackNew()
	defer runtime.KeepAlive(_buffer)
	buffer := common.Dup(_buffer)
	common.Must(buffer.WriteZeroN(3))

	err = M.SocksaddrSerializer.WriteAddrPort(buffer, M.SocksaddrFromNet(addr))
	if err != nil {
		return
	}
	_, err = buffer.Write(p)
	if err != nil {
		return
	}
	_, err = c.PacketConn.WriteTo(buffer.Bytes(), c.addr)
	return
}

func (c *AssociatePacketConn) Read(b []byte) (n int, err error) {
	n, _, err = c.ReadFrom(b)
	return
}

func (c *AssociatePacketConn) Write(b []byte) (n int, err error) {
	_buffer := buf.StackNew()
	defer runtime.KeepAlive(_buffer)
	buffer := common.Dup(_buffer)
	common.Must(buffer.WriteZeroN(3))

	err = M.SocksaddrSerializer.WriteAddrPort(buffer, c.dest)
	if err != nil {
		return
	}
	_, err = buffer.Write(b)
	if err != nil {
		return
	}
	_, err = c.PacketConn.WriteTo(buffer.Bytes(), c.addr)
	return
}

func (c *AssociatePacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	n, addr, err := c.PacketConn.ReadFrom(buffer.FreeBytes())
	if err != nil {
		return M.Socksaddr{}, err
	}
	c.addr = addr
	buffer.Truncate(n)
	buffer.Advance(3)
	dest, err := M.SocksaddrSerializer.ReadAddrPort(buffer)
	return dest, err
}

func (c *AssociatePacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	defer buffer.Release()
	header := buf.With(buffer.ExtendHeader(3 + M.SocksaddrSerializer.AddrPortLen(destination)))
	common.Must(header.WriteZeroN(3))
	common.Must(M.SocksaddrSerializer.WriteAddrPort(header, destination))
	return common.Error(c.PacketConn.WriteTo(buffer.Bytes(), c.addr))
}
