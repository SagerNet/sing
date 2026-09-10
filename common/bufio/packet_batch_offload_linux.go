package bufio

import (
	"encoding/binary"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

type syscallPacketBatchOffload struct {
	disabled bool
	messages []mmsghdr
	controls [][24]byte
	counts   []int
}

func (o *syscallPacketBatchOffload) reset() {
	clear(o.messages[:cap(o.messages)])
}

func (o *syscallPacketBatchOffload) send(descriptor int, messages []mmsghdr, flags int) (int, syscall.Errno) {
	if o.disabled || len(messages) < 2 {
		return sendmmsg(descriptor, messages, flags)
	}
	o.messages = growSlice(o.messages, len(messages))
	o.controls = growSlice(o.controls, len(messages))
	o.counts = growSlice(o.counts, len(messages))
	messageCount := 0
	for start := 0; start < len(messages); {
		first := messages[start].msgHdr
		end := start + 1
		if first.Iovlen == 1 && first.Iov.Len > 0 {
			segmentSize := int(first.Iov.Len)
			length := segmentSize
			for end < len(messages) && end-start < 64 {
				next := messages[end].msgHdr
				if next.Iovlen != 1 || next.Iov.Len == 0 || int(next.Iov.Len) > segmentSize || length+int(next.Iov.Len) > 65507 || next.Namelen != first.Namelen {
					break
				}
				if first.Name != nil && *(*unix.RawSockaddrAny)(unsafe.Pointer(first.Name)) != *(*unix.RawSockaddrAny)(unsafe.Pointer(next.Name)) {
					break
				}
				length += int(next.Iov.Len)
				end++
				if int(next.Iov.Len) < segmentSize {
					break
				}
			}
			if end-start > 1 {
				controlData := o.controls[messageCount][:unix.CmsgSpace(2)]
				clear(controlData)
				header := (*unix.Cmsghdr)(unsafe.Pointer(&controlData[0]))
				header.Level = unix.IPPROTO_UDP
				header.Type = unix.UDP_SEGMENT
				header.SetLen(unix.CmsgLen(2))
				binary.NativeEndian.PutUint16(controlData[unix.CmsgLen(0):], uint16(segmentSize))
				first.Control = &controlData[0]
				first.SetControllen(len(controlData))
				first.SetIovlen(end - start)
			}
		}
		o.messages[messageCount] = mmsghdr{msgHdr: first}
		o.counts[messageCount] = end - start
		messageCount++
		start = end
	}
	if messageCount == len(messages) {
		return sendmmsg(descriptor, messages, flags)
	}
	count, errno := sendmmsg(descriptor, o.messages[:messageCount], flags)
	if errno != 0 {
		switch errno {
		case unix.EINVAL, unix.EIO, unix.ENOPROTOOPT, unix.EOPNOTSUPP:
			o.disabled = true
			return sendmmsg(descriptor, messages, flags)
		case unix.EMSGSIZE:
			return sendmmsg(descriptor, messages, flags)
		}
		return 0, errno
	}
	sent := 0
	for index := range count {
		sent += o.counts[index]
	}
	return sent, 0
}
