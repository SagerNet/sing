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

type AbstractConn interface {
	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

type PacketReader interface {
	ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error)
}

type TimeoutPacketReader interface {
	PacketReader
	SetReadDeadline(t time.Time) error
}

type NetPacketReader interface {
	PacketReader
	ReadFrom(p []byte) (n int, addr net.Addr, err error)
}

type NetPacketWriter interface {
	PacketWriter
	WriteTo(p []byte, addr net.Addr) (n int, err error)
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
	PacketConn
	NetPacketReader
	NetPacketWriter
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

type CachedPacketReader interface {
	ReadCachedPacket() *PacketBuffer
}

type PacketBuffer struct {
	Buffer      *buf.Buffer
	Destination M.Socksaddr
}

type WithUpstreamReader interface {
	UpstreamReader() any
}

type WithUpstreamWriter interface {
	UpstreamWriter() any
}

type ReaderWithUpstream interface {
	ReaderReplaceable() bool
}

type WriterWithUpstream interface {
	WriterReplaceable() bool
}

type ReaderPossiblyReplaceable interface {
	ReaderWithUpstream
	ReaderPossiblyReplaceable() bool
}

type WriterPossiblyReplaceable interface {
	WriterWithUpstream
	WriterPossiblyReplaceable() bool
}

func UnwrapReader(reader io.Reader) io.Reader {
	if u, ok := reader.(ReaderWithUpstream); !ok || !u.ReaderReplaceable() {
		return reader
	}
	if u, ok := reader.(WithUpstreamReader); ok {
		return UnwrapReader(u.UpstreamReader().(io.Reader))
	}
	if u, ok := reader.(common.WithUpstream); ok {
		return UnwrapReader(u.Upstream().(io.Reader))
	}
	return reader
}

func CastReader[T io.Reader](reader io.Reader) (T, bool) {
	if c, ok := reader.(T); ok {
		return c, true
	}
	if u, ok := reader.(ReaderWithUpstream); !ok || !u.ReaderReplaceable() {
		return common.DefaultValue[T](), false
	}
	if u, ok := reader.(WithUpstreamReader); ok {
		return CastReader[T](u.UpstreamReader().(io.Reader))
	}
	if u, ok := reader.(common.WithUpstream); ok {
		return CastReader[T](u.Upstream().(io.Reader))
	}
	return common.DefaultValue[T](), false
}

func UnwrapPacketReader(reader PacketReader) PacketReader {
	if u, ok := reader.(ReaderWithUpstream); !ok || !u.ReaderReplaceable() {
		return reader
	}
	if u, ok := reader.(WithUpstreamReader); ok {
		return UnwrapPacketReader(u.UpstreamReader().(PacketReader))
	}
	if u, ok := reader.(common.WithUpstream); ok {
		return UnwrapPacketReader(u.Upstream().(PacketReader))
	}
	return reader
}

func CastPacketReader[T PacketReader](reader PacketReader) (T, bool) {
	if c, ok := reader.(T); ok {
		return c, true
	}
	if u, ok := reader.(ReaderWithUpstream); !ok || !u.ReaderReplaceable() {
		return common.DefaultValue[T](), false
	}
	if u, ok := reader.(WithUpstreamReader); ok {
		return CastPacketReader[T](u.UpstreamReader().(PacketReader))
	}
	if u, ok := reader.(common.WithUpstream); ok {
		return CastPacketReader[T](u.Upstream().(PacketReader))
	}
	return common.DefaultValue[T](), false
}

func UnwrapWriter(writer io.Writer) io.Writer {
	if u, ok := writer.(WriterWithUpstream); !ok || !u.WriterReplaceable() {
		return writer
	}
	if u, ok := writer.(WithUpstreamWriter); ok {
		return UnwrapWriter(u.UpstreamWriter().(io.Writer))
	}
	if u, ok := writer.(common.WithUpstream); ok {
		return UnwrapWriter(u.Upstream().(io.Writer))
	}
	return writer
}

func CastWriter[T io.Writer](writer io.Writer) (T, bool) {
	if c, ok := writer.(T); ok {
		return c, true
	}
	if u, ok := writer.(WriterWithUpstream); !ok || !u.WriterReplaceable() {
		return common.DefaultValue[T](), false
	}
	if u, ok := writer.(WithUpstreamWriter); ok {
		return CastWriter[T](u.UpstreamWriter().(io.Writer))
	}
	if u, ok := writer.(common.WithUpstream); ok {
		return CastWriter[T](u.Upstream().(io.Writer))
	}
	return common.DefaultValue[T](), false
}

func UnwrapPacketWriter(writer PacketWriter) PacketWriter {
	if u, ok := writer.(WriterWithUpstream); !ok || !u.WriterReplaceable() {
		return writer
	}
	if u, ok := writer.(WithUpstreamWriter); ok {
		return UnwrapPacketWriter(u.UpstreamWriter().(PacketWriter))
	}
	if u, ok := writer.(common.WithUpstream); ok {
		return UnwrapPacketWriter(u.Upstream().(PacketWriter))
	}
	return writer
}

func CastPacketWriter[T PacketWriter](writer PacketWriter) (T, bool) {
	if c, ok := writer.(T); ok {
		return c, true
	}
	if u, ok := writer.(WriterWithUpstream); !ok || !u.WriterReplaceable() {
		return common.DefaultValue[T](), false
	}
	if u, ok := writer.(WithUpstreamWriter); ok {
		return CastPacketWriter[T](u.UpstreamWriter().(PacketWriter))
	}
	if u, ok := writer.(common.WithUpstream); ok {
		return CastPacketWriter[T](u.Upstream().(PacketWriter))
	}
	return common.DefaultValue[T](), false
}
