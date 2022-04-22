//go:build !windows

package rw

import (
	"golang.org/x/sys/unix"
)

func WriteV(fd uintptr, data ...[]byte) (int, error) {
	return unix.Writev(int(fd), data)
}
