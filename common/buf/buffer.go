package buf

import (
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync/atomic"

	"github.com/sagernet/sing/common"
)

const (
	ReversedHeader = 1024
	BufferSize     = 20 * 1024
	UDPBufferSize  = 16 * 1024
)

type Buffer struct {
	data    []byte
	start   int
	end     int
	managed bool
	refs    int32
}

func New() *Buffer {
	return &Buffer{
		data:    Get(BufferSize),
		start:   ReversedHeader,
		end:     ReversedHeader,
		managed: true,
	}
}

func NewPacket() *Buffer {
	return &Buffer{
		data:    Get(UDPBufferSize),
		start:   ReversedHeader,
		end:     ReversedHeader,
		managed: true,
	}
}

func NewSize(size int) *Buffer {
	return &Buffer{
		data:    Get(size),
		managed: true,
	}
}

func StackNew() *Buffer {
	if common.Unsafe {
		return &Buffer{
			data:  make([]byte, BufferSize),
			start: ReversedHeader,
			end:   ReversedHeader,
		}
	} else {
		return New()
	}
}

func StackNewPacket() *Buffer {
	if common.Unsafe {
		return &Buffer{
			data:  make([]byte, UDPBufferSize),
			start: ReversedHeader,
			end:   ReversedHeader,
		}
	} else {
		return NewPacket()
	}
}

func StackNewSize(size int) *Buffer {
	if common.Unsafe {
		return &Buffer{
			data: Make(size),
		}
	} else {
		return NewSize(size)
	}
}

func As(data []byte) *Buffer {
	return &Buffer{
		data: data,
		end:  len(data),
	}
}

func With(data []byte) *Buffer {
	return &Buffer{
		data: data,
	}
}

func (b *Buffer) Byte(index int) byte {
	return b.data[b.start+index]
}

func (b *Buffer) SetByte(index int, value byte) {
	b.data[b.start+index] = value
}

func (b *Buffer) Extend(n int) []byte {
	end := b.end + n
	if end > cap(b.data) {
		panic("buffer overflow: cap " + strconv.Itoa(cap(b.data)) + ",end " + strconv.Itoa(b.end) + ", need " + strconv.Itoa(n))
	}
	ext := b.data[b.end:end]
	b.end = end
	return ext
}

func (b *Buffer) Advance(from int) {
	b.start += from
}

func (b *Buffer) Truncate(to int) {
	b.end = b.start + to
}

func (b *Buffer) Write(data []byte) (n int, err error) {
	if b.IsFull() {
		return 0, io.ErrShortBuffer
	}
	if b.end+len(data) > b.Cap() {
		panic("buffer overflow: cap " + strconv.Itoa(len(b.data)) + ",end " + strconv.Itoa(b.end) + ", need " + strconv.Itoa(len(data)))
	}
	n = copy(b.data[b.end:], data)
	b.end += n
	return
}

func (b *Buffer) ExtendHeader(n int) []byte {
	if b.start < n {
		panic("buffer overflow: cap " + strconv.Itoa(cap(b.data)) + ",start " + strconv.Itoa(b.start) + ", need " + strconv.Itoa(n))
	}
	b.start -= n
	return b.data[b.start : b.start+n]
}

func (b *Buffer) _WriteBufferAtFirst(buffer *Buffer) *Buffer {
	size := buffer.Len()
	if b.start >= size {
		n := copy(b.data[b.start-size:b.start], buffer.Bytes())
		b.start -= n
		buffer.Release()
		return b
	} else if buffer.FreeLen() >= b.Len() {
		common.Must1(buffer.Write(b.Bytes()))
		b.Release()
		return buffer
	} else if b.FreeLen() >= size {
		copy(b.data[b.start+size:b.end+size], b.data[b.start:b.end])
		copy(b.data, buffer.data)
		buffer.Release()
		return b
	} else {
		panic("buffer overflow")
	}
}

func (b *Buffer) _WriteAtFirst(data []byte) (n int, err error) {
	size := len(data)
	if b.start >= size {
		n = copy(b.data[b.start-size:b.start], data)
		b.start -= n
	} else {
		copy(b.data[size:], b.data[b.start:b.end])
		n = copy(b.data[:size], data)
		b.end += size - b.start
		b.start = 0
	}
	return
}

func (b *Buffer) WriteRandom(size int) {
	common.Must1(io.ReadFull(rand.Reader, b.Extend(size)))
}

func (b *Buffer) WriteByte(byte byte) error {
	if b.IsFull() {
		return io.ErrShortBuffer
	}
	b.data[b.end] = byte
	b.end++
	return nil
}

func (b *Buffer) ReadFrom(r io.Reader) (int64, error) {
	if b.IsFull() {
		return 0, io.ErrShortBuffer
	}
	n, err := r.Read(b.FreeBytes())
	if err != nil {
		return 0, err
	}
	b.end += n
	return int64(n), nil
}

func (b *Buffer) ReadPacketFrom(r net.PacketConn) (int64, net.Addr, error) {
	if b.IsFull() {
		return 0, nil, io.ErrShortBuffer
	}
	n, addr, err := r.ReadFrom(b.FreeBytes())
	if err != nil {
		return 0, nil, err
	}
	b.end += n
	return int64(n), addr, nil
}

func (b *Buffer) ReadAtLeastFrom(r io.Reader, min int) (int64, error) {
	if min <= 0 {
		return b.ReadFrom(r)
	}
	if b.IsFull() {
		return 0, io.ErrShortBuffer
	}
	n, err := io.ReadAtLeast(r, b.FreeBytes(), min)
	if err != nil {
		return 0, err
	}
	b.end += n
	return int64(n), nil
}

func (b *Buffer) ReadFullFrom(r io.Reader, size int) (n int, err error) {
	if b.IsFull() {
		return 0, io.ErrShortBuffer
	}
	end := b.end + size
	n, err = io.ReadFull(r, b.data[b.start:end])
	if err != nil {
		return 0, err
	}
	b.end += n
	return
}

func (b *Buffer) WriteRune(s rune) (int, error) {
	return b.Write([]byte{byte(s)})
}

func (b *Buffer) WriteString(s string) (int, error) {
	return b.Write([]byte(s))
}

func (b *Buffer) WriteSprint(s ...any) (int, error) {
	return b.WriteString(fmt.Sprint(s...))
}

func (b *Buffer) WriteZero() error {
	if b.IsFull() {
		return io.ErrShortBuffer
	}
	b.end++
	b.data[b.end] = 0
	return nil
}

func (b *Buffer) WriteZeroN(n int) error {
	if b.end+n > b.Cap() {
		return io.ErrShortBuffer
	}
	for i := b.end; i <= b.end+n; i++ {
		b.data[i] = 0
	}
	b.end += n
	return nil
}

func (b *Buffer) ReadByte() (byte, error) {
	if b.IsEmpty() {
		return 0, io.EOF
	}

	nb := b.data[b.start]
	b.start++
	return nb, nil
}

func (b *Buffer) ReadBytes(n int) ([]byte, error) {
	if b.end-b.start < n {
		return nil, io.EOF
	}

	nb := b.data[b.start : b.start+n]
	b.start += n
	return nb, nil
}

func (b *Buffer) Read(data []byte) (n int, err error) {
	if b.Len() == 0 {
		return 0, io.EOF
	}
	n = copy(data, b.data[b.start:b.end])
	if n == b.Len() {
		b.Reset()
	} else {
		b.start += n
	}
	return n, nil
}

func (b *Buffer) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(b.Bytes())
	return int64(n), err
}

func (b *Buffer) Resize(start, end int) {
	b.start = start
	b.end = b.start + end
}

func (b *Buffer) Reset() {
	b.start = ReversedHeader
	b.end = ReversedHeader
}

func (b *Buffer) FullReset() {
	b.start = 0
	b.end = 0
}

func (b *Buffer) IncRef() {
	atomic.AddInt32(&b.refs, 1)
}

func (b *Buffer) DecRef() {
	if atomic.AddInt32(&b.refs, -1) == 0 {
		b.Release()
	}
}

func (b *Buffer) Release() {
	if b == nil || b.data == nil || !b.managed {
		return
	}
	if atomic.LoadInt32(&b.refs) > 0 {
		return
	}
	common.Must(Put(b.data))
	*b = Buffer{}
}

func (b *Buffer) Cut(start int, end int) *Buffer {
	b.start += start
	b.end = len(b.data) - end
	return &Buffer{
		data: b.data[b.start:b.end],
	}
}

func (b *Buffer) Start() int {
	return b.start
}

func (b *Buffer) Len() int {
	return b.end - b.start
}

func (b *Buffer) Cap() int {
	return len(b.data)
}

func (b *Buffer) Bytes() []byte {
	return b.data[b.start:b.end]
}

func (b *Buffer) Slice() []byte {
	return b.data
}

func (b *Buffer) From(n int) []byte {
	return b.data[b.start+n : b.end]
}

func (b *Buffer) To(n int) []byte {
	return b.data[b.start : b.start+n]
}

func (b *Buffer) Range(start, end int) []byte {
	return b.data[b.start+start : b.start+end]
}

func (b *Buffer) Index(start int) []byte {
	return b.data[b.start+start : b.start+start]
}

func (b *Buffer) FreeLen() int {
	return b.Cap() - b.end
}

func (b *Buffer) FreeBytes() []byte {
	return b.data[b.end:b.Cap()]
}

func (b *Buffer) IsEmpty() bool {
	return b.end-b.start == 0
}

func (b *Buffer) IsFull() bool {
	return b.end == b.Cap()
}
