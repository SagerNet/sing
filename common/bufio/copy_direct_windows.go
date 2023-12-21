package bufio

import (
	"io"
	"os"
	"syscall"

	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	N "github.com/sagernet/sing/common/network"

	"golang.org/x/sys/windows"
)

var _ N.ReadWaiter = (*syscallReadWaiter)(nil)

type syscallReadWaiter struct {
	rawConn  syscall.RawConn
	readErr  error
	readFunc func(fd uintptr) (done bool)
	hasData  bool
	buffer   *buf.Buffer
	options  N.ReadWaitOptions
}

func createSyscallReadWaiter(reader any) (*syscallReadWaiter, bool) {
	if syscallConn, isSyscallConn := reader.(syscall.Conn); isSyscallConn {
		rawConn, err := syscallConn.SyscallConn()
		if err == nil {
			return &syscallReadWaiter{rawConn: rawConn}, true
		}
	}
	return nil, false
}

func (w *syscallReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	w.options = options
	w.readFunc = func(fd uintptr) (done bool) {
		if !w.hasData {
			w.hasData = true
			return false
		}
		buffer := w.options.NewBuffer()
		iovecList := []windows.WSABuf{windows.WSABuf{}}
		iovecList[0].Buf = &buffer.FreeBytes()[0]
		iovecList[0].Len = uint32(len(buffer.FreeBytes()))
		var readN uint32
		var flags uint32
		w.readErr = windows.WSARecv(windows.Handle(fd), &iovecList[0], uint32(len(iovecList)), &readN, &flags, nil, nil)
		if readN > 0 {
			buffer.Truncate(int(readN))
			w.options.PostReturn(buffer)
			w.buffer = buffer
		} else {
			buffer.Release()
		}
		if w.readErr == windows.WSAEWOULDBLOCK {
			return false
		}
		if readN == 0 && w.readErr == nil {
			w.readErr = io.EOF
		}
		w.hasData = false
		return true
	}
	return false
}

func (w *syscallReadWaiter) WaitReadBuffer() (buffer *buf.Buffer, err error) {
	if w.readFunc == nil {
		return nil, os.ErrInvalid
	}
	err = w.rawConn.Read(w.readFunc)
	if err != nil {
		return
	}
	if w.readErr != nil {
		if w.readErr == io.EOF {
			return nil, io.EOF
		}
		return nil, E.Cause(w.readErr, "raw read")
	}
	buffer = w.buffer
	w.buffer = nil
	return
}

func createSyscallPacketReadWaiter(reader any) (N.PacketReadWaiter, bool) {
	return nil, false
}
