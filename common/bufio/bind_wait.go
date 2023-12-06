package bufio

import (
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

var _ N.ReadWaiter = (*BindPacketReadWaiter)(nil)

type BindPacketReadWaiter struct {
	readWaiter N.PacketReadWaiter
}

func (w *BindPacketReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	return w.readWaiter.InitializeReadWaiter(options)
}

func (w *BindPacketReadWaiter) WaitReadBuffer() (buffer *buf.Buffer, err error) {
	buffer, _, err = w.readWaiter.WaitReadPacket()
	return
}

var _ N.PacketReadWaiter = (*UnbindPacketReadWaiter)(nil)

type UnbindPacketReadWaiter struct {
	readWaiter N.ReadWaiter
	addr       M.Socksaddr
}

func (w *UnbindPacketReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	return w.readWaiter.InitializeReadWaiter(options)
}

func (w *UnbindPacketReadWaiter) WaitReadPacket() (buffer *buf.Buffer, destination M.Socksaddr, err error) {
	buffer, err = w.readWaiter.WaitReadBuffer()
	if err != nil {
		return
	}
	destination = w.addr
	return
}
