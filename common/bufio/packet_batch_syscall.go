//go:build linux || netbsd || darwin

package bufio

import (
	"io"
	"syscall"

	"github.com/sagernet/sing/common/control"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"golang.org/x/sys/unix"
)

func syscallPacketBatchRawConnForRead(reader any) syscall.RawConn {
	var rawConn syscall.RawConn
	if syscallConn, isSyscallConn := reader.(syscall.Conn); isSyscallConn {
		rawConn, _ = syscallConn.SyscallConn()
	}
	if rawConn == nil {
		if ioReader, isReader := reader.(io.Reader); isReader {
			_, rawConn = N.SyscallConnForRead(ioReader)
		}
	}
	return rawConn
}

func syscallPacketBatchRawConnForWrite(writer any) syscall.RawConn {
	var rawConn syscall.RawConn
	if syscallConn, isSyscallConn := writer.(syscall.Conn); isSyscallConn {
		rawConn, _ = syscallConn.SyscallConn()
	}
	if rawConn == nil {
		if ioWriter, isWriter := writer.(io.Writer); isWriter {
			_, rawConn = N.SyscallConnForWrite(ioWriter)
		}
	}
	return rawConn
}

func syscallPacketBatchPeerDestination(rawConn syscall.RawConn) (M.Socksaddr, bool) {
	var destination M.Socksaddr
	err := control.Raw(rawConn, func(fd uintptr) error {
		peer, err := unix.Getpeername(int(fd))
		if err != nil {
			return err
		}
		destination = M.SocksaddrFromNetIP(M.AddrPortFromSockaddr(peer)).Unwrap()
		return nil
	})
	return destination, err == nil && destination.IsValid()
}
