package bufio

import (
	"net/netip"
	"syscall"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
)

type ReadWaiter interface {
	WaitReadBuffer(newBuffer func() *buf.Buffer) error
}

type PacketReadWaiter interface {
	WaitReadPacket(newBuffer func() *buf.Buffer) (destination M.Socksaddr, err error)
}

type ReadWaiterCreator interface {
	CreateReadWaiter() (ReadWaiter, bool)
}

type PacketReadWaiterCreator interface {
	CreatePacketReadWaiter() (PacketReadWaiter, bool)
}

func CreateReadWaiter(conn any) (ReadWaiter, bool) {
	if waiter, loaded := common.Cast[ReadWaiterCreator](conn); loaded {
		return waiter.CreateReadWaiter()
	}
	if waiter, loaded := common.Cast[ReadWaiter](conn); loaded {
		return waiter, true
	}
	if rawConn, loaded := common.Cast[syscall.RawConn](conn); loaded {
		return &syscallReadWaiter{rawConn}, true
	}
	if syscallConn, loaded := common.Cast[syscall.Conn](conn); loaded {
		rawConn, err := syscallConn.SyscallConn()
		if err != nil {
			return nil, false
		}
		return &syscallReadWaiter{rawConn}, true
	}
	return nil, false
}

func CreatePacketReadWaiter(conn any) (PacketReadWaiter, bool) {
	if waiter, loaded := common.Cast[PacketReadWaiterCreator](conn); loaded {
		return waiter.CreatePacketReadWaiter()
	}
	if waiter, loaded := common.Cast[PacketReadWaiter](conn); loaded {
		return waiter, true
	}
	if rawConn, loaded := common.Cast[syscall.RawConn](conn); loaded {
		return &syscallReadWaiter{rawConn}, true
	}
	if syscallConn, loaded := common.Cast[syscall.Conn](conn); loaded {
		rawConn, err := syscallConn.SyscallConn()
		if err != nil {
			return nil, false
		}
		return &syscallReadWaiter{rawConn}, true
	}
	return nil, false
}

type syscallReadWaiter struct {
	syscall.RawConn
}

func (w *syscallReadWaiter) WaitReadBuffer(newBuffer func() *buf.Buffer) error {
	var (
		buffer *buf.Buffer
		n      int
		err    error
	)
	err = w.RawConn.Read(func(fd uintptr) (done bool) {
		buffer = newBuffer()
		n, err = syscall.Read(int(fd), buffer.FreeBytes())
		if err == syscall.EAGAIN {
			buffer.Release()
			return false
		}
		buffer.Truncate(n)
		return true
	})
	return err
}

func (w *syscallReadWaiter) WaitReadPacket(newBuffer func() *buf.Buffer) (destination M.Socksaddr, err error) {
	var (
		buffer *buf.Buffer
		n      int
		from   syscall.Sockaddr
	)
	err = w.RawConn.Read(func(fd uintptr) (done bool) {
		buffer = newBuffer()
		n, _, _, from, err = syscall.Recvmsg(int(fd), buffer.FreeBytes(), nil, 0)
		if err == syscall.EAGAIN {
			buffer.Release()
			return false
		}
		buffer.Truncate(n)
		return true
	})
	switch fromAddr := from.(type) {
	case *syscall.SockaddrInet4:
		destination = M.SocksaddrFrom(netip.AddrFrom4(fromAddr.Addr), uint16(fromAddr.Port))
	case *syscall.SockaddrInet6:
		destination = M.SocksaddrFrom(netip.AddrFrom16(fromAddr.Addr), uint16(fromAddr.Port))
	}
	return destination, err
}
