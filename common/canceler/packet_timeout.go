package canceler

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type TimeoutPacketConn struct {
	N.PacketConn
	access  sync.RWMutex
	timeout time.Duration
	cancel  context.CancelCauseFunc
	active  time.Time
}

func NewTimeoutPacketConn(ctx context.Context, conn N.PacketConn, timeout time.Duration) (context.Context, PacketConn) {
	ctx, cancel := context.WithCancelCause(ctx)
	return ctx, &TimeoutPacketConn{
		PacketConn: conn,
		timeout:    timeout,
		cancel:     cancel,
	}
}

func (c *TimeoutPacketConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	for {
		err = c.setReadDeadline()
		if err != nil {
			return
		}
		destination, err = c.PacketConn.ReadPacket(buffer)
		if err == nil {
			c.updateActive()
			return
		} else if E.IsTimeout(err) {
			if c.isInactive() {
				c.cancel(err)
				return
			}
		} else {
			return
		}
	}
}

func (c *TimeoutPacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	err := c.PacketConn.WritePacket(buffer, destination)
	if err == nil {
		c.updateActive()
	}
	return err
}

func (c *TimeoutPacketConn) Timeout() time.Duration {
	c.access.RLock()
	defer c.access.RUnlock()
	return c.timeout
}

func (c *TimeoutPacketConn) SetTimeout(timeout time.Duration) bool {
	c.access.Lock()
	defer c.access.Unlock()
	c.timeout = timeout
	return c.PacketConn.SetReadDeadline(time.Now()) == nil
}

func (c *TimeoutPacketConn) setReadDeadline() error {
	c.access.RLock()
	defer c.access.RUnlock()
	return c.PacketConn.SetReadDeadline(time.Now().Add(c.timeout))
}

func (c *TimeoutPacketConn) updateActive() {
	c.access.Lock()
	c.active = time.Now()
	c.access.Unlock()
}

func (c *TimeoutPacketConn) isInactive() bool {
	c.access.RLock()
	defer c.access.RUnlock()
	return time.Since(c.active) > c.timeout
}

func (c *TimeoutPacketConn) Close() error {
	c.cancel(net.ErrClosed)
	return c.PacketConn.Close()
}

func (c *TimeoutPacketConn) Upstream() any {
	return c.PacketConn
}
