package bufio

import (
	"io"

	"github.com/sagernet/sing/common/buf"
)

type BufferedReader struct {
	upstream io.Reader
	buffer   *buf.Buffer
}

func NewBufferedReader(upstream io.Reader, buffer *buf.Buffer) *BufferedReader {
	return &BufferedReader{
		upstream: upstream,
		buffer:   buffer,
	}
}

func (r *BufferedReader) Read(p []byte) (n int, err error) {
	if r.buffer.IsEmpty() {
		r.buffer.FullReset()
		_, err = r.buffer.ReadFrom(r.upstream)
		if err != nil {
			return
		}
	}
	return r.buffer.Read(p)
}

func (w *BufferedReader) Upstream() any {
	return w.upstream
}

type BufferedWriter struct {
	upstream io.Writer
	buffer   *buf.Buffer
}

func NewBufferedWriter(upstream io.Writer, buffer *buf.Buffer) *BufferedWriter {
	return &BufferedWriter{
		upstream: upstream,
		buffer:   buffer,
	}
}

func (w *BufferedWriter) Write(p []byte) (n int, err error) {
	n, err = w.buffer.Write(p)
	if n == len(p) {
		return
	}
	_, err = w.upstream.Write(w.buffer.Bytes())
	if err != nil {
		return
	}
	w.buffer.FullReset()
	_, err = w.buffer.Write(p[n:])
	if err != nil {
		return
	}
	n = len(p)
	return
}

func (w *BufferedWriter) Flush() error {
	if w.buffer.IsEmpty() {
		return nil
	}
	_, err := w.upstream.Write(w.buffer.Bytes())
	if err != nil {
		return err
	}
	w.buffer.FullReset()
	return nil
}

func (w *BufferedWriter) Upstream() any {
	return w.upstream
}
