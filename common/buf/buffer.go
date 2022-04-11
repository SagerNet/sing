package buf

import (
	"crypto/rand"
	"fmt"
	"io"
	"net"

	"github.com/sagernet/sing/common"
)

type Buffer struct {
	data    []byte
	start   int
	end     int
	managed bool
}

func New() *Buffer {
	return &Buffer{
		data:    GetBytes(),
		start:   ReversedHeader,
		end:     ReversedHeader,
		managed: true,
	}
}

func NewSize(size int) *Buffer {
	if size <= 128 || size > BufferSize {
		return &Buffer{
			data: make([]byte, size),
		}
	}
	return &Buffer{
		data:    GetBytes(),
		start:   ReversedHeader,
		end:     ReversedHeader,
		managed: true,
	}
}

func FullNew() *Buffer {
	return &Buffer{
		data:    GetBytes(),
		managed: true,
	}
}

func StackNew() Buffer {
	return Buffer{
		data:    GetBytes(),
		managed: true,
	}
}

func From(data []byte) *Buffer {
	buffer := New()
	buffer.Write(data)
	return buffer
}

func As(data []byte) *Buffer {
	size := len(data)
	max := cap(data)
	if size != max {
		data = data[:max]
	}
	return &Buffer{
		data: data,
		end:  size,
	}
}

func Or(data []byte, size int) *Buffer {
	max := cap(data)
	if size != max {
		data = data[:max]
	}
	if cap(data) >= size {
		return &Buffer{
			data: data,
		}
	} else {
		return NewSize(size)
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
	n = copy(b.data[b.end:], data)
	b.end += n
	return
}

func (b *Buffer) ExtendHeader(size int) []byte {
	if b.start >= size {
		b.start -= size
		return b.data[b.start-size : b.start]
	} else {
		offset := size - b.start
		end := b.end + size
		copy(b.data[offset:end], b.data[b.start:b.end])
		b.end = end
		return b.data[:offset]
	}
}

func (b *Buffer) WriteBufferAtFirst(buffer *Buffer) *Buffer {
	size := buffer.Len()
	if b.start >= size {
		n := copy(b.data[b.start-size:b.start], buffer.Bytes())
		b.start -= n
		buffer.Release()
		return b
	}
	common.Must1(buffer.Write(b.Bytes()))
	b.Release()
	return buffer
}

func (b *Buffer) WriteAtFirst(data []byte) (n int, err error) {
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

func (b *Buffer) Release() {
	if b == nil || b.data == nil || !b.managed {
		return
	}
	PutBytes(b.data)
	*b = Buffer{}
}

func (b Buffer) Len() int {
	return b.end - b.start
}

func (b Buffer) Cap() int {
	return cap(b.data)
}

func (b Buffer) Bytes() []byte {
	return b.data[b.start:b.end]
}

func (b Buffer) Slice() []byte {
	return b.data
}

func (b Buffer) From(n int) []byte {
	return b.data[b.start+n : b.end]
}

func (b Buffer) To(n int) []byte {
	return b.data[b.start : b.start+n]
}

func (b Buffer) Range(start, end int) []byte {
	return b.data[b.start+start : b.start+end]
}

func (b Buffer) Index(start int) []byte {
	return b.data[b.start+start : b.start+start]
}

func (b Buffer) FreeLen() int {
	return b.Cap() - b.end
}

func (b Buffer) FreeBytes() []byte {
	return b.data[b.end:b.Cap()]
}

func (b Buffer) IsEmpty() bool {
	return b.end-b.start == 0
}

func (b Buffer) IsFull() bool {
	return b.end == b.Cap()
}

func (b Buffer) ToOwned() *Buffer {
	var buffer *Buffer
	if b.Len() > BufferSize {
		buffer = As(make([]byte, b.Len()))
		copy(buffer.data, b.Bytes())
	} else {
		buffer = New()
		buffer.Write(b.Bytes())
	}
	return buffer
}

func (b Buffer) Copy() []byte {
	buffer := make([]byte, b.Len())
	copy(buffer, b.Bytes())
	return buffer
}

func ForeachN(b []byte, size int) [][]byte {
	total := len(b)
	var index int
	var retArr [][]byte
	for {
		nextIndex := index + size
		if nextIndex < total {
			retArr = append(retArr, b[index:nextIndex])
			index = nextIndex
		} else {
			retArr = append(retArr, b[index:])
			break
		}
	}
	return retArr
}
