//go:build !windows

package bufio

import (
	"os"
	"sync"
	"unsafe"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"

	"golang.org/x/sys/unix"
)

type syscallVectorisedWriterFields struct {
	access    sync.Mutex
	iovecList []unix.Iovec
}

func (w *SyscallVectorisedWriter) WriteVectorised(buffers []*buf.Buffer) error {
	w.access.Lock()
	defer w.access.Unlock()
	defer buf.ReleaseMulti(buffers)
	iovecList := w.iovecList
	for _, buffer := range buffers {
		iovecList = append(iovecList, buffer.Iovec())
	}
	var innerErr unix.Errno
	err := w.rawConn.Write(func(fd uintptr) (done bool) {
		//nolint:staticcheck
		_, _, innerErr = unix.Syscall(unix.SYS_WRITEV, fd, uintptr(unsafe.Pointer(&iovecList[0])), uintptr(len(iovecList)))
		return innerErr != unix.EAGAIN && innerErr != unix.EWOULDBLOCK
	})
	w.iovecList = iovecList[:0]
	if innerErr != 0 {
		err = os.NewSyscallError("SYS_WRITEV", innerErr)
	}
	return err
}

func (w *SyscallVectorisedPacketWriter) WriteVectorisedPacket(buffers []*buf.Buffer, destination M.Socksaddr) error {
	w.access.Lock()
	defer w.access.Unlock()
	defer buf.ReleaseMulti(buffers)
	iovecList := w.iovecList
	for _, buffer := range buffers {
		iovecList = append(iovecList, buffer.Iovec())
	}
	var innerErr error
	err := w.rawConn.Write(func(fd uintptr) (done bool) {
		var msg unix.Msghdr
		name, nameLen := ToSockaddr(destination.AddrPort())
		msg.Name = (*byte)(name)
		msg.Namelen = nameLen
		if len(iovecList) > 0 {
			msg.Iov = &iovecList[0]
			msg.SetIovlen(len(iovecList))
		}
		_, innerErr = sendmsg(int(fd), &msg, 0)
		return innerErr != unix.EAGAIN && innerErr != unix.EWOULDBLOCK
	})
	w.iovecList = iovecList[:0]
	if innerErr != nil {
		err = innerErr
	}
	return err
}

//go:linkname sendmsg golang.org/x/sys/unix.sendmsg
func sendmsg(s int, msg *unix.Msghdr, flags int) (n int, err error)
