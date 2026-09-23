package udpnat

import (
	"io"
	"os"

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
			buffer = c.readWaitOptions.NewPacketBuffer()
			var n int
			n, err = buffer.Write(p.data.Bytes())
			if err == nil && n != p.data.Len() {
				err = io.ErrShortBuffer
			}
			p.data.Release()
			if err != nil {
				buffer.Release()
				buffer = nil
				return
			}
			c.readWaitOptions.PostReturn(buffer)
		} else {
			buffer = p.data
		}
		destination = p.destination
		return
	case <-c.ctx.Done():
		return nil, M.Socksaddr{}, io.ErrClosedPipe
	case <-c.readDeadline.Wait():
		return nil, M.Socksaddr{}, os.ErrDeadlineExceeded
	}
}
