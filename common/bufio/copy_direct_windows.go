package bufio

import (
	N "github.com/sagernet/sing/common/network"
)

func createSyscallReadWaiter(reader any) (N.ReadWaiter, bool) {
	return nil, false
}

func createSyscallPacketReadWaiter(reader any) (N.PacketReadWaiter, bool) {
	return nil, false
}
