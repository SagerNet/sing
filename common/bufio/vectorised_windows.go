package bufio

import (
	"sync"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"

	"golang.org/x/sys/windows"
)

type syscallVectorisedWriterFields struct {
	access    sync.Mutex
	iovecList []windows.WSABuf
}

func (w *SyscallVectorisedWriter) WriteVectorised(buffers []*buf.Buffer) error {
	/*w.access.Lock()
	defer w.access.Unlock()
	defer buf.ReleaseMulti(buffers)
	iovecList := w.iovecList
	for _, buffer := range buffers {
		iovecList = append(iovecList, buffer.Iovec(buffer.Len()))
	}
	var n uint32
	var innerErr error
	err := w.rawConn.Write(func(fd uintptr) (done bool) {
		innerErr = windows.WSASend(windows.Handle(fd), &iovecList[0], uint32(len(iovecList)), &n, 0, nil, nil)
		return innerErr != windows.WSAEWOULDBLOCK
	})
	common.ClearArray(iovecList)
	if cap(iovecList) > cap(w.iovecList) {
		w.iovecList = w.iovecList[:0]
	}
	if innerErr != nil {
		err = innerErr
	}
	return err*/
	panic("not implemented")
}

func (w *SyscallVectorisedPacketWriter) WriteVectorisedPacket(buffers []*buf.Buffer, destination M.Socksaddr) error {
	w.access.Lock()
	defer w.access.Unlock()
	defer buf.ReleaseMulti(buffers)
	iovecList := w.iovecList
	for _, buffer := range buffers {
		iovecList = append(iovecList, buffer.Iovec(buffer.Len()))
	}
	var n uint32
	var innerErr error
	err := w.rawConn.Write(func(fd uintptr) (done bool) {
		name, nameLen := ToSockaddr(destination.AddrPort())
		innerErr = windows.WSASendTo(
			windows.Handle(fd),
			&iovecList[0],
			uint32(len(iovecList)),
			&n,
			0,
			(*windows.RawSockaddrAny)(name),
			nameLen,
			nil,
			nil)
		return innerErr != windows.WSAEWOULDBLOCK
	})
	for i := range iovecList {
		iovecList[i] = windows.WSABuf{}
	}
	if cap(iovecList) > cap(w.iovecList) {
		w.iovecList = w.iovecList[:0]
	}
	if innerErr != nil {
		err = innerErr
	}
	return err
}
