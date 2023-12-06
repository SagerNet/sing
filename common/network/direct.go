package network

import (
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
)

type ReadWaitable interface {
	InitializeReadWaiter(options ReadWaitOptions) (needCopy bool)
}

type ReadWaitOptions struct {
	FrontHeadroom int
	RearHeadroom  int
	MTU           int
}

func (o ReadWaitOptions) NeedHeadroom() bool {
	return o.FrontHeadroom > 0 || o.RearHeadroom > 0
}

func (o ReadWaitOptions) NewBuffer() (buffer *buf.Buffer, readBuffer *buf.Buffer) {
	return o.newBuffer(buf.BufferSize)
}

func (o ReadWaitOptions) NewPacketBuffer() (buffer *buf.Buffer, readBuffer *buf.Buffer) {
	return o.newBuffer(buf.UDPBufferSize)
}

func (o ReadWaitOptions) newBuffer(defaultBufferSize int) (buffer *buf.Buffer, readBuffer *buf.Buffer) {
	var bufferSize int
	if o.MTU > 0 {
		bufferSize = o.MTU + o.FrontHeadroom + o.RearHeadroom
	} else {
		bufferSize = defaultBufferSize
	}
	buffer = buf.NewSize(bufferSize)
	if o.RearHeadroom > 0 {
		readBufferRaw := buffer.Slice()
		readBuffer = buf.With(readBufferRaw[:len(readBufferRaw)-o.RearHeadroom])
	} else {
		readBuffer = buffer
	}
	readBuffer.Resize(o.FrontHeadroom, 0)
	return
}

type ReadWaiter interface {
	ReadWaitable
	WaitReadBuffer() (buffer *buf.Buffer, err error)
}

type ReadWaitCreator interface {
	CreateReadWaiter() (ReadWaiter, bool)
}

type PacketReadWaiter interface {
	ReadWaitable
	WaitReadPacket() (buffer *buf.Buffer, destination M.Socksaddr, err error)
}

type PacketReadWaitCreator interface {
	CreateReadWaiter() (PacketReadWaiter, bool)
}
