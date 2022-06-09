package bufio

import (
	"io"
	"net"
	"time"

	"github.com/sagernet/sing/common/buf"
)

type CachedConn struct {
	net.Conn
	Buffer *buf.Buffer
}

func (c *CachedConn) ReadCached() *buf.Buffer {
	buffer := c.Buffer
	c.Buffer = nil
	return buffer
}

func (c *CachedConn) Read(p []byte) (n int, err error) {
	if c.Buffer != nil {
		n, err = c.Buffer.Read(p)
		if err == nil {
			return
		}
		c.Buffer.Release()
		c.Buffer = nil
	}
	return c.Conn.Read(p)
}

func (c *CachedConn) WriteTo(w io.Writer) (n int64, err error) {
	if c.Buffer != nil {
		wn, wErr := w.Write(c.Buffer.Bytes())
		if wErr != nil {
			c.Buffer.Release()
			c.Buffer = nil
		}
		n += int64(wn)
	}
	cn, err := Copy(w, c.Conn)
	n += cn
	return
}

func (c *CachedConn) SetReadDeadline(t time.Time) error {
	if c.Buffer != nil && !c.Buffer.IsEmpty() {
		return nil
	}
	return c.Conn.SetReadDeadline(t)
}

func (c *CachedConn) ReadFrom(r io.Reader) (n int64, err error) {
	return Copy(c.Conn, r)
}

func (c *CachedConn) Upstream() any {
	return c.Conn
}

func (c *CachedConn) ReaderReplaceable() bool {
	return c.Buffer == nil
}

func (c *CachedConn) WriterReplaceable() bool {
	return true
}

type CachedReader struct {
	Reader io.Reader
	Buffer *buf.Buffer
}

func (c *CachedReader) ReadCached() *buf.Buffer {
	buffer := c.Buffer
	c.Buffer = nil
	return buffer
}

func (r *CachedReader) Read(p []byte) (n int, err error) {
	if r.Buffer != nil {
		n, err = r.Buffer.Read(p)
		if err == nil {
			return
		}
		r.Buffer.Release()
		r.Buffer = nil
	}
	return r.Reader.Read(p)
}

func (r *CachedReader) WriteTo(w io.Writer) (n int64, err error) {
	if r.Buffer != nil {
		wn, wErr := w.Write(r.Buffer.Bytes())
		if wErr != nil {
			return 0, wErr
		}
		n += int64(wn)
	}
	cn, err := Copy(w, r.Reader)
	n += cn
	return
}

func (w *CachedReader) Upstream() any {
	return w.Reader
}

func (c *CachedReader) ReaderReplaceable() bool {
	return c.Buffer == nil
}
