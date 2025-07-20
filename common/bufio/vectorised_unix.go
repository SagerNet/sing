//go:build !windows

package bufio

import (
	"os"
	"sync"
	"unsafe"

	"github.com/sagernet/sing/common"
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
		iovecList = append(iovecList, buffer.Iovec(buffer.Len()))
	}
	var innerErr unix.Errno
	err := w.rawConn.Write(func(fd uintptr) (done bool) {
		for {
			var r0 uintptr
			//nolint:staticcheck
			r0, _, innerErr = unix.RawSyscall(unix.SYS_WRITEV, fd, uintptr(unsafe.Pointer(&iovecList[0])), uintptr(len(iovecList)))
			writeN := int(r0)
			for writeN > 0 {
				if buffers[0].Len() > writeN {
					buffers[0].Advance(writeN)
					iovecList[0] = buffers[0].Iovec(buffers[0].Len())
					break
				} else {
					writeN -= buffers[0].Len()
					buffers[0].Release()
					buffers = buffers[1:]
					iovecList = iovecList[1:]
				}
			}
			if innerErr == unix.EINTR || (innerErr == 0 && len(iovecList) > 0) {
				continue
			}
			return innerErr != unix.EAGAIN
		}
	})
	common.ClearArray(iovecList)
	if cap(iovecList) > cap(w.iovecList) {
		w.iovecList = iovecList[:0]
	}
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
		iovecList = append(iovecList, buffer.Iovec(buffer.Len()))
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
		for {
			_, _, innerErr = unix.RawSyscall(unix.SYS_SENDMSG, fd, uintptr(unsafe.Pointer(&msg)), 0)
			if innerErr == unix.EINTR {
				continue
			}
			return innerErr != unix.EAGAIN
		}
	})
	common.ClearArray(iovecList)
	if cap(iovecList) > cap(w.iovecList) {
		w.iovecList = iovecList[:0]
	}
	if innerErr != nil {
		err = innerErr
	}
	return err
}
