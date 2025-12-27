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

	closeOnce sync.Once
	doneChan  chan struct{}
}

func (c *natConn) ReadPacket(buffer *buf.Buffer) (addr M.Socksaddr, err error) {
	select {
	case <-c.doneChan:
		return M.Socksaddr{}, io.ErrClosedPipe
	default:
	}

	c.queueMutex.Lock()
	if len(c.dataQueue) == 0 {
		c.queueMutex.Unlock()
		return M.Socksaddr{}, os.ErrDeadlineExceeded
	}
	packet := c.dataQueue[0]
	c.dataQueue = c.dataQueue[1:]
	c.queueMutex.Unlock()

	_, err = buffer.ReadOnceFrom(packet.Buffer)
	destination := packet.Destination
	packet.Buffer.Release()
	N.PutPacketBuffer(packet)
	return destination, err
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
	select {
	case <-c.doneChan:
		return nil, M.Socksaddr{}, io.ErrClosedPipe
	default:
	}

	c.queueMutex.Lock()
	if len(c.dataQueue) == 0 {
		c.queueMutex.Unlock()
		return nil, M.Socksaddr{}, os.ErrDeadlineExceeded
	}
	packet := c.dataQueue[0]
	c.dataQueue = c.dataQueue[1:]
	c.queueMutex.Unlock()

	buffer = c.readWaitOptions.Copy(packet.Buffer)
	destination = packet.Destination
	N.PutPacketBuffer(packet)
	return
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
	return nil
}

func (c *natConn) SetWriteDeadline(t time.Time) error {
	return os.ErrInvalid
}

func (c *natConn) Upstream() any {
	return c.writer
}
