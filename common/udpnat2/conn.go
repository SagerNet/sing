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

type Conn struct {
	writer          N.PacketWriter
	localAddr       M.Socksaddr
	handler         N.UDPHandlerEx
	packetChan      chan *N.PacketBuffer
	doneChan        chan struct{}
	readDeadline    pipe.Deadline
	readWaitOptions N.ReadWaitOptions
}

func (c *Conn) ReadPacket(buffer *buf.Buffer) (addr M.Socksaddr, err error) {
	select {
	case p := <-c.packetChan:
		_, err = buffer.ReadOnceFrom(p.Buffer)
		destination := p.Destination
		p.Buffer.Release()
		N.PutPacketBuffer(p)
		return destination, err
	case <-c.doneChan:
		return M.Socksaddr{}, io.ErrClosedPipe
	case <-c.readDeadline.Wait():
		return M.Socksaddr{}, os.ErrDeadlineExceeded
	}
}

func (c *Conn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	return c.writer.WritePacket(buffer, destination)
}

func (c *Conn) SetHandler(handler N.UDPHandlerEx) {
	c.handler = handler
fetch:
	for {
		select {
		case packet := <-c.packetChan:
			c.handler.NewPacketEx(packet.Buffer, packet.Destination)
			N.PutPacketBuffer(packet)
			continue fetch
		default:
			break fetch
		}
	}
}

func (c *Conn) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	c.readWaitOptions = options
	return false
}

func (c *Conn) WaitReadPacket() (buffer *buf.Buffer, destination M.Socksaddr, err error) {
	select {
	case packet := <-c.packetChan:
		buffer = c.readWaitOptions.Copy(packet.Buffer)
		destination = packet.Destination
		N.PutPacketBuffer(packet)
		return
	case <-c.doneChan:
		return nil, M.Socksaddr{}, io.ErrClosedPipe
	case <-c.readDeadline.Wait():
		return nil, M.Socksaddr{}, os.ErrDeadlineExceeded
	}
}

func (c *Conn) Close() error {
	select {
	case <-c.doneChan:
	default:
		close(c.doneChan)
	}
	return nil
}

func (c *Conn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *Conn) RemoteAddr() net.Addr {
	return M.Socksaddr{}
}

func (c *Conn) SetDeadline(t time.Time) error {
	return os.ErrInvalid
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Set(t)
	return nil
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	return os.ErrInvalid
}

func (c *Conn) Upstream() any {
	return c.writer
}
