package socks

import (
	"context"
	"github.com/sagernet/sing/common/task"
	"net"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
)

type PacketConn interface {
	ReadPacket(buffer *buf.Buffer) (*M.AddrPort, error)
	WritePacket(buffer *buf.Buffer, addrPort *M.AddrPort) error

	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

type UDPConnectionHandler interface {
	NewPacketConnection(conn PacketConn, metadata M.Metadata) error
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
		_buffer := buf.StackNew()
		buffer := common.Dup(_buffer)
		for {
			destination, err := conn.ReadPacket(buffer)
			if err != nil {
				return err
			}
			err = dest.WritePacket(buffer, destination)
			if err != nil {
				return err
			}
		}
	}, func() error {
		_buffer := buf.StackNew()
		buffer := common.Dup(_buffer)
		for {
			destination, err := dest.ReadPacket(buffer)
			if err != nil {
				return err
			}
			err = conn.WritePacket(buffer, destination)
			if err != nil {
				return err
			}
		}
	})
}

func CopyPacketConn0(dest PacketConn, conn PacketConn, onAction func(destination *M.AddrPort, n int)) error {
	for {
		buffer := buf.New()
		destination, err := conn.ReadPacket(buffer)
		if err != nil {
			buffer.Release()
			return err
		}
		size := buffer.Len()
		err = dest.WritePacket(buffer, destination)
		if err != nil {
			buffer.Release()
			return err
		}
		if onAction != nil {
			onAction(destination, size)
		}
	}
}

type associatePacketConn struct {
	net.PacketConn
	conn net.Conn
	addr net.Addr
}

func NewPacketConn(conn net.Conn, packetConn net.PacketConn) PacketConn {
	return &associatePacketConn{
		PacketConn: packetConn,
		conn:       conn,
	}
}

func (c *associatePacketConn) RemoteAddr() net.Addr {
	return c.addr
}

func (c *associatePacketConn) ReadPacket(buffer *buf.Buffer) (*M.AddrPort, error) {
	n, addr, err := c.PacketConn.ReadFrom(buffer.FreeBytes())
	if err != nil {
		return nil, err
	}
	c.addr = addr
	buffer.Truncate(n)
	buffer.Advance(3)
	return AddressSerializer.ReadAddrPort(buffer)
}

func (c *associatePacketConn) WritePacket(buffer *buf.Buffer, addrPort *M.AddrPort) error {
	defer buffer.Release()
	header := buf.New()
	common.Must(header.WriteZeroN(3))
	common.Must(AddressSerializer.WriteAddrPort(header, addrPort))
	buffer = buffer.WriteBufferAtFirst(header)
	return common.Error(c.PacketConn.WriteTo(buffer.Bytes(), c.addr))
}
