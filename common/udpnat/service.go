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
	"github.com/sagernet/sing/common/pipe"
)

// Deprecated: Use N.UDPConnectionHandler instead.
//
//nolint:staticcheck
type Handler interface {
	N.UDPConnectionHandler
	E.Handler
}

type Service[K comparable] struct {
	nat       *cache.LruCache[K, *conn]
	handler   Handler
	handlerEx N.UDPConnectionHandlerEx
}

// Deprecated: Use NewEx instead.
func New[K comparable](maxAge int64, handler Handler) *Service[K] {
	service := &Service[K]{
		nat: cache.New(
			cache.WithAge[K, *conn](maxAge),
			cache.WithUpdateAgeOnGet[K, *conn](),
			cache.WithEvict[K, *conn](func(key K, conn *conn) {
				conn.Close()
			}),
		),
		handler: handler,
	}
	return service
}

func NewEx[K comparable](maxAge int64, handler N.UDPConnectionHandlerEx) *Service[K] {
	service := &Service[K]{
		nat: cache.New(
			cache.WithAge[K, *conn](maxAge),
			cache.WithUpdateAgeOnGet[K, *conn](),
			cache.WithEvict[K, *conn](func(key K, conn *conn) {
				conn.Close()
			}),
		),
		handlerEx: handler,
	}
	return service
}

func (s *Service[T]) WriteIsThreadUnsafe() {
}

// Deprecated: don't use
func (s *Service[T]) NewPacketDirect(ctx context.Context, key T, conn N.PacketConn, buffer *buf.Buffer, metadata M.Metadata) {
	s.NewContextPacket(ctx, key, buffer, metadata, func(natConn N.PacketConn) (context.Context, N.PacketWriter) {
		return ctx, &DirectBackWriter{conn, natConn}
	})
}

type DirectBackWriter struct {
	Source N.PacketConn
	Nat    N.PacketConn
}

func (w *DirectBackWriter) WritePacket(buffer *buf.Buffer, addr M.Socksaddr) error {
	return w.Source.WritePacket(buffer, M.SocksaddrFromNet(w.Nat.LocalAddr()))
}

func (w *DirectBackWriter) Upstream() any {
	return w.Source
}

// Deprecated: use NewPacketEx instead.
func (s *Service[T]) NewPacket(ctx context.Context, key T, buffer *buf.Buffer, metadata M.Metadata, init func(natConn N.PacketConn) N.PacketWriter) {
	s.NewContextPacket(ctx, key, buffer, metadata, func(natConn N.PacketConn) (context.Context, N.PacketWriter) {
		return ctx, init(natConn)
	})
}

func (s *Service[T]) NewPacketEx(ctx context.Context, key T, buffer *buf.Buffer, source M.Socksaddr, destination M.Socksaddr, init func(natConn N.PacketConn) N.PacketWriter) {
	s.NewContextPacketEx(ctx, key, buffer, source, destination, func(natConn N.PacketConn) (context.Context, N.PacketWriter) {
		return ctx, init(natConn)
	})
}

// Deprecated: Use NewPacketConnectionEx instead.
func (s *Service[T]) NewContextPacket(ctx context.Context, key T, buffer *buf.Buffer, metadata M.Metadata, init func(natConn N.PacketConn) (context.Context, N.PacketWriter)) {
	s.NewContextPacketEx(ctx, key, buffer, metadata.Source, metadata.Destination, init)
}

func (s *Service[T]) NewContextPacketEx(ctx context.Context, key T, buffer *buf.Buffer, source M.Socksaddr, destination M.Socksaddr, init func(natConn N.PacketConn) (context.Context, N.PacketWriter)) {
	for {
		if common.Done(ctx) {
			buffer.Release()
			return
		}
		c, loaded := s.nat.LoadOrStore(key, func() *conn {
			c := &conn{
				data:         make(chan packet, 64),
				localAddr:    source,
				remoteAddr:   destination,
				readDeadline: pipe.MakeDeadline(),
			}
			c.ctx, c.cancel = context.WithCancelCause(ctx)
			return c
		})
		if !loaded {
			callbackContext, writer := init(c)
			c.metadataAccess.Lock()
			c.source = writer
			c.sourceReady = true
			c.metadataAccess.Unlock()
			if common.Done(c.ctx) {
				buffer.Release()
				c.Close()
				s.nat.DeleteIf(key, func(current *conn) bool { return current == c })
				return
			}
			go func() {
				if s.handlerEx != nil {
					s.handlerEx.NewPacketConnectionEx(callbackContext, c, source, destination, func(error) {
						s.nat.DeleteIf(key, func(current *conn) bool { return current == c })
					})
				} else {
					//nolint:staticcheck
					err := s.handler.NewPacketConnection(callbackContext, c, M.Metadata{
						Source: source, Destination: destination,
					})
					if err != nil {
						s.handler.NewError(callbackContext, err)
					}
					c.Close()
					s.nat.DeleteIf(key, func(current *conn) bool { return current == c })
				}
			}()
		} else {
			if common.Done(c.ctx) {
				s.nat.DeleteIf(key, func(current *conn) bool { return current == c })
				continue
			}
			c.metadataAccess.Lock()
			c.localAddr = source
			c.metadataAccess.Unlock()
		}
		c.enqueue(packet{data: buffer, destination: destination})
		return
	}
}

// enqueue takes ownership of the packet whether accepted or canceled.
func (c *conn) enqueue(p packet) {
	c.enqueueAccess.Lock()
	defer c.enqueueAccess.Unlock()
	if !common.Done(c.ctx) {
		select {
		case c.data <- p:
			return
		case <-c.ctx.Done():
		}
	}
	p.data.Release()
}

type packet struct {
	data        *buf.Buffer
	destination M.Socksaddr
}

var _ N.PacketConn = (*conn)(nil)

type conn struct {
	ctx             context.Context
	cancel          context.CancelCauseFunc
	data            chan packet
	localAddr       M.Socksaddr
	remoteAddr      M.Socksaddr
	source          N.PacketWriter
	readDeadline    pipe.Deadline
	readWaitOptions N.ReadWaitOptions
	sourceReady     bool
	metadataAccess  sync.RWMutex
	enqueueAccess   sync.Mutex
	sourceCloseOnce sync.Once
	sourceCloseErr  error
}

func (c *conn) ReadPacket(buffer *buf.Buffer) (addr M.Socksaddr, err error) {
	select {
	case p := <-c.data:
		length := p.data.Len()
		var n int
		n, err = buffer.ReadOnceFrom(p.data)
		if err == nil && n != length {
			err = io.ErrShortBuffer
		}
		p.data.Release()
		return p.destination, err
	case <-c.ctx.Done():
		return M.Socksaddr{}, io.ErrClosedPipe
	case <-c.readDeadline.Wait():
		return M.Socksaddr{}, os.ErrDeadlineExceeded
	}
}

func (c *conn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	return c.source.WritePacket(buffer, destination)
}

func (c *conn) Close() error {
	c.cancel(net.ErrClosed)
	c.enqueueAccess.Lock()
	for {
		select {
		case p := <-c.data:
			p.data.Release()
		default:
			c.enqueueAccess.Unlock()
			goto drained
		}
	}
drained:
	c.metadataAccess.RLock()
	source, ready := c.source, c.sourceReady
	c.metadataAccess.RUnlock()
	if ready {
		c.sourceCloseOnce.Do(func() {
			if closer, ok := source.(io.Closer); ok {
				c.sourceCloseErr = closer.Close()
			}
		})
		return c.sourceCloseErr
	}
	return nil
}

func (c *conn) LocalAddr() net.Addr {
	c.metadataAccess.RLock()
	defer c.metadataAccess.RUnlock()
	return c.localAddr
}

func (c *conn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *conn) SetDeadline(t time.Time) error {
	return os.ErrInvalid
}

func (c *conn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Set(t)
	return nil
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	return os.ErrInvalid
}

func (c *conn) Upstream() any {
	c.metadataAccess.RLock()
	defer c.metadataAccess.RUnlock()
	return c.source
}
