package socks

import (
	"net"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/socksaddr"
)

type PacketConn interface {
	ReadPacket(buffer *buf.Buffer) (socksaddr.Addr, uint16, error)
	WritePacket(buffer *buf.Buffer, addr socksaddr.Addr, port uint16) error

	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

func CopyPacketConn(dest PacketConn, conn PacketConn, onAction func(size int)) error {
	for {
		buffer := buf.New()
		addr, port, err := conn.ReadPacket(buffer)
		if err != nil {
			buffer.Release()
			return err
		}
		size := buffer.Len()
		err = dest.WritePacket(buffer, addr, port)
		if err != nil {
			return err
		}
		if onAction != nil {
			onAction(size)
		}
		buffer.Reset()
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

func (c *associatePacketConn) ReadPacket(buffer *buf.Buffer) (socksaddr.Addr, uint16, error) {
	n, addr, err := c.PacketConn.ReadFrom(buffer.FreeBytes())
	if err != nil {
		return nil, 0, err
	}
	c.addr = addr
	buffer.Truncate(n)
	buffer.Advance(3)
	return AddressSerializer.ReadAddressAndPort(buffer)
}

func (c *associatePacketConn) WritePacket(buffer *buf.Buffer, addr socksaddr.Addr, port uint16) error {
	defer buffer.Release()
	header := buf.New()
	common.Must(header.WriteZeroN(3))
	common.Must(AddressSerializer.WriteAddressAndPort(header, addr, port))
	buffer = buffer.WriteBufferAtFirst(header)
	return common.Error(c.PacketConn.WriteTo(buffer.Bytes(), c.addr))
}
