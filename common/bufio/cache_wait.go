package bufio

import (
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
)

type cachedReadWaiter struct {
	reader ReadWaiter
	cache  *buf.Buffer
}

func (c *cachedReadWaiter) WaitReadBuffer(newBuffer func() *buf.Buffer) error {
	cache := c.cache
	if cache != nil {
		var err error
		if !cache.IsEmpty() {
			_, err = newBuffer().ReadOnceFrom(c.cache)
		}
		if cache.IsEmpty() {
			cache.Release()
			c.cache = nil
		}
		return err
	}
	return c.reader.WaitReadBuffer(newBuffer)
}

type cachedPacketReadWaiter struct {
	reader      PacketReadWaiter
	cache       *buf.Buffer
	destination M.Socksaddr
}

func (c *cachedPacketReadWaiter) WaitReadPacket(newBuffer func() *buf.Buffer) (destination M.Socksaddr, err error) {
	cache := c.cache
	if cache != nil {
		if !cache.IsEmpty() {
			_, err = newBuffer().ReadOnceFrom(c.cache)
		}
		if cache.IsEmpty() {
			cache.Release()
			c.cache = nil
		}
		destination = c.destination
		return
	}
	return c.reader.WaitReadPacket(newBuffer)
}
