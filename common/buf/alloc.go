package buf

import (
	"errors"
	"math/bits"
	"sync"
)

var DefaultAllocator = newDefaultAllocator()

const MaxPooledBufferSize = 1<<16 + 1<<13

type Allocator interface {
	Get(size int) []byte
	Put(buf []byte) error
}

type defaultAllocator struct {
	buffers [12]sync.Pool
}

func newDefaultAllocator() Allocator {
	return &defaultAllocator{
		buffers: [...]sync.Pool{
			{New: func() any { return new([1 << 6]byte) }},
			{New: func() any { return new([1 << 7]byte) }},
			{New: func() any { return new([1 << 8]byte) }},
			{New: func() any { return new([1 << 9]byte) }},
			{New: func() any { return new([1 << 10]byte) }},
			{New: func() any { return new([1 << 11]byte) }},
			{New: func() any { return new([1 << 12]byte) }},
			{New: func() any { return new([1 << 13]byte) }},
			{New: func() any { return new([1 << 14]byte) }},
			{New: func() any { return new([1 << 15]byte) }},
			{New: func() any { return new([1 << 16]byte) }},
			{New: func() any { return new([MaxPooledBufferSize]byte) }},
		},
	}
}

func (alloc *defaultAllocator) Get(size int) []byte {
	if size <= 0 || size > MaxPooledBufferSize {
		return nil
	}
	var index uint16
	if size > 64 {
		if size > 1<<16 {
			index = 11
		} else {
			index = msb(size)
			if size != 1<<index {
				index += 1
			}
			index -= 6
		}
	}

	buffer := alloc.buffers[index].Get()
	switch index {
	case 0:
		return buffer.(*[1 << 6]byte)[:size]
	case 1:
		return buffer.(*[1 << 7]byte)[:size]
	case 2:
		return buffer.(*[1 << 8]byte)[:size]
	case 3:
		return buffer.(*[1 << 9]byte)[:size]
	case 4:
		return buffer.(*[1 << 10]byte)[:size]
	case 5:
		return buffer.(*[1 << 11]byte)[:size]
	case 6:
		return buffer.(*[1 << 12]byte)[:size]
	case 7:
		return buffer.(*[1 << 13]byte)[:size]
	case 8:
		return buffer.(*[1 << 14]byte)[:size]
	case 9:
		return buffer.(*[1 << 15]byte)[:size]
	case 10:
		return buffer.(*[1 << 16]byte)[:size]
	case 11:
		return buffer.(*[MaxPooledBufferSize]byte)[:size]
	default:
		panic("invalid pool index")
	}
}

func (alloc *defaultAllocator) Put(buf []byte) error {
	index := msb(cap(buf))
	if cap(buf) == MaxPooledBufferSize {
		index = 11
	} else if cap(buf) < 64 || cap(buf) > 65536 || cap(buf) != 1<<index {
		return errors.New("allocator Put() incorrect buffer size")
	} else {
		index -= 6
	}
	buf = buf[:cap(buf)]

	//nolint
	//lint:ignore SA6002 ignore temporarily
	switch index {
	case 0:
		alloc.buffers[index].Put((*[1 << 6]byte)(buf))
	case 1:
		alloc.buffers[index].Put((*[1 << 7]byte)(buf))
	case 2:
		alloc.buffers[index].Put((*[1 << 8]byte)(buf))
	case 3:
		alloc.buffers[index].Put((*[1 << 9]byte)(buf))
	case 4:
		alloc.buffers[index].Put((*[1 << 10]byte)(buf))
	case 5:
		alloc.buffers[index].Put((*[1 << 11]byte)(buf))
	case 6:
		alloc.buffers[index].Put((*[1 << 12]byte)(buf))
	case 7:
		alloc.buffers[index].Put((*[1 << 13]byte)(buf))
	case 8:
		alloc.buffers[index].Put((*[1 << 14]byte)(buf))
	case 9:
		alloc.buffers[index].Put((*[1 << 15]byte)(buf))
	case 10:
		alloc.buffers[index].Put((*[1 << 16]byte)(buf))
	case 11:
		alloc.buffers[11].Put((*[MaxPooledBufferSize]byte)(buf))
	default:
		panic("invalid pool index")
	}
	return nil
}

func msb(size int) uint16 {
	return uint16(bits.Len32(uint32(size)) - 1)
}
