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
	upstream  any
	rawConn   syscall.RawConn
	access    sync.Mutex
	localAddr netip.AddrPort
	names     []unix.RawSockaddrAny
	nameLens  []uint32
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
	}()
	for index, destination := range destinations {
		nameLens[index] = M.AddrPortToRawSockaddrAny(&names[index], destination.AddrPort(), w.localAddr.Addr().Is6())
	}
	// Keep the cursor across poller wakeups so a partially sent batch is not replayed.
	var index int
	var innerErr syscall.Errno
	err := w.rawConn.Write(func(fd uintptr) (done bool) {
		for index < len(buffers) {
			errno := send(int(fd), buffers[index].Bytes(), &names[index], nameLens[index])
			switch errno {
			case 0:
				index++
			case syscall.EINTR:
				continue
			case syscall.EAGAIN:
				return false
			default:
				if errno == syscall.EWOULDBLOCK {
					return false
				}
				innerErr = errno
				return true
			}
		}
		return true
	})
	if innerErr != 0 {
		err = os.NewSyscallError("sendto", innerErr)
	}
	return err
}

func (w *syscallPacketBatchWriter) Upstream() any {
	return w.upstream
}

func sendto(fd int, data []byte, name *unix.RawSockaddrAny, nameLen uint32) syscall.Errno {
	//nolint:staticcheck
	_, _, errno := unix.Syscall6(unix.SYS_SENDTO, uintptr(fd), uintptr(unsafe.Pointer(unsafe.SliceData(data))), uintptr(len(data)), 0, uintptr(unsafe.Pointer(name)), uintptr(nameLen))
	return errno
}
