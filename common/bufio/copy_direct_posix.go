//go:build !windows

package bufio

import (
	"errors"
	"io"
	"net/netip"
	"os"
	"syscall"

	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func copyWaitWithPool(originSource io.Reader, destination N.ExtendedWriter, source N.ReadWaiter, readCounters []N.CountFunc, writeCounters []N.CountFunc) (handled bool, n int64, err error) {
	handled = true
	var (
		buffer       *buf.Buffer
		notFirstTime bool
	)
	for {
		buffer, err = source.WaitReadBuffer()
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
				return
			}
			return
		}
		dataLen := buffer.Len()
		err = destination.WriteBuffer(buffer)
		if err != nil {
			buffer.Leak()
			if !notFirstTime {
				err = N.ReportHandshakeFailure(originSource, err)
			}
			return
		}
		n += int64(dataLen)
		for _, counter := range readCounters {
			counter(int64(dataLen))
		}
		for _, counter := range writeCounters {
			counter(int64(dataLen))
		}
		notFirstTime = true
	}
}

func copyPacketWaitWithPool(originSource N.PacketReader, destinationConn N.PacketWriter, source N.PacketReadWaiter, readCounters []N.CountFunc, writeCounters []N.CountFunc, notFirstTime bool) (handled bool, n int64, err error) {
	handled = true
	var (
		buffer      *buf.Buffer
		destination M.Socksaddr
	)
	for {
		buffer, destination, err = source.WaitReadPacket()
		if err != nil {
			return
		}
		dataLen := buffer.Len()
		err = destinationConn.WritePacket(buffer, destination)
		if err != nil {
			buffer.Leak()
			if !notFirstTime {
				err = N.ReportHandshakeFailure(originSource, err)
			}
			return
		}
		n += int64(dataLen)
		for _, counter := range readCounters {
			counter(int64(dataLen))
		}
		for _, counter := range writeCounters {
			counter(int64(dataLen))
		}
		notFirstTime = true
	}
}

var _ N.ReadWaiter = (*syscallReadWaiter)(nil)

type syscallReadWaiter struct {
	rawConn  syscall.RawConn
	readErr  error
	readFunc func(fd uintptr) (done bool)
	buffer   *buf.Buffer
	options  N.ReadWaitOptions
}

func createSyscallReadWaiter(reader any) (*syscallReadWaiter, bool) {
	if syscallConn, isSyscallConn := reader.(syscall.Conn); isSyscallConn {
		rawConn, err := syscallConn.SyscallConn()
		if err == nil {
			return &syscallReadWaiter{rawConn: rawConn}, true
		}
	}
	return nil, false
}

func (w *syscallReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	w.options = options
	w.readFunc = func(fd uintptr) (done bool) {
		buffer, readBuffer := w.options.NewBuffer()
		var readN int
		readN, w.readErr = syscall.Read(int(fd), readBuffer.FreeBytes())
		if readN > 0 {
			buffer.Resize(readBuffer.Start(), readN)
		} else {
			buffer.Release()
			buffer = nil
		}
		if w.readErr == syscall.EAGAIN {
			return false
		}
		if readN == 0 {
			w.readErr = io.EOF
		}
		w.buffer = buffer
		return true
	}
	return false
}

func (w *syscallReadWaiter) WaitReadBuffer() (buffer *buf.Buffer, err error) {
	if w.readFunc == nil {
		return nil, os.ErrInvalid
	}
	err = w.rawConn.Read(w.readFunc)
	if err != nil {
		return
	}
	if w.readErr != nil {
		if w.readErr == io.EOF {
			return nil, io.EOF
		}
		return nil, E.Cause(w.readErr, "raw read")
	}
	buffer = w.buffer
	w.buffer = nil
	return
}

var _ N.PacketReadWaiter = (*syscallPacketReadWaiter)(nil)

type syscallPacketReadWaiter struct {
	rawConn  syscall.RawConn
	readErr  error
	readFrom M.Socksaddr
	readFunc func(fd uintptr) (done bool)
	buffer   *buf.Buffer
	options  N.ReadWaitOptions
}

func createSyscallPacketReadWaiter(reader any) (*syscallPacketReadWaiter, bool) {
	if syscallConn, isSyscallConn := reader.(syscall.Conn); isSyscallConn {
		rawConn, err := syscallConn.SyscallConn()
		if err == nil {
			return &syscallPacketReadWaiter{rawConn: rawConn}, true
		}
	}
	return nil, false
}

func (w *syscallPacketReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	w.options = options
	w.readFunc = func(fd uintptr) (done bool) {
		buffer, readBuffer := w.options.NewPacketBuffer()
		var readN int
		var from syscall.Sockaddr
		readN, _, _, from, w.readErr = syscall.Recvmsg(int(fd), readBuffer.FreeBytes(), nil, 0)
		if readN > 0 {
			buffer.Resize(readBuffer.Start(), readN)
		} else {
			buffer.Release()
			buffer = nil
		}
		if w.readErr == syscall.EAGAIN {
			return false
		}
		if from != nil {
			switch fromAddr := from.(type) {
			case *syscall.SockaddrInet4:
				w.readFrom = M.SocksaddrFrom(netip.AddrFrom4(fromAddr.Addr), uint16(fromAddr.Port))
			case *syscall.SockaddrInet6:
				w.readFrom = M.SocksaddrFrom(netip.AddrFrom16(fromAddr.Addr), uint16(fromAddr.Port)).Unwrap()
			}
		}
		w.buffer = buffer
		return true
	}
	return false
}

func (w *syscallPacketReadWaiter) WaitReadPacket() (buffer *buf.Buffer, destination M.Socksaddr, err error) {
	if w.readFunc == nil {
		return nil, M.Socksaddr{}, os.ErrInvalid
	}
	err = w.rawConn.Read(w.readFunc)
	if err != nil {
		return
	}
	if w.readErr != nil {
		err = E.Cause(w.readErr, "raw read")
		return
	}
	buffer = w.buffer
	w.buffer = nil
	destination = w.readFrom
	return
}
