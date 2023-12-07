package bufio

import (
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	N "github.com/sagernet/sing/common/network"
)

func CopyExtendedOnce(dst N.ExtendedWriter, src N.ExtendedReader) (n int64, err error) {
	options := N.ReadWaitOptions{
		FrontHeadroom: N.CalculateFrontHeadroom(dst),
		RearHeadroom:  N.CalculateRearHeadroom(dst),
		MTU:           N.CalculateMTU(src, dst),
	}
	var buffer *buf.Buffer
	calledWaitRead := false
	if readWaiter, isReadWaiter := CreateReadWaiter(src); isReadWaiter {
		if needCopy := readWaiter.InitializeReadWaiter(options); !needCopy || common.LowMemory {
			calledWaitRead = true
			buffer, err = readWaiter.WaitReadBuffer()
		}
	}
	if !calledWaitRead {
		buffer = options.NewBuffer()
		err = src.ReadBuffer(buffer)
		options.PostReturn(buffer)
	}
	if err != nil {
		buffer.Release()
		return
	}

	dataLen := buffer.Len()
	err = dst.WriteBuffer(buffer)
	if err != nil {
		buffer.Release()
	} else {
		n = int64(dataLen)
	}
	return
}
