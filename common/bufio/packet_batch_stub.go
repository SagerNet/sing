//go:build !linux && !netbsd && !darwin

package bufio

import (
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func createSyscallPacketBatchReadWaiter(reader any) (N.PacketBatchReadWaiter, bool) {
	return nil, false
}

func createSyscallPacketBatchWriter(writer any) (N.PacketBatchWriter, bool) {
	return nil, false
}

func createSyscallConnectedPacketBatchReadWaiter(reader any, destination M.Socksaddr) (N.ConnectedPacketBatchReadWaiter, bool) {
	return nil, false
}

func createSyscallConnectedPacketBatchWriter(writer any) (N.ConnectedPacketBatchWriter, bool) {
	return nil, false
}
