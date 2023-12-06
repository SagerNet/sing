package udpnat

import (
	"io"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

var _ N.PacketReadWaiter = (*conn)(nil)

func (c *conn) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	c.readWaitOptions = options
	return false
}

func (c *conn) WaitReadPacket() (buffer *buf.Buffer, destination M.Socksaddr, err error) {
	select {
	case p := <-c.data:
		if c.readWaitOptions.NeedHeadroom() {
			var readBuffer *buf.Buffer
			buffer, readBuffer = c.readWaitOptions.NewPacketBuffer()
			_, err = readBuffer.Write(p.data.Bytes())
			if err != nil {
				buffer.Release()
				return
			}
			p.data.Release()
			buffer.Resize(readBuffer.Start(), readBuffer.Len())
		} else {
			buffer = p.data
		}
		destination = p.destination
		return
	case <-c.ctx.Done():
		return nil, M.Socksaddr{}, io.ErrClosedPipe
	}
}
