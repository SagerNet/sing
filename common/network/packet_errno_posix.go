//go:build !windows

package network

import "syscall"

const ErrPacketTooBig = syscall.EMSGSIZE
