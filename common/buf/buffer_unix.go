//go:build !windows

package buf

import "golang.org/x/sys/unix"

func (b *Buffer) Iovec() unix.Iovec {
	var iov unix.Iovec
	iov.Base = &b.data[b.start]
	iov.SetLen(b.capacity)
	return iov
}
