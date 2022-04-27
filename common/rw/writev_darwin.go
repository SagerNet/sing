package rw

import (
	"syscall"
	"unsafe"
)

func WriteV(fd uintptr, data ...[]byte) (int, error) {
	iovecs := make([]syscall.Iovec, len(data))
	for i := range iovecs {
		iovecs[i] = syscall.Iovec{
			Base: &data[i][0],
			Len:  uint64(len(data[i])),
		}
	}
	var (
		r uintptr
		e syscall.Errno
	)
	for {
		r, _, e = syscall.Syscall(syscall.SYS_WRITEV, fd, uintptr(unsafe.Pointer(&iovecs[0])), uintptr(len(iovecs)))
		if e != syscall.EINTR {
			break
		}
	}
	if e != 0 {
		return 0, e
	}
	return int(r), nil
}
