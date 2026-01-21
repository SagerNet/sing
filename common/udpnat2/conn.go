package udpnat

import (
	"io"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/canceler"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/contrab/freelru"
)

type Conn interface {
	N.PacketConn
	SetHandler(handler N.UDPHandlerEx)
	canceler.PacketConn
}

var (
	_ Conn               = (*natConn)(nil)
	_ N.PacketPushable   = (*natConn)(nil)
	_ N.PacketReadWaiter = (*natConn)(nil)
)

type natConn struct {
	cache           freelru.Cache[netip.AddrPort, *natConn]
	writer          N.PacketWriter
	localAddr       M.Socksaddr
	handlerAccess   sync.RWMutex
	handler         N.UDPHandlerEx
	readWaitOptions N.ReadWaitOptions

	dataQueue   []*N.PacketBuffer
	queueMutex  sync.Mutex
	onDataReady func()

	deadlineMutex sync.Mutex
	deadlineTimer *time.Timer
	deadlineChan  chan struct{}
	dataSignal    chan struct{}

	closeOnce sync.Once
	doneChan  chan struct{}
}

func (c *natConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	for {
		select {
		case <-c.doneChan:
			return M.Socksaddr{}, io.ErrClosedPipe
		default:
		}

		c.queueMutex.Lock()
		if len(c.dataQueue) > 0 {
			packet := c.dataQueue[0]
			c.dataQueue = c.dataQueue[1:]
			c.queueMutex.Unlock()
			_, err = buffer.ReadOnceFrom(packet.Buffer)
			destination = packet.Destination
			packet.Buffer.Release()
			N.PutPacketBuffer(packet)
			return
		}
		c.queueMutex.Unlock()

		select {
		case <-c.doneChan:
			return M.Socksaddr{}, io.ErrClosedPipe
		case <-c.waitDeadline():
			return M.Socksaddr{}, os.ErrDeadlineExceeded
		case <-c.dataSignal:
			continue
		}
	}
}

func (c *natConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	return c.writer.WritePacket(buffer, destination)
}

func (c *natConn) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	c.handlerAccess.Lock()
	defer c.handlerAccess.Unlock()
	c.readWaitOptions = options
	return false
}

func (c *natConn) WaitReadPacket() (buffer *buf.Buffer, destination M.Socksaddr, err error) {
	for {
		select {
		case <-c.doneChan:
			return nil, M.Socksaddr{}, io.ErrClosedPipe
		default:
		}

		c.queueMutex.Lock()
		if len(c.dataQueue) > 0 {
			packet := c.dataQueue[0]
			c.dataQueue = c.dataQueue[1:]
			c.queueMutex.Unlock()
			buffer = c.readWaitOptions.Copy(packet.Buffer)
			destination = packet.Destination
			N.PutPacketBuffer(packet)
			return
		}
		c.queueMutex.Unlock()

		select {
		case <-c.doneChan:
			return nil, M.Socksaddr{}, io.ErrClosedPipe
		case <-c.waitDeadline():
			return nil, M.Socksaddr{}, os.ErrDeadlineExceeded
		case <-c.dataSignal:
			continue
		}
	}
}

func (c *natConn) SetHandler(handler N.UDPHandlerEx) {
	c.handlerAccess.Lock()
	c.handler = handler
	c.readWaitOptions = N.NewReadWaitOptions(c.writer, handler)
	c.handlerAccess.Unlock()

	c.queueMutex.Lock()
	pending := c.dataQueue
	c.dataQueue = nil
	c.queueMutex.Unlock()

	for _, packet := range pending {
		handler.NewPacketEx(packet.Buffer, packet.Destination)
		N.PutPacketBuffer(packet)
	}
}

func (c *natConn) SetOnDataReady(callback func()) {
	c.queueMutex.Lock()
	c.onDataReady = callback
	c.queueMutex.Unlock()
}

func (c *natConn) HasPendingData() bool {
	c.queueMutex.Lock()
	defer c.queueMutex.Unlock()
	return len(c.dataQueue) > 0
}

func (c *natConn) PushPacket(packet *N.PacketBuffer) {
	c.queueMutex.Lock()
	if len(c.dataQueue) >= 64 {
		c.queueMutex.Unlock()
		packet.Buffer.Release()
		N.PutPacketBuffer(packet)
		return
	}
	c.dataQueue = append(c.dataQueue, packet)
	callback := c.onDataReady
	c.queueMutex.Unlock()

	select {
	case c.dataSignal <- struct{}{}:
	default:
	}

	if callback != nil {
		callback()
	}
}

func (c *natConn) Timeout() time.Duration {
	rawConn, lifetime, loaded := c.cache.PeekWithLifetime(c.localAddr.AddrPort())
	if !loaded || rawConn != c {
		return 0
	}
	return time.Until(lifetime)
}

func (c *natConn) SetTimeout(timeout time.Duration) bool {
	return c.cache.UpdateLifetime(c.localAddr.AddrPort(), c, timeout)
}

func (c *natConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.doneChan)

		c.queueMutex.Lock()
		pending := c.dataQueue
		c.dataQueue = nil
		c.onDataReady = nil
		c.queueMutex.Unlock()

		N.ReleaseMultiPacketBuffer(pending)
		common.Close(c.handler)
	})
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
	c.deadlineMutex.Lock()
	defer c.deadlineMutex.Unlock()

	if c.deadlineTimer != nil && !c.deadlineTimer.Stop() {
		<-c.deadlineChan
	}
	c.deadlineTimer = nil

	if t.IsZero() {
		if isClosedChan(c.deadlineChan) {
			c.deadlineChan = make(chan struct{})
		}
		return nil
	}

	if duration := time.Until(t); duration > 0 {
		if isClosedChan(c.deadlineChan) {
			c.deadlineChan = make(chan struct{})
		}
		c.deadlineTimer = time.AfterFunc(duration, func() {
			close(c.deadlineChan)
		})
		return nil
	}

	if !isClosedChan(c.deadlineChan) {
		close(c.deadlineChan)
	}
	return nil
}

func (c *natConn) waitDeadline() chan struct{} {
	c.deadlineMutex.Lock()
	defer c.deadlineMutex.Unlock()
	return c.deadlineChan
}

func isClosedChan(channel <-chan struct{}) bool {
	select {
	case <-channel:
		return true
	default:
		return false
	}
}

func (c *natConn) SetWriteDeadline(t time.Time) error {
	return os.ErrInvalid
}

func (c *natConn) Upstream() any {
	return c.writer
}
