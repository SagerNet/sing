package bufio

import (
	"sync"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func (c *UnbindPacketConn) CreateConnectedPacketBatchWriter() (N.ConnectedPacketBatchWriter, bool) {
	var packetWriter N.PacketWriter
	var destination func() M.Socksaddr
	upstream := N.UnwrapWriter(c.ExtendedConn)
	switch conn := upstream.(type) {
	case *bindPacketConn:
		packetWriter = conn.NetPacketConn
		address := M.SocksaddrFromNet(conn.addr).Unwrap()
		destination = func() M.Socksaddr { return address }
	case *serverPacketConn:
		packetWriter = conn.NetPacketConn
		destination = conn.remoteDestination
	default:
		return createSyscallConnectedPacketBatchWriter(c.ExtendedConn)
	}
	writer, created := CreatePacketBatchWriter(packetWriter)
	if !created {
		return nil, false
	}
	return &boundConnectedPacketBatchWriter{writer: writer, destination: destination}, true
}

func (c *serverPacketConn) CreatePacketBatchWriter() (N.PacketBatchWriter, bool) {
	return CreatePacketBatchWriter(c.NetPacketConn)
}

type boundConnectedPacketBatchWriter struct {
	writer       N.PacketBatchWriter
	destination  func() M.Socksaddr
	access       sync.Mutex
	destinations []M.Socksaddr
}

func (w *boundConnectedPacketBatchWriter) WriteConnectedPacketBatch(buffers []*buf.Buffer) error {
	count := 0
	for _, buffer := range buffers {
		if buffer.IsEmpty() {
			buffer.Release()
			continue
		}
		buffers[count] = buffer
		count++
	}
	clear(buffers[count:])
	if count == 0 {
		return nil
	}
	buffers = buffers[:count]
	w.access.Lock()
	defer w.access.Unlock()
	if cap(w.destinations) < len(buffers) {
		w.destinations = make([]M.Socksaddr, len(buffers))
	} else {
		w.destinations = w.destinations[:len(buffers)]
	}
	destination := w.destination()
	for index := range w.destinations {
		w.destinations[index] = destination
	}
	return w.writer.WritePacketBatch(buffers, w.destinations)
}

func (w *boundConnectedPacketBatchWriter) Upstream() any { return w.writer }

func (c *UnbindPacketConn) CreateConnectedPacketBatchReadWaiter() (N.ConnectedPacketBatchReadWaiter, bool) {
	upstream := N.UnwrapReader(c.ExtendedConn)
	switch conn := upstream.(type) {
	case *bindPacketConn:
		reader, created := CreatePacketBatchReadWaiter(conn.NetPacketConn)
		if !created {
			return nil, false
		}
		return &bindConnectedPacketBatchReadWaiter{reader: reader, destination: c.addr}, true
	case *serverPacketConn:
		reader, created := CreatePacketBatchReadWaiter(conn.NetPacketConn)
		if !created {
			return nil, false
		}
		return &serverConnectedPacketBatchReadWaiter{conn: conn, reader: reader, destination: c.addr}, true
	default:
		return createSyscallConnectedPacketBatchReadWaiter(c.ExtendedConn, c.addr)
	}
}

type bindConnectedPacketBatchReadWaiter struct {
	reader      N.PacketBatchReadWaiter
	destination M.Socksaddr
}

func (r *bindConnectedPacketBatchReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) bool {
	return r.reader.InitializeReadWaiter(options)
}

func (r *bindConnectedPacketBatchReadWaiter) WaitReadConnectedPackets() ([]*buf.Buffer, M.Socksaddr, error) {
	buffers, _, err := r.reader.WaitReadPackets()
	return buffers, r.destination, err
}

func (r *bindConnectedPacketBatchReadWaiter) Upstream() any { return r.reader }

type serverConnectedPacketBatchReadWaiter struct {
	conn        *serverPacketConn
	reader      N.PacketBatchReadWaiter
	destination M.Socksaddr
}

func (r *serverConnectedPacketBatchReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) bool {
	return r.reader.InitializeReadWaiter(options)
}

func (r *serverConnectedPacketBatchReadWaiter) WaitReadConnectedPackets() ([]*buf.Buffer, M.Socksaddr, error) {
	buffers, destinations, err := r.reader.WaitReadPackets()
	if err == nil {
		r.conn.updateRemoteAddr(destinations[len(destinations)-1])
	}
	return buffers, r.destination, err
}

func (r *serverConnectedPacketBatchReadWaiter) Upstream() any { return r.reader }

var (
	_ N.PacketBatchReadWaitCreator          = (*serverPacketConn)(nil)
	_ N.ConnectedPacketBatchWriteCreator    = (*UnbindPacketConn)(nil)
	_ N.ConnectedPacketBatchReadWaitCreator = (*UnbindPacketConn)(nil)
	_ N.PacketBatchWriteCreator             = (*serverPacketConn)(nil)
	_ N.ConnectedPacketBatchWriter          = (*boundConnectedPacketBatchWriter)(nil)
	_ N.ConnectedPacketBatchReadWaiter      = (*bindConnectedPacketBatchReadWaiter)(nil)
	_ N.ConnectedPacketBatchReadWaiter      = (*serverConnectedPacketBatchReadWaiter)(nil)
)
