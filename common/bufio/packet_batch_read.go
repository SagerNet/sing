//go:build linux || netbsd || darwin

package bufio

import (
	"io"
	"syscall"
	"unsafe"

	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"

	"golang.org/x/sys/unix"
)

func (w *syscallPacketBatchReadWaiter) WaitReadPackets() (buffers []*buf.Buffer, destinations []M.Socksaddr, err error) {
	err = w.read()
	if err != nil {
		return
	}
	buffers = make([]*buf.Buffer, w.readN)
	destinations = make([]M.Socksaddr, w.readN)
	for i := 0; i < w.readN; i++ {
		buffers[i] = w.buffers[i]
		w.buffers[i] = nil
		destinations[i] = w.destinations[i]
	}
	w.readN = 0
	return
}

func (w *syscallPacketBatchReadWaiter) WaitReadConnectedPackets() (buffers []*buf.Buffer, destination M.Socksaddr, err error) {
	err = w.read()
	if err != nil {
		return
	}
	buffers = make([]*buf.Buffer, w.readN)
	for i := 0; i < w.readN; i++ {
		buffers[i] = w.buffers[i]
		w.buffers[i] = nil
	}
	w.readN = 0
	destination = w.destination
	return
}

func (w *syscallPacketBatchReadWaiter) read() error {
	w.readErr = nil
	w.probe = w.released
	err := w.rawConn.Read(w.readFunc)
	if err == nil && w.readErr != nil {
		if w.readErr == io.EOF {
			err = io.EOF
		} else {
			err = E.Cause(w.readErr, "raw read")
		}
	}
	if err != nil {
		w.releaseBuffers()
		return err
	}
	clear(w.iovecs)
	clear(w.msgvec)
	return nil
}

func (w *syscallPacketBatchReadWaiter) releaseBuffers() {
	clear(w.iovecs)
	clear(w.msgvec)
	buf.ReleaseMulti(w.buffers)
	clear(w.buffers)
	w.readN = 0
	w.released = true
}

// XNU returns 0 for a zero-length recvfrom on an empty socket (observed on
// macOS 27).
func peekPacket(fd int, buffer *[1]byte) syscall.Errno {
	for {
		//nolint:staticcheck
		_, _, errno := unix.Syscall6(unix.SYS_RECVFROM, uintptr(fd), uintptr(unsafe.Pointer(&buffer[0])), 1, unix.MSG_PEEK, 0, 0)
		if errno != syscall.EINTR {
			return errno
		}
	}
}
