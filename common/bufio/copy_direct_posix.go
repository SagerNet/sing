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
	frontHeadroom := N.CalculateFrontHeadroom(destination)
	rearHeadroom := N.CalculateRearHeadroom(destination)
	bufferSize := N.CalculateMTU(source, destination)
	if bufferSize > 0 {
		bufferSize += frontHeadroom + rearHeadroom
	} else {
		bufferSize = buf.BufferSize
	}
	var (
		buffer       *buf.Buffer
		readBuffer   *buf.Buffer
		notFirstTime bool
	)
	source.InitializeReadWaiter(func() *buf.Buffer {
		buffer = buf.NewSize(bufferSize)
		readBufferRaw := buffer.Slice()
		readBuffer = buf.With(readBufferRaw[:len(readBufferRaw)-rearHeadroom])
		readBuffer.Resize(frontHeadroom, 0)
		return readBuffer
	})
	defer source.InitializeReadWaiter(nil)
	for {
		err = source.WaitReadBuffer()
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
				return
			}
			return
		}
		dataLen := readBuffer.Len()
		buffer.Resize(readBuffer.Start(), dataLen)
		err = destination.WriteBuffer(buffer)
		if err != nil {
			buffer.Release()
			if !notFirstTime {
				err = N.HandshakeFailure(originSource, err)
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
	frontHeadroom := N.CalculateFrontHeadroom(destinationConn)
	rearHeadroom := N.CalculateRearHeadroom(destinationConn)
	bufferSize := N.CalculateMTU(source, destinationConn)
	if bufferSize > 0 {
		bufferSize += frontHeadroom + rearHeadroom
	} else {
		bufferSize = buf.UDPBufferSize
	}
	var (
		buffer      *buf.Buffer
		readBuffer  *buf.Buffer
		destination M.Socksaddr
	)
	source.InitializeReadWaiter(func() *buf.Buffer {
		buffer = buf.NewSize(bufferSize)
		readBufferRaw := buffer.Slice()
		readBuffer = buf.With(readBufferRaw[:len(readBufferRaw)-rearHeadroom])
		readBuffer.Resize(frontHeadroom, 0)
		return readBuffer
	})
	defer source.InitializeReadWaiter(nil)
	for {
		destination, err = source.WaitReadPacket()
		if err != nil {
			return
		}
		dataLen := readBuffer.Len()
		buffer.Resize(readBuffer.Start(), dataLen)
		err = destinationConn.WritePacket(buffer, destination)
		if err != nil {
			buffer.Release()
			if !notFirstTime {
				err = N.HandshakeFailure(originSource, err)
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

func (w *syscallReadWaiter) InitializeReadWaiter(newBuffer func() *buf.Buffer) {
	w.readErr = nil
	if newBuffer == nil {
		w.readFunc = nil
	} else {
		w.readFunc = func(fd uintptr) (done bool) {
			buffer := newBuffer()
			var readN int
			readN, w.readErr = syscall.Read(int(fd), buffer.FreeBytes())
			if readN > 0 {
				buffer.Truncate(readN)
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
			return true
		}
	}
}

func (w *syscallReadWaiter) WaitReadBuffer() error {
	if w.readFunc == nil {
		return os.ErrInvalid
	}
	err := w.rawConn.Read(w.readFunc)
	if err != nil {
		return err
	}
	if w.readErr != nil {
		if w.readErr == io.EOF {
			return io.EOF
		}
		return E.Cause(w.readErr, "raw read")
	}
	return nil
}

var _ N.PacketReadWaiter = (*syscallPacketReadWaiter)(nil)

type syscallPacketReadWaiter struct {
	rawConn  syscall.RawConn
	readErr  error
	readFrom M.Socksaddr
	readFunc func(fd uintptr) (done bool)
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

func (w *syscallPacketReadWaiter) InitializeReadWaiter(newBuffer func() *buf.Buffer) {
	w.readErr = nil
	w.readFrom = M.Socksaddr{}
	if newBuffer == nil {
		w.readFunc = nil
	} else {
		w.readFunc = func(fd uintptr) (done bool) {
			buffer := newBuffer()
			var readN int
			var from syscall.Sockaddr
			readN, _, _, from, w.readErr = syscall.Recvmsg(int(fd), buffer.FreeBytes(), nil, 0)
			if readN > 0 {
				buffer.Truncate(readN)
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
			return true
		}
	}
}

func (w *syscallPacketReadWaiter) WaitReadPacket() (destination M.Socksaddr, err error) {
	if w.readFunc == nil {
		return M.Socksaddr{}, os.ErrInvalid
	}
	err = w.rawConn.Read(w.readFunc)
	if err != nil {
		return
	}
	if w.readErr != nil {
		err = E.Cause(w.readErr, "raw read")
		return
	}
	destination = w.readFrom
	return
}
