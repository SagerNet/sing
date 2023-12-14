package buf

import "sync"

func Get(size int) []byte {
	if size == 0 {
		return nil
	}
	return DefaultAllocator.Get(size)
}

func Put(buf []byte) error {
	return DefaultAllocator.Put(buf)
}

var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(Buffer)
	},
}

func getBuffer() *Buffer {
	return bufferPool.Get().(*Buffer)
}

func putBuffer(b *Buffer) {
	bufferPool.Put(b)
}

// Deprecated: use array instead.
func Make(size int) []byte {
	return make([]byte, size)
}
