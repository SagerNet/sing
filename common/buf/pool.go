package buf

import "sync"

const (
	ReversedHeader = 1024
	BufferSize     = 20 * 1024
)

var pool = sync.Pool{
	New: func() any {
		var buffer [BufferSize]byte
		return buffer[:]
	},
}

func GetBytes() []byte {
	return pool.Get().([]byte)
}

func PutBytes(buffer []byte) {
	pool.Put(buffer)
}
