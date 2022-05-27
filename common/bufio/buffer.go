package bufio

import (
	"io"
	"net"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/rw"
)

type CachedReader interface {
	ReadCached() *buf.Buffer
}

type BufferedConn struct {
	net.Conn
	Buffer *buf.Buffer
}

func (c *BufferedConn) ReadCached() *buf.Buffer {
	buffer := c.Buffer
	c.Buffer = nil
	return buffer
}

func (c *BufferedConn) Read(p []byte) (n int, err error) {
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

func (c *BufferedConn) WriteTo(w io.Writer) (n int64, err error) {
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

func (c *BufferedConn) SetReadDeadline(t time.Time) error {
	if c.Buffer != nil && !c.Buffer.IsEmpty() {
		return nil
	}
	return c.Conn.SetReadDeadline(t)
}

func (c *BufferedConn) ReadFrom(r io.Reader) (n int64, err error) {
	return Copy(c.Conn, r)
}

func (c *BufferedConn) Upstream() any {
	return c.Conn
}

func (c *BufferedConn) ReaderReplaceable() bool {
	return c.Buffer == nil
}

func (c *BufferedConn) WriterReplaceable() bool {
	return true
}

type BufferedReader struct {
	Reader io.Reader
	Buffer *buf.Buffer
}

func (c *BufferedReader) ReadCached() *buf.Buffer {
	buffer := c.Buffer
	c.Buffer = nil
	return buffer
}

func (r *BufferedReader) Read(p []byte) (n int, err error) {
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

func (r *BufferedReader) WriteTo(w io.Writer) (n int64, err error) {
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

func (w *BufferedReader) Upstream() any {
	return w.Reader
}

func (c *BufferedReader) ReaderReplaceable() bool {
	return c.Buffer == nil
}

type BufferedWriter struct {
	Writer io.Writer
	Buffer *buf.Buffer
}

func (w *BufferedWriter) Write(p []byte) (n int, err error) {
	if w.Buffer == nil {
		return w.Writer.Write(p)
	}
	n, err = w.Buffer.Write(p)
	if n == len(p) {
		return
	}
	fd, err := common.TryFileDescriptor(w.Writer)
	if err == nil {
		_, err = rw.WriteV(fd, w.Buffer.Bytes(), p[n:])
		if err != nil {
			return
		}
		w.Buffer.Release()
		w.Buffer = nil
		return len(p), nil
	}
	_, err = w.Writer.Write(w.Buffer.Bytes())
	if err != nil {
		return
	}
	err = w.Flush()
	if err != nil {
		return
	}
	_, err = w.Writer.Write(p[n:])
	if err != nil {
		return
	}
	return len(p), nil
}

func (w *BufferedWriter) Flush() error {
	if w.Buffer == nil {
		return nil
	}
	if w.Buffer.IsEmpty() {
		w.Buffer.Release()
		w.Buffer = nil
		return nil
	}
	_, err := w.Writer.Write(w.Buffer.Bytes())
	if err != nil {
		return err
	}
	w.Buffer.Release()
	w.Buffer = nil
	return nil
}

func (w *BufferedWriter) Close() error {
	buffer := w.Buffer
	if buffer != nil {
		w.Buffer = nil
		buffer.Release()
	}
	return nil
}

func (w *BufferedWriter) Upstream() any {
	return w.Writer
}

func (w *BufferedWriter) WriterReplaceable() bool {
	return w.Buffer == nil
}

type HeaderWriter struct {
	Writer io.Writer
	Header *buf.Buffer
}

func (w *HeaderWriter) Write(p []byte) (n int, err error) {
	if w.Header == nil {
		return w.Writer.Write(p)
	}
	fd, err := common.TryFileDescriptor(w.Writer)
	if err == nil {
		_, err = rw.WriteV(fd, w.Header.Bytes(), p)
		if err != nil {
			return
		}
		w.Header.Release()
		w.Header = nil
		return len(p), nil
	}
	cachedN, _ := w.Header.Write(p)
	_, err = w.Writer.Write(w.Header.Bytes())
	if err != nil {
		return
	}
	w.Header.Release()
	w.Header = nil
	if cachedN < len(p) {
		_, err = w.Writer.Write(p[cachedN:])
		if err != nil {
			return
		}
	}
	return len(p), nil
}

func (w *HeaderWriter) Close() error {
	buffer := w.Header
	if buffer != nil {
		w.Header = nil
		buffer.Release()
	}
	return nil
}

func (w *HeaderWriter) Upstream() any {
	return w.Writer
}

func (w *HeaderWriter) WriterReplaceable() bool {
	return w.Header == nil
}
