package buf

import (
	"bytes"
	"sync"
)

const BufferSize = 20 * 1024

var bufferPool = sync.Pool{
	New: func() any {
		var data [BufferSize]byte
		return bytes.NewBuffer(data[:0])
	},
}

func New() *bytes.Buffer {
	return bufferPool.Get().(*bytes.Buffer)
}

func Extend(buffer *bytes.Buffer, size int) []byte {
	l := buffer.Len()
	buffer.Grow(size)
	return buffer.Bytes()[l : l+size]
}

func Release(buffer *bytes.Buffer) {
	buffer.Reset()
	bufferPool.Put(buffer)
}
