//go:build !linux

package bufio

import "syscall"

func splice(conn syscall.RawConn, remoteConn syscall.RawConn, readCounters []CountFunc, writeCounters []CountFunc) (handed bool, n int64, err error) {
	return
}
