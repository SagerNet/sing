package bufio

import (
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

var _ N.ReadWaiter = (*bindPacketReadWaiter)(nil)

type bindPacketReadWaiter struct {
	readWaiter N.PacketReadWaiter
}

func (w *bindPacketReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	return w.readWaiter.InitializeReadWaiter(options)
}

func (w *bindPacketReadWaiter) WaitReadBuffer() (buffer *buf.Buffer, err error) {
	buffer, _, err = w.readWaiter.WaitReadPacket()
	return
}

var _ N.PacketReadWaiter = (*unbindPacketReadWaiter)(nil)

type unbindPacketReadWaiter struct {
	readWaiter N.ReadWaiter
	addr       M.Socksaddr
}

func (w *unbindPacketReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	return w.readWaiter.InitializeReadWaiter(options)
}

func (w *unbindPacketReadWaiter) WaitReadPacket() (buffer *buf.Buffer, destination M.Socksaddr, err error) {
	buffer, err = w.readWaiter.WaitReadBuffer()
	if err != nil {
		return
	}
	destination = w.addr
	return
}

var _ N.ReadWaiter = (*serverPacketReadWaiter)(nil)

type serverPacketReadWaiter struct {
	*serverPacketConn
	readWaiter N.PacketReadWaiter
}

func (w *serverPacketReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	return w.readWaiter.InitializeReadWaiter(options)
}

func (w *serverPacketReadWaiter) WaitReadBuffer() (buffer *buf.Buffer, err error) {
	for {
		var destination M.Socksaddr
		buffer, destination, err = w.readWaiter.WaitReadPacket()
		if err != nil || w.acceptSource(destination) {
			return
		}
		buffer.Release()
	}
}

var _ N.PacketBatchReadWaiter = (*serverPacketBatchReadWaiter)(nil)

type serverPacketBatchReadWaiter struct {
	conn       *serverPacketConn
	readWaiter N.PacketBatchReadWaiter
}

func (w *serverPacketBatchReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	return w.readWaiter.InitializeReadWaiter(options)
}

func (w *serverPacketBatchReadWaiter) WaitReadPackets() ([]*buf.Buffer, []M.Socksaddr, error) {
	for {
		buffers, destinations, err := w.readWaiter.WaitReadPackets()
		if err != nil {
			return nil, nil, err
		}
		var accepted int
		for index, buffer := range buffers {
			if !w.conn.acceptSource(destinations[index]) {
				buffer.Release()
				continue
			}
			buffers[accepted] = buffer
			destinations[accepted] = destinations[index]
			accepted++
		}
		if accepted > 0 {
			return buffers[:accepted], destinations[:accepted], nil
		}
	}
}

func (w *serverPacketBatchReadWaiter) Upstream() any {
	return w.readWaiter
}
