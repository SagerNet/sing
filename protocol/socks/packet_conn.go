package socks

import (
	"context"
	"net"
	"os"
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
	return os.ErrInvalid
}

func (s *PacketConnStub) SetReadDeadline(t time.Time) error {
	return os.ErrInvalid
}

func (s *PacketConnStub) SetWriteDeadline(t time.Time) error {
	return os.ErrInvalid
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

func CopyNetPacketConn(ctx context.Context, conn PacketConn, dest net.PacketConn) error {
	return CopyPacketConn(ctx, conn, &PacketConnWrapper{dest})
}

type PacketConnWrapper struct {
	net.PacketConn
}

func (p *PacketConnWrapper) ReadPacket(buffer *buf.Buffer) (*M.AddrPort, error) {
	_, addr, err := buffer.ReadPacketFrom(p)
	if err != nil {
		return nil, err
	}
	return M.AddrPortFromNetAddr(addr), err
}

func (p *PacketConnWrapper) WritePacket(buffer *buf.Buffer, destination *M.AddrPort) error {
	return common.Error(p.WriteTo(buffer.Bytes(), destination.UDPAddr()))
}

func (p *PacketConnWrapper) RemoteAddr() net.Addr {
	return &common.DummyAddr{}
}
