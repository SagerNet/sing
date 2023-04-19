package bufio

import (
	"os"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
)

type bufferedReadWaiter struct {
	*BufferedReader
	upstream ReadWaiter
}

func (w *bufferedReadWaiter) WaitReadBuffer(newBuffer func() *buf.Buffer) error {
	if w.buffer == nil {
		return w.upstream.WaitReadBuffer(newBuffer)
	}
	if w.buffer.Closed() {
		return os.ErrClosed
	}
	var err error
	if w.buffer.IsEmpty() {
		w.buffer.Reset()
		w.buffer.IncRef()
		err = w.upstream.WaitReadBuffer(func() *buf.Buffer {
			return w.buffer
		})
		w.buffer.DecRef()
		if err != nil {
			w.buffer.Release()
			return err
		}
	}
	buffer := newBuffer()
	if w.buffer.Len() > buffer.FreeLen() {
		err = common.Error(buffer.ReadFullFrom(w.buffer, buffer.FreeLen()))
	} else {
		err = common.Error(buffer.ReadFullFrom(w.buffer, w.buffer.Len()))
	}
	if err != nil {
		w.buffer.Release()
	}
	return err
}
