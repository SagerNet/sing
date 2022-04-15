package buf

import "sync"

const (
	ReversedHeader = 1024
	BufferSize     = 20 * 1024
	UDPBufferSize  = 16 * 1024
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

func Make(size int) []byte {
	var buffer []byte
	if size <= 64 {
		buffer = make([]byte, 64)
	} else if size <= 1024 {
		buffer = make([]byte, 1024)
	} else if size <= 4096 {
		buffer = make([]byte, 4096)
	} else if size <= 16384 {
		buffer = make([]byte, 16384)
	} else if size <= 65535 {
		buffer = make([]byte, 65535)
	} else {
		buffer = make([]byte, size)
	}
	return buffer[:size]
}
