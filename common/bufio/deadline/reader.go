package deadline

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/sagernet/sing/common/atomic"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	N "github.com/sagernet/sing/common/network"
)

type TimeoutReader interface {
	io.Reader
	SetReadDeadline(t time.Time) error
}

type Reader struct {
	N.ExtendedReader
	timeoutReader TimeoutReader
	deadline      time.Time
	pipeDeadline  pipeDeadline
	disablePipe   atomic.Bool
	inRead        atomic.Bool
	cacheAccess   sync.RWMutex
	cached        bool
	cachedBuffer  *buf.Buffer
	cachedErr     error
}

func NewReader(reader TimeoutReader) *Reader {
	return &Reader{ExtendedReader: bufio.NewExtendedReader(reader), timeoutReader: reader, pipeDeadline: makePipeDeadline()}
}

func (r *Reader) Read(p []byte) (n int, err error) {
	r.cacheAccess.Lock()
	if r.cached {
		n = copy(p, r.cachedBuffer.Bytes())
		err = r.cachedErr
		r.cachedBuffer.Advance(n)
		if r.cachedBuffer.IsEmpty() {
			r.cachedBuffer.Release()
			r.cached = false
		}
		r.cacheAccess.Unlock()
		return
	}
	r.cacheAccess.Unlock()
	if r.disablePipe.Load() {
		return r.ExtendedReader.Read(p)
	} else if r.deadline.IsZero() {
		r.inRead.Store(true)
		defer r.inRead.Store(false)
		n, err = r.ExtendedReader.Read(p)
		return
	}
	done := make(chan struct{})
	var access sync.Mutex
	var cancel bool
	go func() {
		n, err = r.pipeRead(p, &access, &cancel, done)
	}()
	select {
	case <-done:
		return
	case <-r.pipeDeadline.wait():
	}
	access.Lock()
	defer access.Unlock()
	select {
	case <-done:
		return
	default:
	}
	cancel = true
	return 0, os.ErrDeadlineExceeded
}

func (r *Reader) pipeRead(p []byte, access *sync.Mutex, cancel *bool, done chan struct{}) (n int, err error) {
	r.cacheAccess.Lock()
	defer r.cacheAccess.Unlock()
	buffer := buf.NewSize(len(p))
	n, err = buffer.ReadOnceFrom(r.ExtendedReader)
	access.Lock()
	defer access.Unlock()
	if *cancel {
		r.cached = true
		r.cachedBuffer = buffer
		r.cachedErr = err
	} else {
		n = copy(p, buffer.Bytes())
		buffer.Release()
	}
	close(done)
	return
}

func (r *Reader) ReadBuffer(buffer *buf.Buffer) error {
	r.cacheAccess.Lock()
	if r.cached {
		buffer.Resize(r.cachedBuffer.Start(), 0)
		n := copy(buffer.FreeBytes(), r.cachedBuffer.Bytes())
		err := r.cachedErr
		buffer.Truncate(n)
		r.cachedBuffer.Advance(n)
		if r.cachedBuffer.IsEmpty() {
			r.cachedBuffer.Release()
			r.cached = false
		}
		r.cacheAccess.Unlock()
		return err
	}
	r.cacheAccess.Unlock()
	if r.disablePipe.Load() {
		return r.ExtendedReader.ReadBuffer(buffer)
	} else if r.deadline.IsZero() {
		r.inRead.Store(true)
		defer r.inRead.Store(false)
		return r.ExtendedReader.ReadBuffer(buffer)
	}
	done := make(chan struct{})
	var access sync.Mutex
	var cancel bool
	var err error
	go func() {
		err = r.pipeReadBuffer(buffer, &access, &cancel, done)
	}()
	select {
	case <-done:
		return err
	case <-r.pipeDeadline.wait():
	}
	access.Lock()
	defer access.Unlock()
	select {
	case <-done:
		return err
	default:
	}
	cancel = true
	return os.ErrDeadlineExceeded
}

func (r *Reader) pipeReadBuffer(buffer *buf.Buffer, access *sync.Mutex, cancel *bool, done chan struct{}) error {
	r.cacheAccess.Lock()
	defer r.cacheAccess.Unlock()
	cacheBuffer := buf.NewSize(buffer.Cap())
	cacheBuffer.Resize(buffer.Start(), 0)
	err := r.ExtendedReader.ReadBuffer(cacheBuffer)
	access.Lock()
	defer access.Unlock()
	if *cancel {
		r.cached = true
		r.cachedBuffer = cacheBuffer
		r.cachedErr = err
	} else {
		buffer.Resize(cacheBuffer.Start(), 0)
		buffer.ReadOnceFrom(cacheBuffer)
		cacheBuffer.Release()
	}
	close(done)
	return err
}

func (r *Reader) SetReadDeadline(t time.Time) error {
	if r.disablePipe.Load() {
		return r.timeoutReader.SetReadDeadline(t)
	} else if r.inRead.Load() {
		r.disablePipe.Store(true)
		return r.timeoutReader.SetReadDeadline(t)
	}
	r.deadline = t
	r.pipeDeadline.set(t)
	return nil
}

func (r *Reader) ReaderReplaceable() bool {
	r.cacheAccess.RLock()
	if r.cached {
		r.cacheAccess.RUnlock()
		return false
	}
	r.cacheAccess.RUnlock()
	return r.disablePipe.Load() || r.deadline.IsZero()
}

func (r *Reader) UpstreamReader() any {
	return r.ExtendedReader
}
