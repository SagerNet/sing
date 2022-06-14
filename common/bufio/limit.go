package bufio

import (
	"io"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	N "github.com/sagernet/sing/common/network"
)

type LimitedWriter struct {
	upstream       N.ExtendedWriter
	maxChunkLength int
}

func NewLimitedWriter(writer io.Writer, maxChunkLength int) *LimitedWriter {
	return &LimitedWriter{
		upstream:       NewExtendedWriter(writer),
		maxChunkLength: maxChunkLength,
	}
}

func (w *LimitedWriter) Write(p []byte) (n int, err error) {
	for pLen := len(p); pLen > 0; {
		var data []byte
		if pLen > w.maxChunkLength {
			data = p[:w.maxChunkLength]
			p = p[w.maxChunkLength:]
			pLen -= w.maxChunkLength
		} else {
			data = p
			pLen = 0
		}
		var writeN int
		writeN, err = w.upstream.Write(data)
		if err != nil {
			return
		}
		n += writeN
	}
	return
}

func (w *LimitedWriter) WriteBuffer(buffer *buf.Buffer) error {
	if buffer.Len() <= w.maxChunkLength {
		return w.upstream.WriteBuffer(buffer)
	}
	defer buffer.Release()
	return common.Error(w.Write(buffer.Bytes()))
}
