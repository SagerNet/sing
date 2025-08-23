package bufio

import (
	"os"
	"sync"

	"github.com/metacubex/sing/common/buf"
	M "github.com/metacubex/sing/common/metadata"

	"golang.org/x/sys/windows"
)

type syscallVectorisedWriterFields struct {
	access    sync.Mutex
	iovecList *[]windows.WSABuf
}

func (w *SyscallVectorisedWriter) WriteVectorised(buffers []*buf.Buffer) error {
	w.access.Lock()
	defer w.access.Unlock()
	defer buf.ReleaseMulti(buffers)
	var iovecList []windows.WSABuf
	if w.iovecList != nil {
		iovecList = *w.iovecList
	}
	iovecList = iovecList[:0]
	for _, buffer := range buffers {
		if buffer.IsEmpty() {
			continue
		}
		iovecList = append(iovecList, buffer.Iovec(buffer.Len()))
	}
	if len(iovecList) == 0 {
		return os.ErrInvalid
	}
	if w.iovecList == nil {
		w.iovecList = new([]windows.WSABuf)
	}
	*w.iovecList = iovecList // cache
	var n uint32
	var innerErr error
	err := w.rawConn.Write(func(fd uintptr) (done bool) {
		innerErr = windows.WSASend(windows.Handle(fd), &iovecList[0], uint32(len(iovecList)), &n, 0, nil, nil)
		return innerErr != windows.WSAEWOULDBLOCK
	})
	if innerErr != nil {
		err = innerErr
	}
	for index := range iovecList {
		iovecList[index] = windows.WSABuf{}
	}
	return err
}

func (w *SyscallVectorisedPacketWriter) WriteVectorisedPacket(buffers []*buf.Buffer, destination M.Socksaddr) error {
	w.access.Lock()
	defer w.access.Unlock()
	defer buf.ReleaseMulti(buffers)
	var iovecList []windows.WSABuf
	if w.iovecList != nil {
		iovecList = *w.iovecList
	}
	iovecList = iovecList[:0]
	for _, buffer := range buffers {
		if buffer.IsEmpty() {
			continue
		}
		iovecList = append(iovecList, buffer.Iovec(buffer.Len()))
	}
	if w.iovecList == nil {
		w.iovecList = new([]windows.WSABuf)
	}
	*w.iovecList = iovecList // cache
	var n uint32
	var innerErr error
	err := w.rawConn.Write(func(fd uintptr) (done bool) {
		name, nameLen := M.AddrPortToRawSockaddr(destination.AddrPort())
		var bufs *windows.WSABuf
		if len(iovecList) > 0 {
			bufs = &iovecList[0]
		}
		innerErr = windows.WSASendTo(
			windows.Handle(fd),
			bufs,
			uint32(len(iovecList)),
			&n,
			0,
			(*windows.RawSockaddrAny)(name),
			nameLen,
			nil,
			nil)
		return innerErr != windows.WSAEWOULDBLOCK
	})
	if innerErr != nil {
		err = innerErr
	}
	for index := range iovecList {
		iovecList[index] = windows.WSABuf{}
	}
	return err
}
