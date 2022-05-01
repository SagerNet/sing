package socks

import (
	"net"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
)

type AssociateConn struct {
	net.Conn
	conn net.Conn
	addr net.Addr
	dest *M.AddrPort
}

func NewAssociateConn(conn net.Conn, packetConn net.Conn, destination *M.AddrPort) net.PacketConn {
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
	destination, err := AddressSerializer.ReadAddrPort(reader)
	if err != nil {
		return
	}
	addr = destination.UDPAddr()
	n = copy(p, reader.Bytes())
	return
}

func (c *AssociateConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	_buffer := buf.StackNew()
	buffer := common.Dup(_buffer)
	common.Must(buffer.WriteZeroN(3))
	err = AddressSerializer.WriteAddrPort(buffer, M.AddrPortFromNetAddr(addr))
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
	buffer := common.Dup(_buffer)
	common.Must(buffer.WriteZeroN(3))
	err = AddressSerializer.WriteAddrPort(buffer, c.dest)
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

func (c *AssociateConn) ReadPacket(buffer *buf.Buffer) (*M.AddrPort, error) {
	n, err := buffer.ReadFrom(c.conn)
	if err != nil {
		return nil, err
	}
	buffer.Truncate(int(n))
	buffer.Advance(3)
	return AddressSerializer.ReadAddrPort(buffer)
}

func (c *AssociateConn) WritePacket(buffer *buf.Buffer, destination *M.AddrPort) error {
	defer buffer.Release()
	header := buf.With(buffer.ExtendHeader(3 + AddressSerializer.AddrPortLen(destination)))
	common.Must(header.WriteZeroN(3))
	common.Must(AddressSerializer.WriteAddrPort(header, destination))
	return common.Error(c.Conn.Write(buffer.Bytes()))
}

type AssociatePacketConn struct {
	net.PacketConn
	conn net.Conn
	addr net.Addr
	dest *M.AddrPort
}

func NewAssociatePacketConn(conn net.Conn, packetConn net.PacketConn, destination *M.AddrPort) *AssociatePacketConn {
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
	destination, err := AddressSerializer.ReadAddrPort(reader)
	if err != nil {
		return
	}
	addr = destination.UDPAddr()
	n = copy(p, reader.Bytes())
	return
}

func (c *AssociatePacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	_buffer := buf.StackNew()
	buffer := common.Dup(_buffer)
	common.Must(buffer.WriteZeroN(3))

	err = AddressSerializer.WriteAddrPort(buffer, M.AddrPortFromNetAddr(addr))
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
	buffer := common.Dup(_buffer)
	common.Must(buffer.WriteZeroN(3))

	err = AddressSerializer.WriteAddrPort(buffer, c.dest)
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

func (c *AssociatePacketConn) ReadPacket(buffer *buf.Buffer) (*M.AddrPort, error) {
	n, addr, err := c.PacketConn.ReadFrom(buffer.FreeBytes())
	if err != nil {
		return nil, err
	}
	c.addr = addr
	buffer.Truncate(n)
	buffer.Advance(3)
	dest, err := AddressSerializer.ReadAddrPort(buffer)
	return dest, err
}

func (c *AssociatePacketConn) WritePacket(buffer *buf.Buffer, destination *M.AddrPort) error {
	defer buffer.Release()
	header := buf.With(buffer.ExtendHeader(3 + AddressSerializer.AddrPortLen(destination)))
	common.Must(header.WriteZeroN(3))
	common.Must(AddressSerializer.WriteAddrPort(header, destination))
	return common.Error(c.PacketConn.WriteTo(buffer.Bytes(), c.addr))
}
