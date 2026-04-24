//go:build (linux && !386 && !arm && !mips && !mipsle && !ppc) || netbsd

package bufio

import "golang.org/x/sys/unix"

const sysRecvmmsg = unix.SYS_RECVMMSG
