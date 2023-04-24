package bufio

import (
	"io"
	"syscall"

	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
)

func CopyDirect(conn syscall.Conn, remoteConn syscall.Conn, readCounters []CountFunc, writeCounters []CountFunc) (handed bool, n int64, err error) {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return
	}
	rawRemoteConn, err := remoteConn.SyscallConn()
	if err != nil {
		return
	}
	handed, n, err = splice(rawConn, rawRemoteConn, readCounters, writeCounters)
	if handed {
		return
	}
	handed = true
	n, err = copySyscallWithPool(rawConn, rawRemoteConn, readCounters, writeCounters)
	return
}

func copySyscallWithPool(conn syscall.RawConn, remoteConn syscall.RawConn, readCounters []CountFunc, writeCounters []CountFunc) (n int64, err error) {
	var buffer *buf.Buffer
	var readN int
	var readErr error
	var writeErr error
	readFunc := func(fd uintptr) (done bool) {
		buffer = buf.New()
		buffer.FullReset()
		readN, readErr = syscall.Read(int(fd), buffer.FreeBytes())
		if readN > 0 {
			buffer.Truncate(readN)
		} else {
			buffer.Release()
			buffer = nil
		}
		if readErr == syscall.EAGAIN {
			return false
		}
		if readN == 0 {
			readErr = io.EOF
		}
		return true
	}
	writeFunc := func(fd uintptr) (done bool) {
		for !buffer.IsEmpty() {
			var writeN int
			writeN, writeErr = syscall.Write(int(fd), buffer.Bytes())
			if writeErr != nil {
				return writeErr != syscall.EAGAIN
			}
			buffer.Advance(writeN)
		}
		return true
	}
	for {
		err = conn.Read(readFunc)
		if err != nil {
			return 0, E.Cause(err, "read")
		}
		if readErr != nil {
			return 0, E.Cause(err, "raw read")
		}
		err = remoteConn.Write(writeFunc)
		buffer.Release()
		if err != nil {
			return 0, E.Cause(err, "write")
		}
		if writeErr != nil {
			return 0, E.Cause(writeErr, "raw write")
		}
		for _, readCounter := range readCounters {
			readCounter(int64(readN))
		}
		for _, writeCounter := range writeCounters {
			writeCounter(int64(readN))
		}
	}
}
