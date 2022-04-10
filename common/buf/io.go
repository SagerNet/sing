package buf

import (
	"io"

	"github.com/sagernet/sing/common"
)

type BufferedReader struct {
	Reader io.Reader
	Buffer *Buffer
}

func (r *BufferedReader) Upstream() io.Reader {
	if r.Buffer != nil {
		return nil
	}
	return r.Reader
}

func (r *BufferedReader) Replaceable() bool {
	return r.Buffer == nil
}

func (r *BufferedReader) Read(p []byte) (n int, err error) {
	if r.Buffer != nil {
		n, err = r.Buffer.Read(p)
		if r.Buffer.IsEmpty() {
			r.Buffer.Release()
			r.Buffer = nil
		}
		return
	}
	return r.Reader.Read(p)
}

type BufferedWriter struct {
	Writer io.Writer
	Buffer *Buffer
}

func (w *BufferedWriter) Upstream() io.Writer {
	return w.Writer
}

func (w *BufferedWriter) Replaceable() bool {
	return w.Buffer == nil
}

func (w *BufferedWriter) Write(p []byte) (n int, err error) {
	if w.Buffer == nil {
		return w.Writer.Write(p)
	}
	n, err = w.Buffer.Write(p)
	if err == nil {
		return
	}
	return len(p), w.Flush()
}

func (w *BufferedWriter) Flush() error {
	if w.Buffer == nil {
		return nil
	}
	buffer := w.Buffer
	w.Buffer = nil
	defer buffer.Release()
	if buffer.IsEmpty() {
		return nil
	}
	return common.Error(w.Writer.Write(buffer.Bytes()))
}

func (w *BufferedWriter) Close() error {
	buffer := w.Buffer
	if buffer != nil {
		w.Buffer = nil
		buffer.Release()
	}
	return nil
}
