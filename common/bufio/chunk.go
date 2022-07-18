package bufio

import (
	"io"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	N "github.com/sagernet/sing/common/network"
)

type ChunkWriter struct {
	upstream     N.ExtendedWriter
	maxChunkSize int
}

func NewChunkWriter(writer io.Writer, maxChunkSize int) *ChunkWriter {
	return &ChunkWriter{
		upstream:     NewExtendedWriter(writer),
		maxChunkSize: maxChunkSize,
	}
}

func (w *ChunkWriter) Write(p []byte) (n int, err error) {
	for pLen := len(p); pLen > 0; {
		var data []byte
		if pLen > w.maxChunkSize {
			data = p[:w.maxChunkSize]
			p = p[w.maxChunkSize:]
			pLen -= w.maxChunkSize
		} else {
			data = p
			pLen = 0
		}
		var writeN int
		writeN, err = w.upstream.Write(data)
		n += writeN
		if err != nil {
			return
		}
	}
	return
}

func (w *ChunkWriter) WriteBuffer(buffer *buf.Buffer) error {
	if buffer.Len() > w.maxChunkSize {
		defer buffer.Release()
		return common.Error(w.Write(buffer.Bytes()))
	}
	return w.upstream.WriteBuffer(buffer)
}

func (w *ChunkWriter) Upstream() any {
	return w.upstream
}
