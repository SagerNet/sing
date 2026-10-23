package bufio

import (
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

var _ N.PacketBatchWriter = (*syscallPacketBatchWriter)(nil)

type syscallPacketBatchWriter struct {
	upstream       any
	rawConn        syscall.RawConn
	writeFunc      func(fd uintptr) (done bool)
	access         sync.Mutex
	localAddr      netip.AddrPort
	names          []unix.RawSockaddrAny
	nameLens       []uint32
	send           func(int, []byte, *unix.RawSockaddrAny, uint32) syscall.Errno
	pending        []*buf.Buffer
	index          int
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

func (w *syscallPacketBatchWriter) WritePacketBatch(buffers []*buf.Buffer, destinations []M.Socksaddr) error {
	return w.writePacketBatch(buffers, destinations, sendto)
}

func (w *syscallPacketBatchWriter) writePacketBatch(buffers []*buf.Buffer, destinations []M.Socksaddr, send func(int, []byte, *unix.RawSockaddrAny, uint32) syscall.Errno) error {
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
	nameLens := growSlice(w.nameLens, len(buffers))
	defer func() {
		w.names = names[:0]
		w.nameLens = nameLens[:0]
		w.send = nil
		w.pending = nil
	}()
	for index, destination := range destinations {
		nameLens[index] = M.AddrPortToRawSockaddrAny(&names[index], destination.AddrPort(), w.localAddr.Addr().Is6())
	}
	w.names, w.nameLens = names, nameLens
	w.send = send
	w.pending = buffers
	// Keep the cursor across poller wakeups so a partially sent batch is not replayed.
	w.index = 0
	w.writeErr, w.messageTooLong = 0, false
	err := w.rawConn.Write(w.writeFunc)
	if w.writeErr != 0 {
		err = os.NewSyscallError("sendto", w.writeErr)
	} else if err == nil && w.messageTooLong {
		err = os.NewSyscallError("sendto", syscall.EMSGSIZE)
	}
	return err
}

func (w *syscallPacketBatchWriter) write(fd uintptr) (done bool) {
	for w.index < len(w.pending) {
		errno := w.send(int(fd), w.pending[w.index].Bytes(), &w.names[w.index], w.nameLens[w.index])
		switch errno {
		case 0:
			w.index++
		case syscall.EINTR:
			continue
		case syscall.EAGAIN:
			return false
		case syscall.EMSGSIZE:
			w.messageTooLong = true
			w.index++
		default:
			if errno == syscall.EWOULDBLOCK {
				return false
			}
			w.writeErr = errno
			return true
		}
	}
	return true
}

func (w *syscallPacketBatchWriter) Upstream() any {
	return w.upstream
}

func sendto(fd int, data []byte, name *unix.RawSockaddrAny, nameLen uint32) syscall.Errno {
	//nolint:staticcheck
	_, _, errno := unix.Syscall6(unix.SYS_SENDTO, uintptr(fd), uintptr(unsafe.Pointer(unsafe.SliceData(data))), uintptr(len(data)), 0, uintptr(unsafe.Pointer(name)), uintptr(nameLen))
	return errno
}
