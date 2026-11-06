package bufio

import (
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type CachedConn struct {
	net.Conn
	access sync.Mutex
	buffer *buf.Buffer
}

func NewCachedConn(conn net.Conn, buffer *buf.Buffer) *CachedConn {
	buffer.IncRef()
	return &CachedConn{
		Conn:   conn,
		buffer: buffer,
	}
}

func (c *CachedConn) ReadCached() *buf.Buffer {
	c.access.Lock()
	defer c.access.Unlock()
	buffer := c.buffer
	c.buffer = nil
	if buffer != nil {
		buffer.DecRef()
	}
	return buffer
}

func (c *CachedConn) Read(p []byte) (n int, err error) {
	c.access.Lock()
	if c.buffer != nil {
		n, err = c.buffer.Read(p)
		if c.buffer.IsEmpty() {
			c.buffer.DecRef()
			c.buffer.Release()
			c.buffer = nil
		}
		if err == nil {
			c.access.Unlock()
			return
		}
	}
	c.access.Unlock()
	return c.Conn.Read(p)
}

func (c *CachedConn) WriteTo(w io.Writer) (n int64, err error) {
	if buffer := c.ReadCached(); buffer != nil {
		n, err = buffer.WriteTo(w)
		buffer.Release()
		if err != nil {
			return
		}
	}
	cn, err := Copy(w, c.Conn)
	n += cn
	return
}

func (c *CachedConn) ReadFrom(r io.Reader) (n int64, err error) {
	return Copy(c.Conn, r)
}

func (c *CachedConn) Upstream() any {
	return c.Conn
}

func (c *CachedConn) ReaderReplaceable() bool {
	c.access.Lock()
	defer c.access.Unlock()
	return c.buffer == nil
}

func (c *CachedConn) WriterReplaceable() bool {
	return true
}

func (c *CachedConn) Close() error {
	c.ReadCached().Release()
	return c.Conn.Close()
}

type CachedReader struct {
	upstream io.Reader
	access   sync.Mutex
	buffer   *buf.Buffer
}

func NewCachedReader(upstream io.Reader, buffer *buf.Buffer) *CachedReader {
	buffer.IncRef()
	return &CachedReader{
		upstream: upstream,
		buffer:   buffer,
	}
}

func (r *CachedReader) ReadCached() *buf.Buffer {
	r.access.Lock()
	defer r.access.Unlock()
	buffer := r.buffer
	r.buffer = nil
	if buffer != nil {
		buffer.DecRef()
	}
	return buffer
}

func (r *CachedReader) Read(p []byte) (n int, err error) {
	r.access.Lock()
	if r.buffer != nil {
		n, err = r.buffer.Read(p)
		if r.buffer.IsEmpty() {
			r.buffer.DecRef()
			r.buffer.Release()
			r.buffer = nil
		}
		if err == nil {
			r.access.Unlock()
			return
		}
	}
	r.access.Unlock()
	return r.upstream.Read(p)
}

func (r *CachedReader) WriteTo(w io.Writer) (n int64, err error) {
	if buffer := r.ReadCached(); buffer != nil {
		n, err = buffer.WriteTo(w)
		buffer.Release()
		if err != nil {
			return
		}
	}
	cn, err := Copy(w, r.upstream)
	n += cn
	return
}

func (r *CachedReader) Upstream() any {
	return r.upstream
}

func (r *CachedReader) ReaderReplaceable() bool {
	r.access.Lock()
	defer r.access.Unlock()
	return r.buffer == nil
}

func (r *CachedReader) Close() error {
	r.ReadCached().Release()
	return nil
}

type CachedPacketConn struct {
	N.PacketConn
	taken       atomic.Bool
	buffer      *buf.Buffer
	destination M.Socksaddr
}

func NewCachedPacketConn(conn N.PacketConn, buffer *buf.Buffer, destination M.Socksaddr) *CachedPacketConn {
	buffer.IncRef()
	return &CachedPacketConn{
		PacketConn:  conn,
		buffer:      buffer,
		destination: destination,
	}
}

func (c *CachedPacketConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	packet := c.ReadCachedPacket()
	if packet != nil {
		defer packet.Buffer.Release()
		defer N.PutPacketBuffer(packet)
		_, err = buffer.Write(packet.Buffer.Bytes())
		return packet.Destination, err
	}
	return c.PacketConn.ReadPacket(buffer)
}

func (c *CachedPacketConn) ReadCachedPacket() *N.PacketBuffer {
	if !c.taken.CompareAndSwap(false, true) {
		return nil
	}
	buffer := c.buffer
	c.buffer = nil
	if buffer == nil {
		return nil
	}
	buffer.DecRef()
	packet := N.NewPacketBuffer()
	*packet = N.PacketBuffer{
		Buffer:      buffer,
		Destination: c.destination,
	}
	return packet
}

func (c *CachedPacketConn) Upstream() any {
	return c.PacketConn
}

func (c *CachedPacketConn) ReaderReplaceable() bool {
	return c.taken.Load()
}

func (c *CachedPacketConn) WriterReplaceable() bool {
	return true
}

func (c *CachedPacketConn) Close() error {
	if c.taken.CompareAndSwap(false, true) {
		if buffer := c.buffer; buffer != nil {
			c.buffer = nil
			buffer.DecRef()
			buffer.Release()
		}
	}
	return c.PacketConn.Close()
}
