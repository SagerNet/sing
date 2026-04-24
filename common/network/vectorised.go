package network

import (
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
)

type VectorisedWriter interface {
	WriteVectorised(buffers []*buf.Buffer) error
}

type VectorisedPacketWriter interface {
	WriteVectorisedPacket(buffers []*buf.Buffer, destination M.Socksaddr) error
}

type PacketBatchWriter interface {
	WritePacketBatch(buffers []*buf.Buffer, destinations []M.Socksaddr) error
}

type PacketBatchWriteCreator interface {
	CreatePacketBatchWriter() (PacketBatchWriter, bool)
}

type ConnectedPacketBatchWriter interface {
	WriteConnectedPacketBatch(buffers []*buf.Buffer) error
}

type ConnectedPacketBatchWriteCreator interface {
	CreateConnectedPacketBatchWriter() (ConnectedPacketBatchWriter, bool)
}
