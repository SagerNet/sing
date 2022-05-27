package udpnat

import (
	"context"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/cache"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type Handler interface {
	N.UDPConnectionHandler
	E.Handler
}

type Service[K comparable] struct {
	nat     *cache.LruCache[K, *conn]
	handler Handler
}

func New[K comparable](maxAge int64, handler Handler) *Service[K] {
	return &Service[K]{
		nat: cache.New(
			cache.WithAge[K, *conn](maxAge),
			cache.WithUpdateAgeOnGet[K, *conn](),
			cache.WithEvict[K, *conn](func(key K, conn *conn) {
				conn.Close()
			}),
		),
		handler: handler,
	}
}

func (s *Service[T]) NewPacket(ctx context.Context, key T, writer func() N.PacketWriter, buffer *buf.Buffer, metadata M.Metadata) {
	s.NewContextPacket(ctx, key, func() (context.Context, N.PacketWriter) { return ctx, writer() }, buffer, metadata)
}

func (s *Service[T]) NewContextPacket(ctx context.Context, key T, init func() (context.Context, N.PacketWriter), buffer *buf.Buffer, metadata M.Metadata) {
	c, loaded := s.nat.LoadOrStore(key, func() *conn {
		c := &conn{
			data:       make(chan packet),
			localAddr:  metadata.Source,
			remoteAddr: metadata.Destination,
			fastClose:  metadata.Destination.Port == 53,
		}
		c.ctx, c.cancel = context.WithCancel(ctx)
		return c
	})
	if !loaded {
		ctx, c.source = init()
		go func() {
			err := s.handler.NewPacketConnection(ctx, c, metadata)
			if err != nil {
				s.handler.HandleError(err)
			}
			c.Close()
			s.nat.Delete(key)
		}()
	}
	c.access.Lock()
	if common.Done(c.ctx) {
		s.nat.Delete(key)
		c.access.Unlock()
		s.NewContextPacket(ctx, key, init, buffer, metadata)
		return
	}
	packetCtx, done := context.WithCancel(c.ctx)
	p := packet{
		done:        done,
		data:        buffer,
		destination: metadata.Destination,
	}
	c.data <- p
	c.access.Unlock()
	<-packetCtx.Done()
}

type packet struct {
	data        *buf.Buffer
	destination M.Socksaddr
	done        context.CancelFunc
}

type conn struct {
	access     sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	data       chan packet
	localAddr  M.Socksaddr
	remoteAddr M.Socksaddr
	source     N.PacketWriter
	fastClose  bool
}

func (c *conn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	select {
	case p, ok := <-c.data:
		if !ok {
			return M.Socksaddr{}, io.ErrClosedPipe
		}
		defer p.data.Release()
		_, err := buffer.ReadFrom(p.data)
		p.done()
		return p.destination, err
	}
}

func (c *conn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	if c.fastClose {
		defer c.Close()
	}
	return c.source.WritePacket(buffer, destination)
}

func (c *conn) Close() error {
	c.access.Lock()
	defer c.access.Unlock()

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
	return c.localAddr
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

func (c *conn) Upstream() any {
	return c.source
}
