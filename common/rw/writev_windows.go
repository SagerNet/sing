package rw

import "golang.org/x/sys/windows"

func WriteV(fd uintptr, data ...[]byte) (int, error) {
	var n uint32
	buffers := make([]*windows.WSABuf, len(data))
	for i, buf := range data {
		buffers[i] = &windows.WSABuf{
			Len: uint32(len(buf)),
			Buf: &buf[0],
		}
	}
	err := windows.WSASend(windows.Handle(fd), buffers[0], uint32(len(buffers)), &n, 0, nil, nil)
	return int(n), err
}
