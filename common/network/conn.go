package network

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
	ReadPacket(buffer *buf.Buffer) (addr M.Socksaddr, err error)
}

type PacketWriter interface {
	WritePacket(buffer *buf.Buffer, addr M.Socksaddr) error
}

type PacketConn interface {
	PacketReader
	PacketWriter

	Close() error
	LocalAddr() net.Addr
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
	if udpConn, ok := dest.(*net.UDPConn); ok {
		return CopyPacketConn(ctx, conn, &UDPConnWrapper{udpConn})
	} else {
		return CopyPacketConn(ctx, conn, &PacketConnWrapper{dest})
	}
}

type UDPConnWrapper struct {
	*net.UDPConn
}

func (w *UDPConnWrapper) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	n, addr, err := w.ReadFromUDPAddrPort(buffer.FreeBytes())
	if err != nil {
		return M.Socksaddr{}, err
	}
	buffer.Truncate(n)
	return M.SocksaddrFromNetIP(addr), nil
}

func (w *UDPConnWrapper) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	if destination.Family().IsFqdn() {
		udpAddr, err := net.ResolveUDPAddr("udp", destination.String())
		if err != nil {
			return err
		}
		return common.Error(w.UDPConn.WriteTo(buffer.Bytes(), udpAddr))
	}
	return common.Error(w.UDPConn.WriteToUDPAddrPort(buffer.Bytes(), destination.AddrPort()))
}

type PacketConnWrapper struct {
	net.PacketConn
}

func (p *PacketConnWrapper) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	_, addr, err := buffer.ReadPacketFrom(p)
	if err != nil {
		return M.Socksaddr{}, err
	}
	return M.SocksaddrFromNet(addr), err
}

func (p *PacketConnWrapper) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	return common.Error(p.WriteTo(buffer.Bytes(), destination.UDPAddr()))
}
