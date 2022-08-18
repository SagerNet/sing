package bufio

import (
	"net"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func Read(reader N.ExtendedReader, buffer *buf.Buffer) (n int, err error) {
	n, err = reader.Read(buffer.FreeBytes())
	buffer.Truncate(n)
	return
}

func ReadFrom(reader N.PacketReader, buffer *buf.Buffer) (n int, addr net.Addr, err error) {
	startLen := buffer.Len()
	addr, err = reader.ReadPacket(buffer)
	n = buffer.Len() - startLen
	return
}

func Write(writer N.ExtendedWriter, buffer *buf.Buffer) (n int, err error) {
	dataLen := buffer.Len()
	err = writer.WriteBuffer(buffer)
	if err == nil {
		n = dataLen
	}
	return
}

func WriteTo(writer N.PacketWriter, buffer *buf.Buffer, addr net.Addr) (n int, err error) {
	dataLen := buffer.Len()
	err = writer.WritePacket(buffer, M.SocksaddrFromNet(addr))
	if err == nil {
		n = dataLen
	}
	return
}

func WriteVectorised(writer N.VectorisedWriter, data [][]byte) (n int, err error) {
	var dataLen int
	buffers := make([]*buf.Buffer, 0, len(data))
	for _, p := range data {
		dataLen += len(p)
		buffers = append(buffers, buf.As(p))
	}
	err = writer.WriteVectorised(buffers)
	if err == nil {
		n = dataLen
	}
	return
}

func WriteVectorisedPacket(writer N.VectorisedPacketWriter, data [][]byte, destination M.Socksaddr) (n int, err error) {
	var dataLen int
	buffers := make([]*buf.Buffer, 0, len(data))
	for _, p := range data {
		dataLen += len(p)
		buffers = append(buffers, buf.As(p))
	}
	err = writer.WriteVectorisedPacket(buffers, destination)
	if err == nil {
		n = dataLen
	}
	return
}
