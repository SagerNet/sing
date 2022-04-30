package socks

import (
	"context"
	"net"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/common/task"
)

type PacketReader interface {
	ReadPacket(buffer *buf.Buffer) (*M.AddrPort, error)
}

type PacketWriter interface {
	WritePacket(buffer *buf.Buffer, destination *M.AddrPort) error
}

type PacketConn interface {
	PacketReader
	PacketWriter

	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

type UDPHandler interface {
	NewPacket(conn PacketConn, buffer *buf.Buffer, metadata M.Metadata) error
}

type UDPConnectionHandler interface {
	NewPacketConnection(ctx context.Context, conn PacketConn, metadata M.Metadata) error
}

type PacketConnStub struct{}

func (s *PacketConnStub) RemoteAddr() net.Addr {
	return &common.DummyAddr{}
}

func (s *PacketConnStub) SetDeadline(t time.Time) error {
	return nil
}

func (s *PacketConnStub) SetReadDeadline(t time.Time) error {
	return nil
}

func (s *PacketConnStub) SetWriteDeadline(t time.Time) error {
	return nil
}

func CopyPacketConn(ctx context.Context, dest PacketConn, conn PacketConn) error {
	return task.Run(ctx, func() error {
		defer rw.CloseRead(conn)
		defer rw.CloseWrite(dest)
		_buffer := buf.StackNewMax()
		buffer := common.Dup(_buffer)
		data := buffer.Cut(buf.ReversedHeader, buf.ReversedHeader)
		for {
			data.FullReset()
			destination, err := conn.ReadPacket(data)
			if err != nil {
				return err
			}
			buffer.Resize(buf.ReversedHeader+data.Start(), data.Len())
			err = dest.WritePacket(buffer, destination)
			if err != nil {
				return err
			}
		}
	}, func() error {
		defer rw.CloseRead(dest)
		defer rw.CloseWrite(conn)
		_buffer := buf.StackNewMax()
		buffer := common.Dup(_buffer)
		data := buffer.Cut(buf.ReversedHeader, buf.ReversedHeader)
		for {
			data.FullReset()
			destination, err := dest.ReadPacket(data)
			if err != nil {
				return err
			}
			buffer.Resize(buf.ReversedHeader+data.Start(), data.Len())
			err = conn.WritePacket(buffer, destination)
			if err != nil {
				return err
			}
		}
	})
}

func CopyNetPacketConn(ctx context.Context, dest net.PacketConn, conn PacketConn) error {
	return task.Run(ctx, func() error {
		defer rw.CloseRead(conn)
		defer rw.CloseWrite(dest)

		_buffer := buf.StackNew()
		buffer := common.Dup(_buffer)
		for {
			buffer.FullReset()
			destination, err := conn.ReadPacket(buffer)
			if err != nil {
				return err
			}

			_, err = dest.WriteTo(buffer.Bytes(), destination.UDPAddr())
			if err != nil {
				return err
			}
		}
	}, func() error {
		defer rw.CloseRead(dest)
		defer rw.CloseWrite(conn)

		_buffer := buf.StackNew()
		buffer := common.Dup(_buffer)
		data := buffer.Cut(buf.ReversedHeader, buf.ReversedHeader)
		for {
			data.FullReset()
			n, addr, err := dest.ReadFrom(data.FreeBytes())
			if err != nil {
				return err
			}
			buffer.Resize(buf.ReversedHeader, n)
			err = conn.WritePacket(buffer, M.AddrPortFromNetAddr(addr))
			if err != nil {
				return err
			}
		}
	})
}

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
