package buf

import "sync"

const (
	ReversedHeader = 1024
	BufferSize     = 20 * 1024
	UDPBufferSize  = 16 * 1024
)

var pool = sync.Pool{
	New: func() any {
		buffer := make([]byte, BufferSize)
		return &buffer
	},
}

func GetBytes() []byte {
	return *pool.Get().(*[]byte)
}

func PutBytes(buffer []byte) {
	pool.Put(&buffer)
}

func Make(size int) []byte {
	var buffer []byte
	if size <= 16 {
		buffer = make([]byte, 16)
	} else if size <= 32 {
		buffer = make([]byte, 32)
	} else if size <= 64 {
		buffer = make([]byte, 64)
	} else if size <= 128 {
		buffer = make([]byte, 128)
	} else if size <= 256 {
		buffer = make([]byte, 256)
	} else if size <= 512 {
		buffer = make([]byte, 512)
	} else if size <= 1024 {
		buffer = make([]byte, 1024)
	} else if size <= 4*1024 {
		buffer = make([]byte, 4*1024)
	} else if size <= 16*1024 {
		buffer = make([]byte, 16*1024)
	} else if size <= 20*1024 {
		buffer = make([]byte, 20*1024)
	} else if size <= 65535 {
		buffer = make([]byte, 65535)
	} else {
		return make([]byte, size)
	}
	return buffer[:size]
}
