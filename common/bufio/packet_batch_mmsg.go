//go:build linux || netbsd

package bufio

import (
	"io"
	"net/netip"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/control"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"golang.org/x/sys/unix"
)

var (
	_ N.PacketBatchReadWaiter          = (*syscallPacketBatchReadWaiter)(nil)
	_ N.ConnectedPacketBatchReadWaiter = (*syscallPacketBatchReadWaiter)(nil)
)

type mmsghdr struct {
	msgHdr unix.Msghdr
	msgLen uint32
}

type syscallPacketBatchReadWaiter struct {
	rawConn      syscall.RawConn
	connected    bool
	destination  M.Socksaddr
	readErr      error
	readN        int
	readFunc     func(fd uintptr) (done bool)
	buffers      []*buf.Buffer
	destinations []M.Socksaddr
	names        []unix.RawSockaddrAny
	iovecs       []unix.Iovec
	msgvec       []mmsghdr
	options      N.ReadWaitOptions
}

func createSyscallPacketBatchReadWaiter(reader any) (N.PacketBatchReadWaiter, bool) {
	rawConn := syscallPacketBatchRawConnForRead(reader)
	if rawConn == nil {
		return nil, false
	}
	if _, isConnected := syscallPacketBatchPeerDestination(rawConn); isConnected {
		return nil, false
	}
	return &syscallPacketBatchReadWaiter{rawConn: rawConn}, true
}

func createSyscallConnectedPacketBatchReadWaiter(reader any, destination M.Socksaddr) (N.ConnectedPacketBatchReadWaiter, bool) {
	rawConn := syscallPacketBatchRawConnForRead(reader)
	if rawConn == nil {
		return nil, false
	}
	peerDestination, isConnected := syscallPacketBatchPeerDestination(rawConn)
	if !isConnected {
		return nil, false
	}
	if !destination.IsValid() {
		destination = peerDestination
	}
	return &syscallPacketBatchReadWaiter{rawConn: rawConn, connected: true, destination: destination}, true
}

func (w *syscallPacketBatchReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	if options.BatchSize <= 0 {
		options.BatchSize = DefaultPacketReadBatchSize
	}
	w.options = options
	w.buffers = make([]*buf.Buffer, options.BatchSize)
	if !w.connected {
		w.destinations = make([]M.Socksaddr, options.BatchSize)
		w.names = make([]unix.RawSockaddrAny, options.BatchSize)
	}
	w.iovecs = make([]unix.Iovec, options.BatchSize)
	w.msgvec = make([]mmsghdr, options.BatchSize)
	w.readFunc = func(fd uintptr) (done bool) {
		for i := range w.msgvec {
			buffer := w.buffers[i]
			if buffer == nil {
				buffer = w.options.NewPacketBuffer()
				w.buffers[i] = buffer
			}
			w.iovecs[i] = buffer.Iovec(buffer.FreeLen())
			w.msgvec[i] = mmsghdr{}
			if !w.connected {
				w.names[i] = unix.RawSockaddrAny{}
				w.msgvec[i].msgHdr.Name = (*byte)(unsafe.Pointer(&w.names[i]))
				w.msgvec[i].msgHdr.Namelen = unix.SizeofSockaddrAny
			}
			w.msgvec[i].msgHdr.Iov = &w.iovecs[i]
			w.msgvec[i].msgHdr.SetIovlen(1)
		}
		for {
			var errno syscall.Errno
			w.readN, errno = recvmmsg(int(fd), w.msgvec, 0)
			switch {
			case errno == 0:
				w.readErr = nil
			case errno == syscall.EINTR:
				continue
			case errno == syscall.EAGAIN || errno == syscall.EWOULDBLOCK:
				return false
			default:
				w.readErr = os.NewSyscallError("recvmmsg", errno)
			}
			break
		}
		if w.readN == 0 && w.readErr == nil {
			w.readErr = io.EOF
		}
		for i := 0; i < w.readN; i++ {
			buffer := w.buffers[i]
			buffer.Truncate(int(w.msgvec[i].msgLen))
			w.options.PostReturn(buffer)
			if !w.connected {
				w.destinations[i] = M.SocksaddrFromRawSockaddrAny(&w.names[i])
			}
		}
		return true
	}
	return false
}

func (w *syscallPacketBatchReadWaiter) WaitReadPackets() (buffers []*buf.Buffer, destinations []M.Socksaddr, err error) {
	if w.connected {
		return nil, nil, os.ErrInvalid
	}
	if w.readFunc == nil {
		return nil, nil, os.ErrInvalid
	}
	err = w.rawConn.Read(w.readFunc)
	if err != nil {
		return
	}
	if w.readErr != nil {
		if w.readErr == io.EOF {
			return nil, nil, io.EOF
		}
		return nil, nil, E.Cause(w.readErr, "raw read")
	}
	buffers = make([]*buf.Buffer, w.readN)
	destinations = make([]M.Socksaddr, w.readN)
	for i := 0; i < w.readN; i++ {
		buffers[i] = w.buffers[i]
		w.buffers[i] = nil
		destinations[i] = w.destinations[i]
	}
	w.readN = 0
	return
}

func (w *syscallPacketBatchReadWaiter) WaitReadConnectedPackets() (buffers []*buf.Buffer, destination M.Socksaddr, err error) {
	if !w.connected {
		return nil, M.Socksaddr{}, os.ErrInvalid
	}
	if w.readFunc == nil {
		return nil, M.Socksaddr{}, os.ErrInvalid
	}
	err = w.rawConn.Read(w.readFunc)
	if err != nil {
		return
	}
	if w.readErr != nil {
		if w.readErr == io.EOF {
			return nil, M.Socksaddr{}, io.EOF
		}
		return nil, M.Socksaddr{}, E.Cause(w.readErr, "raw read")
	}
	buffers = make([]*buf.Buffer, w.readN)
	for i := 0; i < w.readN; i++ {
		buffers[i] = w.buffers[i]
		w.buffers[i] = nil
	}
	w.readN = 0
	destination = w.destination
	return
}

var (
	_ N.PacketBatchWriter          = (*syscallPacketBatchWriter)(nil)
	_ N.ConnectedPacketBatchWriter = (*syscallPacketBatchWriter)(nil)
)

type syscallPacketBatchWriter struct {
	upstream  any
	rawConn   syscall.RawConn
	connected bool
	access    sync.Mutex
	localAddr netip.AddrPort
	names     []unix.RawSockaddrAny
	iovecs    []unix.Iovec
	msgvec    []mmsghdr
}

func createSyscallPacketBatchWriter(writer any) (N.PacketBatchWriter, bool) {
	rawConn := syscallPacketBatchRawConnForWrite(writer)
	if rawConn == nil {
		return nil, false
	}
	if _, isConnected := syscallPacketBatchPeerDestination(rawConn); isConnected {
		return nil, false
	}
	return &syscallPacketBatchWriter{upstream: writer, rawConn: rawConn}, true
}

func createSyscallConnectedPacketBatchWriter(writer any) (N.ConnectedPacketBatchWriter, bool) {
	rawConn := syscallPacketBatchRawConnForWrite(writer)
	if rawConn == nil {
		return nil, false
	}
	if _, isConnected := syscallPacketBatchPeerDestination(rawConn); !isConnected {
		return nil, false
	}
	return &syscallPacketBatchWriter{upstream: writer, rawConn: rawConn, connected: true}, true
}

func (w *syscallPacketBatchWriter) WritePacketBatch(buffers []*buf.Buffer, destinations []M.Socksaddr) error {
	if w.connected {
		buf.ReleaseMulti(buffers)
		return os.ErrInvalid
	}
	w.access.Lock()
	defer w.access.Unlock()
	defer buf.ReleaseMulti(buffers)
	if len(buffers) == 0 || len(buffers) != len(destinations) {
		return os.ErrInvalid
	}
	if !w.localAddr.IsValid() {
		err := control.Raw(w.rawConn, func(fd uintptr) error {
			name, err := unix.Getsockname(int(fd))
			if err != nil {
				return err
			}
			w.localAddr = M.AddrPortFromSockaddr(name)
			return nil
		})
		if err != nil {
			return err
		}
	}
	names := growSlice(w.names, len(buffers))
	iovecs := growSlice(w.iovecs, len(buffers))
	msgvec := growSlice(w.msgvec, len(buffers))
	defer func() {
		common.ClearArray(iovecs)
		common.ClearArray(msgvec)
		w.names = names[:0]
		w.iovecs = iovecs[:0]
		w.msgvec = msgvec[:0]
	}()
	for i, buffer := range buffers {
		names[i] = unix.RawSockaddrAny{}
		iovecs[i] = unix.Iovec{}
		msgvec[i] = mmsghdr{}
		msgvec[i].msgHdr.Name = (*byte)(unsafe.Pointer(&names[i]))
		msgvec[i].msgHdr.Namelen = M.AddrPortToRawSockaddrAny(&names[i], destinations[i].AddrPort(), w.localAddr.Addr().Is6())
		if !buffer.IsEmpty() {
			iovecs[i] = buffer.Iovec(buffer.Len())
			msgvec[i].msgHdr.Iov = &iovecs[i]
			msgvec[i].msgHdr.SetIovlen(1)
		}
	}
	writeMsgvec := msgvec
	var innerErr syscall.Errno
	err := w.rawConn.Write(func(fd uintptr) (done bool) {
		for len(writeMsgvec) > 0 {
			n, errno := sendmmsg(int(fd), writeMsgvec, 0)
			switch {
			case errno == 0:
			case errno == syscall.EINTR:
				continue
			case errno == syscall.EAGAIN || errno == syscall.EWOULDBLOCK:
				return false
			default:
				innerErr = errno
				return true
			}
			if n == 0 {
				innerErr = syscall.EIO
				return true
			}
			writeMsgvec = writeMsgvec[n:]
		}
		return true
	})
	if innerErr != 0 {
		err = os.NewSyscallError("sendmmsg", innerErr)
	}
	return err
}

func (w *syscallPacketBatchWriter) WriteConnectedPacketBatch(buffers []*buf.Buffer) error {
	if !w.connected {
		buf.ReleaseMulti(buffers)
		return os.ErrInvalid
	}
	w.access.Lock()
	defer w.access.Unlock()
	defer buf.ReleaseMulti(buffers)
	if len(buffers) == 0 {
		return os.ErrInvalid
	}
	iovecs := growSlice(w.iovecs, len(buffers))
	msgvec := growSlice(w.msgvec, len(buffers))
	defer func() {
		common.ClearArray(iovecs)
		common.ClearArray(msgvec)
		w.iovecs = iovecs[:0]
		w.msgvec = msgvec[:0]
	}()
	for i, buffer := range buffers {
		iovecs[i] = unix.Iovec{}
		msgvec[i] = mmsghdr{}
		if !buffer.IsEmpty() {
			iovecs[i] = buffer.Iovec(buffer.Len())
			msgvec[i].msgHdr.Iov = &iovecs[i]
			msgvec[i].msgHdr.SetIovlen(1)
		}
	}
	writeMsgvec := msgvec
	var innerErr syscall.Errno
	err := w.rawConn.Write(func(fd uintptr) (done bool) {
		for len(writeMsgvec) > 0 {
			n, errno := sendmmsg(int(fd), writeMsgvec, 0)
			switch {
			case errno == 0:
			case errno == syscall.EINTR:
				continue
			case errno == syscall.EAGAIN || errno == syscall.EWOULDBLOCK:
				return false
			default:
				innerErr = errno
				return true
			}
			if n == 0 {
				innerErr = syscall.EIO
				return true
			}
			writeMsgvec = writeMsgvec[n:]
		}
		return true
	})
	if innerErr != 0 {
		err = os.NewSyscallError("sendmmsg", innerErr)
	}
	return err
}

func (w *syscallPacketBatchWriter) Upstream() any {
	return w.upstream
}

func growSlice[T any](values []T, size int) []T {
	if cap(values) < size {
		return make([]T, size)
	}
	return values[:size]
}

func recvmmsg(fd int, msgvec []mmsghdr, flags int) (int, syscall.Errno) {
	return mmsgSyscall(sysRecvmmsg, fd, msgvec, flags)
}

func sendmmsg(fd int, msgvec []mmsghdr, flags int) (int, syscall.Errno) {
	return mmsgSyscall(unix.SYS_SENDMMSG, fd, msgvec, flags)
}

func mmsgSyscall(trap uintptr, fd int, msgvec []mmsghdr, flags int) (int, syscall.Errno) {
	r0, _, errno := unix.Syscall6(trap, uintptr(fd), uintptr(unsafe.Pointer(&msgvec[0])), uintptr(len(msgvec)), uintptr(flags), 0, 0)
	if errno != 0 {
		return 0, errno
	}
	return int(r0), 0
}
