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
