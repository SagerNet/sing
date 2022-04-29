package udpnat

import (
	"context"
	"io"
	"net"
	"os"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/gsync"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/protocol/socks"
)

type Handler interface {
	socks.UDPConnectionHandler
	E.Handler
}

type Service[K comparable] struct {
	nat     gsync.Map[K, *conn]
	handler Handler
}

func New[T comparable](handler Handler) *Service[T] {
	return &Service[T]{
		handler: handler,
	}
}

func (s *Service[T]) NewPacket(key T, writer func() socks.PacketWriter, buffer *buf.Buffer, metadata M.Metadata) error {
	c, loaded := s.nat.LoadOrStore(key, func() *conn {
		c := &conn{
			data:       make(chan packet),
			remoteAddr: metadata.Source.UDPAddr(),
			source:     writer(),
		}
		c.ctx, c.cancel = context.WithCancel(context.Background())
		return c
	})
	if !loaded {
		go func() {
			err := s.handler.NewPacketConnection(c, metadata)
			if err != nil {
				s.handler.HandleError(err)
			}
		}()
	}
	ctx, done := context.WithCancel(c.ctx)
	p := packet{
		done:        done,
		data:        buffer,
		destination: metadata.Destination,
	}
	c.data <- p
	<-ctx.Done()
	return nil
}

type packet struct {
	data        *buf.Buffer
	destination *M.AddrPort
	done        context.CancelFunc
}

type conn struct {
	ctx        context.Context
	cancel     context.CancelFunc
	data       chan packet
	remoteAddr *net.UDPAddr
	source     socks.PacketWriter
}

func (c *conn) ReadPacket(buffer *buf.Buffer) (*M.AddrPort, error) {
	select {
	case p, ok := <-c.data:
		if !ok {
			return nil, io.ErrClosedPipe
		}
		defer p.data.Release()
		_, err := buffer.ReadFrom(p.data)
		p.done()
		return p.destination, err
	}
}

func (c *conn) WritePacket(buffer *buf.Buffer, destination *M.AddrPort) error {
	return c.source.WritePacket(buffer, destination)
}

func (c *conn) Close() error {
	c.cancel()
	select {
	case <-c.data:
		return os.ErrClosed
	default:
		close(c.data)
		return nil
	}
}

func (c *conn) LocalAddr() net.Addr {
	return &common.DummyAddr{}
}

func (c *conn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *conn) SetDeadline(t time.Time) error {
	return nil
}

func (c *conn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	return nil
}
