package rw

import (
	"io"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
)

type BufferedWriter struct {
	Writer io.Writer
	Buffer *buf.Buffer
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
	if n == len(p) {
		return
	}
	fd, err := common.TryFileDescriptor(w.Writer)
	if err == nil {
		_, err = WriteV(fd, w.Buffer.Bytes(), p[n:])
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

type HeaderWriter struct {
	Writer io.Writer
	Header *buf.Buffer
}

func (w *HeaderWriter) Upstream() io.Writer {
	return w.Writer
}

func (w *HeaderWriter) Replaceable() bool {
	return w.Header == nil
}

func (w *HeaderWriter) Write(p []byte) (n int, err error) {
	if w.Header == nil {
		return w.Writer.Write(p)
	}
	fd, err := common.TryFileDescriptor(w.Writer)
	if err == nil {
		_, err = WriteV(fd, w.Header.Bytes(), p)
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
