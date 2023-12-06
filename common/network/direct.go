package network

import (
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
)

type ReadWaiter interface {
	InitializeReadWaiter(newBuffer func() *buf.Buffer)
	WaitReadBuffer() (buffer *buf.Buffer, err error)
}

type ReadWaitCreator interface {
	CreateReadWaiter() (ReadWaiter, bool)
}

type PacketReadWaiter interface {
	InitializeReadWaiter(newBuffer func() *buf.Buffer)
	WaitReadPacket() (buffer *buf.Buffer, destination M.Socksaddr, err error)
}

type PacketReadWaitCreator interface {
	CreateReadWaiter() (PacketReadWaiter, bool)
}
