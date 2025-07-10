package buf

import "golang.org/x/sys/windows"

func (b *Buffer) Iovec() windows.WSABuf {
	return windows.WSABuf{
		Buf: &b.data[b.start],
		Len: uint32(b.capacity),
	}
}
