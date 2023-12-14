package bufio

import (
	"sync"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"

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
		iovecList = append(iovecList, windows.WSABuf{
			Buf: &buffer.Bytes()[0],
			Len: uint32(buffer.Len()),
		})
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
		iovecList = append(iovecList, windows.WSABuf{
			Buf: &buffer.Bytes()[0],
			Len: uint32(buffer.Len()),
		})
	}
	if w.iovecList == nil {
		w.iovecList = new([]windows.WSABuf)
	}
	*w.iovecList = iovecList // cache
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
	if innerErr != nil {
		err = innerErr
	}
	for index := range iovecList {
		iovecList[index] = windows.WSABuf{}
	}
	return err
}
