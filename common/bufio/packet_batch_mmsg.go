//go:build linux || netbsd

package bufio

import (
	"io"
	"net/netip"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/control"
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
	probe        bool
	probeBuffer  [1]byte
	released     bool
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
	w.releaseBuffers()
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
		if w.probe {
			w.probe = false
			errno := peekPacket(int(fd), &w.probeBuffer)
			if errno == syscall.EAGAIN || errno == syscall.EWOULDBLOCK {
				return false
			}
			if errno != 0 {
				w.readErr = os.NewSyscallError("recvfrom", errno)
				return true
			}
		}
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
		w.released = false
		for {
			var errno syscall.Errno
			w.readN, errno = recvmmsg(int(fd), w.msgvec, 0)
			switch errno {
			case 0:
				w.readErr = nil
			case syscall.EINTR:
				continue
			case syscall.EAGAIN:
				w.releaseBuffers()
				return false
			default:
				if errno == syscall.EWOULDBLOCK {
					w.releaseBuffers()
					return false
				}
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

var (
	_ N.PacketBatchWriter          = (*syscallPacketBatchWriter)(nil)
	_ N.ConnectedPacketBatchWriter = (*syscallPacketBatchWriter)(nil)
)

type syscallPacketBatchWriter struct {
	offload        syscallPacketBatchOffload
	upstream       any
	rawConn        syscall.RawConn
	writeFunc      func(fd uintptr) (done bool)
	access         sync.Mutex
	localAddr      netip.AddrPort
	names          []unix.RawSockaddrAny
	iovecs         []unix.Iovec
	msgvec         []mmsghdr
	pending        []mmsghdr
	writeErr       syscall.Errno
	messageTooLong bool
}

func createSyscallPacketBatchWriter(writer any) (N.PacketBatchWriter, bool) {
	rawConn := syscallPacketBatchRawConnForWrite(writer)
	if rawConn == nil {
		return nil, false
	}
	if _, isConnected := syscallPacketBatchPeerDestination(rawConn); isConnected {
		return nil, false
	}
	w := &syscallPacketBatchWriter{upstream: writer, rawConn: rawConn}
	w.writeFunc = w.write
	return w, true
}

func createSyscallConnectedPacketBatchWriter(writer any) (N.ConnectedPacketBatchWriter, bool) {
	rawConn := syscallPacketBatchRawConnForWrite(writer)
	if rawConn == nil {
		return nil, false
	}
	if _, isConnected := syscallPacketBatchPeerDestination(rawConn); !isConnected {
		return nil, false
	}
	w := &syscallPacketBatchWriter{upstream: writer, rawConn: rawConn}
	w.writeFunc = w.write
	return w, true
}

func (w *syscallPacketBatchWriter) WritePacketBatch(buffers []*buf.Buffer, destinations []M.Socksaddr) error {
	w.access.Lock()
	defer w.access.Unlock()
	defer buf.ReleaseMulti(buffers)
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
		w.offload.reset()
		clear(iovecs)
		clear(msgvec)
		w.names = names[:0]
		w.iovecs = iovecs[:0]
		w.msgvec = msgvec[:0]
		w.pending = nil
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
	w.pending = msgvec
	w.writeErr, w.messageTooLong = 0, false
	err := w.rawConn.Write(w.writeFunc)
	if w.writeErr != 0 {
		err = os.NewSyscallError("sendmmsg", w.writeErr)
	} else if err == nil && w.messageTooLong {
		err = os.NewSyscallError("sendmmsg", syscall.EMSGSIZE)
	}
	return err
}

func (w *syscallPacketBatchWriter) WriteConnectedPacketBatch(buffers []*buf.Buffer) error {
	w.access.Lock()
	defer w.access.Unlock()
	defer buf.ReleaseMulti(buffers)
	iovecs := growSlice(w.iovecs, len(buffers))
	msgvec := growSlice(w.msgvec, len(buffers))
	defer func() {
		w.offload.reset()
		clear(iovecs)
		clear(msgvec)
		w.iovecs = iovecs[:0]
		w.msgvec = msgvec[:0]
		w.pending = nil
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
	w.pending = msgvec
	w.writeErr, w.messageTooLong = 0, false
	err := w.rawConn.Write(w.writeFunc)
	if w.writeErr != 0 {
		err = os.NewSyscallError("sendmmsg", w.writeErr)
	} else if err == nil && w.messageTooLong {
		err = os.NewSyscallError("sendmmsg", syscall.EMSGSIZE)
	}
	return err
}

func (w *syscallPacketBatchWriter) write(fd uintptr) (done bool) {
	for len(w.pending) > 0 {
		n, errno := w.offload.send(int(fd), w.pending, 0)
		switch errno {
		case 0:
		case syscall.EINTR:
			continue
		case syscall.EAGAIN:
			return false
		case syscall.EMSGSIZE:
			w.messageTooLong = true
			w.pending = w.pending[1:]
			continue
		default:
			if errno == syscall.EWOULDBLOCK {
				return false
			}
			w.writeErr = errno
			return true
		}
		if n == 0 {
			w.writeErr = syscall.EIO
			return true
		}
		w.pending = w.pending[n:]
	}
	return true
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
	return mmsgSyscall(unix.SYS_RECVMMSG, fd, msgvec, flags)
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
