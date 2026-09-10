//go:build linux || netbsd || darwin

package bufio

import "github.com/sagernet/sing/common/buf"

func (w *syscallPacketBatchReadWaiter) releaseBuffers() {
	clear(w.iovecs)
	clear(w.msgvec)
	buf.ReleaseMulti(w.buffers)
	clear(w.buffers)
	w.readN = 0
}
