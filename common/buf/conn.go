package buf

import (
	"io"
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

func (r *BufferedReader) Read(p []byte) (n int, err error) {
	if r.Buffer != nil {
		n, err = r.Buffer.Read(p)
		if err == nil {
			return
		}
		r.Buffer = nil
	}
	return r.Reader.Read(p)
}

type BufferedWriter struct {
	Writer io.Writer
	Buffer *Buffer
}

func (w *BufferedWriter) Upstream() io.Writer {
	if w.Buffer != nil {
		return nil
	}
	return w.Writer
}

func (w *BufferedWriter) Write(p []byte) (n int, err error) {
	if w.Buffer == nil {
		return w.Writer.Write(p)
	}
	n, err = w.Buffer.Write(p)
	if err == nil {
		return
	}
	n, err = w.Writer.Write(w.Buffer.Bytes())
	if err != nil {
		return 0, err
	}
	w.Buffer.Release()
	w.Buffer = nil
	return w.Writer.Write(p)
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
	_, err := w.Writer.Write(buffer.Bytes())
	return err
}
