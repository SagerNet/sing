package network

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
)

type PacketReader interface {
	ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error)
}

type TimeoutPacketReader interface {
	PacketReader
	SetReadDeadline(t time.Time) error
}

type PacketWriter interface {
	WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error
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

type ExtendedReader interface {
	io.Reader
	ReadBuffer(buffer *buf.Buffer) error
}

type ExtendedWriter interface {
	io.Writer
	WriteBuffer(buffer *buf.Buffer) error
}

type ExtendedConn interface {
	ExtendedReader
	ExtendedWriter
	net.Conn
}

type TCPConnectionHandler interface {
	NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error
}

type NetPacketConn interface {
	net.PacketConn
	PacketConn
}

type BindPacketConn interface {
	NetPacketConn
	net.Conn
}

type UDPHandler interface {
	NewPacket(ctx context.Context, conn PacketConn, buffer *buf.Buffer, metadata M.Metadata) error
}

type UDPConnectionHandler interface {
	NewPacketConnection(ctx context.Context, conn PacketConn, metadata M.Metadata) error
}

type CachedReader interface {
	ReadCached() *buf.Buffer
}

type ReaderWithUpstream interface {
	common.WithUpstream
	ReaderReplaceable() bool
}

type WriterWithUpstream interface {
	common.WithUpstream
	WriterReplaceable() bool
}

func UnwrapReader(reader io.Reader) io.Reader {
	if u, ok := reader.(ReaderWithUpstream); ok && u.ReaderReplaceable() {
		return UnwrapReader(u.Upstream().(io.Reader))
	}
	return reader
}

func UnwrapWriter(writer io.Writer) io.Writer {
	if u, ok := writer.(WriterWithUpstream); ok && u.WriterReplaceable() {
		return UnwrapWriter(u.Upstream().(io.Writer))
	}
	return writer
}
