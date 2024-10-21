package udpnat

import (
	"io"
	"net"
	"os"
	"time"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/pipe"
)

type natConn struct {
	writer          N.PacketWriter
	localAddr       M.Socksaddr
	packetChan      chan *Packet
	doneChan        chan struct{}
	readDeadline    pipe.Deadline
	readWaitOptions N.ReadWaitOptions
}

func (c *natConn) ReadPacket(buffer *buf.Buffer) (addr M.Socksaddr, err error) {
	select {
	case p := <-c.packetChan:
		_, err = buffer.ReadOnceFrom(p.Buffer)
		destination := p.Destination
		p.Buffer.Release()
		PutPacket(p)
		return destination, err
	case <-c.doneChan:
		return M.Socksaddr{}, io.ErrClosedPipe
	case <-c.readDeadline.Wait():
		return M.Socksaddr{}, os.ErrDeadlineExceeded
	}
}

func (c *natConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	return c.writer.WritePacket(buffer, destination)
}

func (c *natConn) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	c.readWaitOptions = options
	return false
}

func (c *natConn) WaitReadPacket() (buffer *buf.Buffer, destination M.Socksaddr, err error) {
	select {
	case packet := <-c.packetChan:
		buffer = c.readWaitOptions.Copy(packet.Buffer)
		destination = packet.Destination
		PutPacket(packet)
		return
	case <-c.doneChan:
		return nil, M.Socksaddr{}, io.ErrClosedPipe
	case <-c.readDeadline.Wait():
		return nil, M.Socksaddr{}, os.ErrDeadlineExceeded
	}
}

func (c *natConn) Close() error {
	select {
	case <-c.doneChan:
	default:
		close(c.doneChan)
	}
	return nil
}

func (c *natConn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *natConn) RemoteAddr() net.Addr {
	return M.Socksaddr{}
}

func (c *natConn) SetDeadline(t time.Time) error {
	return os.ErrInvalid
}

func (c *natConn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Set(t)
	return nil
}

func (c *natConn) SetWriteDeadline(t time.Time) error {
	return os.ErrInvalid
}
