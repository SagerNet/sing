package bufio

import (
	"io"

	"github.com/metacubex/sing/common"
	"github.com/metacubex/sing/common/buf"
	N "github.com/metacubex/sing/common/network"
)

func CopyExtendedOnce(destination io.Writer, source io.Reader, readCounters []N.CountFunc, writeCounters []N.CountFunc) (n int64, err error) {
	options := N.NewReadWaitOptions(source, destination)
	var buffer *buf.Buffer
	calledWaitRead := false
	if readWaiter, isReadWaiter := CreateReadWaiter(source); isReadWaiter {
		if needCopy := readWaiter.InitializeReadWaiter(options); !needCopy || common.LowMemory {
			calledWaitRead = true
			buffer, err = readWaiter.WaitReadBuffer()
		}
	}
	if !calledWaitRead {
		buffer = options.NewBuffer()
		err = NewExtendedReader(source).ReadBuffer(buffer)
		options.PostReturn(buffer)
	}
	if err != nil {
		buffer.Release()
		return
	}

	dataLen := buffer.Len()
	err = NewExtendedWriter(destination).WriteBuffer(buffer)
	if err != nil {
		buffer.Release()
		return
	}
	for _, counter := range readCounters {
		counter(int64(dataLen))
	}
	for _, counter := range writeCounters {
		counter(int64(dataLen))
	}
	n = int64(dataLen)

	return
}
