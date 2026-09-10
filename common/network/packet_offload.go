package network

import (
	"syscall"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
)

type PacketOffload interface {
	EncodePacket(buffer *buf.Buffer, destination M.Socksaddr) error
	DecodePacket(buffer *buf.Buffer) (M.Socksaddr, error)
}

type PacketOffloadCreator interface {
	CreatePacketOffload() (PacketOffload, bool)
}

func UnwrapPacketOffload(conn any) (any, PacketOffload) {
	var offload PacketOffload
	for {
		readerWithUpstream, isReaderWithUpstream := conn.(ReaderWithUpstream)
		writerWithUpstream, isWriterWithUpstream := conn.(WriterWithUpstream)
		replaceable := isReaderWithUpstream && readerWithUpstream.ReaderReplaceable() && isWriterWithUpstream && writerWithUpstream.WriterReplaceable()
		creator, isCreator := conn.(PacketOffloadCreator)
		if replaceable {
			if _, isSyscallConn := conn.(syscall.Conn); isSyscallConn {
				return conn, offload
			}
		} else if !isCreator || offload != nil {
			return conn, offload
		}
		var upstream any
		if withUpstream, hasUpstream := conn.(common.WithUpstream); hasUpstream {
			upstream = withUpstream.Upstream()
		} else {
			upstreamReader, hasUpstreamReader := conn.(WithUpstreamReader)
			upstreamWriter, hasUpstreamWriter := conn.(WithUpstreamWriter)
			if hasUpstreamReader && hasUpstreamWriter {
				upstream = upstreamReader.UpstreamReader()
				if upstream != upstreamWriter.UpstreamWriter() {
					upstream = nil
				}
			}
		}
		if upstream == nil {
			return conn, offload
		}
		if !replaceable {
			created, loaded := creator.CreatePacketOffload()
			if !loaded {
				return conn, offload
			}
			offload = created
		}
		conn = upstream
	}
}
