//go:build !windows

package bufio

import (
	"os"
	"sync"
	"unsafe"

	"github.com/metacubex/sing/common/buf"
	M "github.com/metacubex/sing/common/metadata"

	"golang.org/x/sys/unix"
)

type syscallVectorisedWriterFields struct {
	access    sync.Mutex
	iovecList *[]unix.Iovec
}

func (w *SyscallVectorisedWriter) WriteVectorised(buffers []*buf.Buffer) error {
	w.access.Lock()
	defer w.access.Unlock()
	defer buf.ReleaseMulti(buffers)
	var iovecList []unix.Iovec
	if w.iovecList != nil {
		iovecList = *w.iovecList
	}
	iovecList = iovecList[:0]
	for _, buffer := range buffers {
		if buffer.IsEmpty() {
			continue
		}
		iovecList = append(iovecList, buffer.Iovec(buffer.Len()))
	}
	if len(iovecList) == 0 {
		return os.ErrInvalid
	}
	if w.iovecList == nil {
		w.iovecList = new([]unix.Iovec)
	}
	*w.iovecList = iovecList // cache
	var innerErr unix.Errno
	writeIovecList := iovecList
	err := w.rawConn.Write(func(fd uintptr) (done bool) {
		for {
			var r0 uintptr
			//nolint:staticcheck
			r0, _, innerErr = unix.RawSyscall(unix.SYS_WRITEV, fd, uintptr(unsafe.Pointer(&writeIovecList[0])), uintptr(len(writeIovecList)))
			writeN := int(r0)
			for writeN > 0 && len(writeIovecList) > 0 {
				if int(writeIovecList[0].Len) > writeN {
					writeIovecList[0].Base = (*byte)(unsafe.Add(unsafe.Pointer(writeIovecList[0].Base), writeN))
					writeIovecList[0].SetLen(int(writeIovecList[0].Len) - writeN)
					break
				} else {
					writeN -= int(writeIovecList[0].Len)
					writeIovecList = writeIovecList[1:]
				}
			}
			if innerErr == unix.EINTR || (innerErr == 0 && len(writeIovecList) > 0) {
				continue
			}
			return innerErr != unix.EAGAIN
		}
	})
	if innerErr != 0 {
		err = os.NewSyscallError("SYS_WRITEV", innerErr)
	}
	for index := range iovecList {
		iovecList[index] = unix.Iovec{}
	}
	return err
}

func (w *SyscallVectorisedPacketWriter) WriteVectorisedPacket(buffers []*buf.Buffer, destination M.Socksaddr) error {
	w.access.Lock()
	defer w.access.Unlock()
	defer buf.ReleaseMulti(buffers)
	var iovecList []unix.Iovec
	if w.iovecList != nil {
		iovecList = *w.iovecList
	}
	iovecList = iovecList[:0]
	for _, buffer := range buffers {
		if buffer.IsEmpty() {
			continue
		}
		iovecList = append(iovecList, buffer.Iovec(buffer.Len()))
	}
	if w.iovecList == nil {
		w.iovecList = new([]unix.Iovec)
	}
	*w.iovecList = iovecList // cache
	var innerErr unix.Errno
	err := w.rawConn.Write(func(fd uintptr) (done bool) {
		var msg unix.Msghdr
		name, nameLen := M.AddrPortToRawSockaddr(destination.AddrPort())
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
	if innerErr != 0 {
		err = os.NewSyscallError("SYS_SENDMSG", innerErr)
	}
	for index := range iovecList {
		iovecList[index] = unix.Iovec{}
	}
	return err
}
