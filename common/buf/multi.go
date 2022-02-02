package buf

type MultiBuffer struct {
	buffers []*Buffer
	index   int
}

func (b MultiBuffer) Size() int {
	return len(b.buffers)
}

func (b MultiBuffer) Len() int {
	var length int
	for _, buffer := range b.buffers {
		length += buffer.Len()
	}
	return length
}

func (b *MultiBuffer) Release() {
	for _, buffer := range b.buffers {
		buffer.Release()
	}
	b.buffers = nil
	b.index = 0
}

func (b MultiBuffer) From(n int) MultiBuffer {
	var newBuffer MultiBuffer
	for _, buffer := range b.buffers {
		if n == 0 {
			newBuffer.buffers = append(newBuffer.buffers, buffer)
		} else if buffer.Len() < n {
			n -= buffer.Len()
		} else {
			newBuffer.buffers = append(newBuffer.buffers, As(buffer.From(n)))
			n = 0
		}
	}
	return newBuffer
}

func (b *MultiBuffer) BufferForWrite() *Buffer {
	var buffer *Buffer
	if b.Size() > 0 && !b.buffers[b.index].IsFull() {
		buffer = b.buffers[b.index]
	} else {
		buffer = New()
		b.buffers = append(b.buffers, buffer)
		b.index++
	}
	return buffer
}

func (b *MultiBuffer) Write(data []byte) (n int, err error) {
	size := len(data)
	var wn int
	for wn < size {
		n, err = b.BufferForWrite().Write(data)
		if err != nil {
			return 0, err
		}
		wn += n
	}
	return wn, nil
}

func (b *MultiBuffer) WriteAtFirst(data []byte) (n int, err error) {
	length := len(data)
	if b.Size() > 0 {
		buffer := b.buffers[0]
		if buffer.start > 0 {
			n = copy(buffer.data[:buffer.start], data[length-buffer.start:length])
			buffer.start -= n
		}
	}
	if n < length {
		b.buffers = append([]*Buffer{As(data[n:length])}, b.buffers...)
		b.index++
	}
	return
}

func (b *MultiBuffer) WriteMulti(data *MultiBuffer) (n int, err error) {
	defer data.Release()
	for _, buffer := range data.buffers {
		writeN, err := b.Write(buffer.Bytes())
		if err != nil {
			return 0, err
		}
		n += writeN
	}
	return
}

func (b *MultiBuffer) WriteString(str string) (n int, err error) {
	return b.Write([]byte(str))
}
