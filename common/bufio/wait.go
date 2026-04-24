package bufio

import (
	"io"

	"github.com/sagernet/sing/common"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func CreateReadWaiter(reader io.Reader) (N.ReadWaiter, bool) {
	if readWaiter, isReadWaiter := reader.(N.ReadWaiter); isReadWaiter {
		return readWaiter, true
	}
	if readWaitCreator, isCreator := reader.(N.ReadWaitCreator); isCreator {
		return readWaitCreator.CreateReadWaiter()
	}
	if readWaiter, created := createSyscallReadWaiter(reader); created {
		return readWaiter, true
	}
	if u, ok := reader.(N.ReaderWithUpstream); !ok || !u.ReaderReplaceable() {
		return nil, false
	}
	if u, ok := reader.(N.WithUpstreamReader); ok {
		return CreateReadWaiter(u.UpstreamReader().(io.Reader))
	}
	if u, ok := reader.(common.WithUpstream); ok {
		return CreateReadWaiter(u.Upstream().(io.Reader))
	}
	return nil, false
}

func CreateVectorisedReadWaiter(reader io.Reader) (N.VectorisedReadWaiter, bool) {
	if vectorisedReadWaiter, isVectorised := reader.(N.VectorisedReadWaiter); isVectorised {
		return vectorisedReadWaiter, true
	}
	if readWaitCreator, isCreator := reader.(N.VectorisedReadWaitCreator); isCreator {
		return readWaitCreator.CreateVectorisedReadWaiter()
	}
	if vectorisedReadWaiter, created := createVectorisedSyscallReadWaiter(reader); created {
		return vectorisedReadWaiter, true
	}
	if u, ok := reader.(N.ReaderWithUpstream); !ok || !u.ReaderReplaceable() {
		return nil, false
	}
	if u, ok := reader.(N.WithUpstreamReader); ok {
		return CreateVectorisedReadWaiter(u.UpstreamReader().(io.Reader))
	}
	if u, ok := reader.(common.WithUpstream); ok {
		return CreateVectorisedReadWaiter(u.Upstream().(io.Reader))
	}
	return nil, false
}

func CreatePacketReadWaiter(reader N.PacketReader) (N.PacketReadWaiter, bool) {
	if readWaiter, isReadWaiter := reader.(N.PacketReadWaiter); isReadWaiter {
		return readWaiter, true
	}
	if readWaitCreator, isCreator := reader.(N.PacketReadWaitCreator); isCreator {
		return readWaitCreator.CreateReadWaiter()
	}
	if readWaiter, created := createSyscallPacketReadWaiter(reader); created {
		return readWaiter, true
	}
	if u, ok := reader.(N.ReaderWithUpstream); !ok || !u.ReaderReplaceable() {
		return nil, false
	}
	if u, ok := reader.(N.WithUpstreamReader); ok {
		return CreatePacketReadWaiter(u.UpstreamReader().(N.PacketReader))
	}
	if u, ok := reader.(common.WithUpstream); ok {
		return CreatePacketReadWaiter(u.Upstream().(N.PacketReader))
	}
	return nil, false
}

func CreatePacketBatchReadWaiter(reader N.PacketReader) (N.PacketBatchReadWaiter, bool) {
	if readWaiter, isReadWaiter := reader.(N.PacketBatchReadWaiter); isReadWaiter {
		return readWaiter, true
	}
	if readWaitCreator, isCreator := reader.(N.PacketBatchReadWaitCreator); isCreator {
		return readWaitCreator.CreatePacketBatchReadWaiter()
	}
	if readWaitCreator, isCreator := reader.(N.VectorisedPacketReadWaitCreator); isCreator {
		return readWaitCreator.CreateVectorisedPacketReadWaiter()
	}
	if readWaiter, created := createSyscallPacketBatchReadWaiter(reader); created {
		return readWaiter, true
	}
	if u, ok := reader.(N.ReaderWithUpstream); !ok || !u.ReaderReplaceable() {
		return nil, false
	}
	if u, ok := reader.(N.WithUpstreamReader); ok {
		return CreatePacketBatchReadWaiter(u.UpstreamReader().(N.PacketReader))
	}
	if u, ok := reader.(common.WithUpstream); ok {
		return CreatePacketBatchReadWaiter(u.Upstream().(N.PacketReader))
	}
	return nil, false
}

func CreateConnectedPacketBatchReadWaiter(reader N.PacketReader) (N.ConnectedPacketBatchReadWaiter, bool) {
	if readWaiter, isReadWaiter := reader.(N.ConnectedPacketBatchReadWaiter); isReadWaiter {
		return readWaiter, true
	}
	if readWaitCreator, isCreator := reader.(N.ConnectedPacketBatchReadWaitCreator); isCreator {
		return readWaitCreator.CreateConnectedPacketBatchReadWaiter()
	}
	if readWaiter, created := createSyscallConnectedPacketBatchReadWaiter(reader, M.Socksaddr{}); created {
		return readWaiter, true
	}
	if u, ok := reader.(N.ReaderWithUpstream); !ok || !u.ReaderReplaceable() {
		return nil, false
	}
	if u, ok := reader.(N.WithUpstreamReader); ok {
		return CreateConnectedPacketBatchReadWaiter(u.UpstreamReader().(N.PacketReader))
	}
	if u, ok := reader.(common.WithUpstream); ok {
		return CreateConnectedPacketBatchReadWaiter(u.Upstream().(N.PacketReader))
	}
	return nil, false
}

// Deprecated: use CreatePacketBatchReadWaiter.
func CreatePacketVectorisedReadWaiter(reader N.PacketReader) (N.VectorisedPacketReadWaiter, bool) {
	return CreatePacketBatchReadWaiter(reader)
}
