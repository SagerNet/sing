package bufio

import (
	"os"
	"sync/atomic"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type CounterPacketConn struct {
	N.PacketConn
	readCounter  []N.CountFunc
	writeCounter []N.CountFunc
}

func NewInt64CounterPacketConn(conn N.PacketConn, readCounter []*atomic.Int64, readPacketCounter []*atomic.Int64, writeCounter []*atomic.Int64, writePacketCounter []*atomic.Int64) *CounterPacketConn {
	return &CounterPacketConn{
		conn,
		append(common.Map(readCounter, func(it *atomic.Int64) N.CountFunc {
			return func(n int64) {
				it.Add(n)
			}
		}), common.Map(readPacketCounter, func(it *atomic.Int64) N.CountFunc {
			return func(n int64) {
				it.Add(1)
			}
		})...),
		append(common.Map(writeCounter, func(it *atomic.Int64) N.CountFunc {
			return func(n int64) {
				it.Add(n)
			}
		}), common.Map(writePacketCounter, func(it *atomic.Int64) N.CountFunc {
			return func(n int64) {
				it.Add(1)
			}
		})...),
	}
}

func NewCounterPacketConn(conn N.PacketConn, readCounter []N.CountFunc, writeCounter []N.CountFunc) *CounterPacketConn {
	return &CounterPacketConn{conn, readCounter, writeCounter}
}

func (c *CounterPacketConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	destination, err = c.PacketConn.ReadPacket(buffer)
	if err == nil {
		if buffer.Len() > 0 {
			for _, counter := range c.readCounter {
				counter(int64(buffer.Len()))
			}
		}
	}
	return
}

func (c *CounterPacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	dataLen := int64(buffer.Len())
	err := c.PacketConn.WritePacket(buffer, destination)
	if err != nil {
		return err
	}
	if dataLen > 0 {
		for _, counter := range c.writeCounter {
			counter(dataLen)
		}
	}
	return nil
}

func (c *CounterPacketConn) CreatePacketBatchWriter() (N.PacketBatchWriter, bool) {
	writer, created := CreatePacketBatchWriter(c.PacketConn)
	if !created {
		return nil, false
	}
	return &counterPacketBatchWriter{writer, c.writeCounter}, true
}

type counterPacketBatchWriter struct {
	writer      N.PacketBatchWriter
	writeCounts []N.CountFunc
}

func (w *counterPacketBatchWriter) WritePacketBatch(buffers []*buf.Buffer, destinations []M.Socksaddr) error {
	if len(buffers) == 0 || len(buffers) != len(destinations) {
		buf.ReleaseMulti(buffers)
		return os.ErrInvalid
	}
	dataLens := make([]int64, len(buffers))
	for index, buffer := range buffers {
		dataLens[index] = int64(buffer.Len())
	}
	err := w.writer.WritePacketBatch(buffers, destinations)
	if err != nil {
		return err
	}
	for _, dataLen := range dataLens {
		if dataLen > 0 {
			for _, counter := range w.writeCounts {
				counter(dataLen)
			}
		}
	}
	return nil
}

func (c *CounterPacketConn) CreateConnectedPacketBatchWriter() (N.ConnectedPacketBatchWriter, bool) {
	writer, created := CreateConnectedPacketBatchWriter(c.PacketConn)
	if !created {
		return nil, false
	}
	return &counterConnectedPacketBatchWriter{writer, c.writeCounter}, true
}

type counterConnectedPacketBatchWriter struct {
	writer      N.ConnectedPacketBatchWriter
	writeCounts []N.CountFunc
}

func (w *counterConnectedPacketBatchWriter) WriteConnectedPacketBatch(buffers []*buf.Buffer) error {
	if len(buffers) == 0 {
		buf.ReleaseMulti(buffers)
		return os.ErrInvalid
	}
	dataLens := make([]int64, len(buffers))
	for index, buffer := range buffers {
		dataLens[index] = int64(buffer.Len())
	}
	err := w.writer.WriteConnectedPacketBatch(buffers)
	if err != nil {
		return err
	}
	for _, dataLen := range dataLens {
		if dataLen > 0 {
			for _, counter := range w.writeCounts {
				counter(dataLen)
			}
		}
	}
	return nil
}

func (c *CounterPacketConn) CreatePacketBatchReadWaiter() (N.PacketBatchReadWaiter, bool) {
	readWaiter, isReadWaiter := CreatePacketBatchReadWaiter(c.PacketConn)
	if !isReadWaiter {
		return nil, false
	}
	return &counterPacketBatchReadWaiter{readWaiter, c.readCounter}, true
}

func (c *CounterPacketConn) CreateConnectedPacketBatchReadWaiter() (N.ConnectedPacketBatchReadWaiter, bool) {
	readWaiter, isReadWaiter := CreateConnectedPacketBatchReadWaiter(c.PacketConn)
	if !isReadWaiter {
		return nil, false
	}
	return &counterConnectedPacketBatchReadWaiter{readWaiter, c.readCounter}, true
}

func (c *CounterPacketConn) UnwrapPacketReader() (N.PacketReader, []N.CountFunc) {
	return c.PacketConn, c.readCounter
}

func (c *CounterPacketConn) UnwrapPacketWriter() (N.PacketWriter, []N.CountFunc) {
	return c.PacketConn, c.writeCounter
}

func (c *CounterPacketConn) Upstream() any {
	return c.PacketConn
}

type counterPacketBatchReadWaiter struct {
	readWaiter N.PacketBatchReadWaiter
	readCounts []N.CountFunc
}

func (w *counterPacketBatchReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	return w.readWaiter.InitializeReadWaiter(options)
}

func (w *counterPacketBatchReadWaiter) WaitReadPackets() (buffers []*buf.Buffer, destinations []M.Socksaddr, err error) {
	buffers, destinations, err = w.readWaiter.WaitReadPackets()
	if err != nil {
		return
	}
	for _, buffer := range buffers {
		if buffer.Len() > 0 {
			for _, counter := range w.readCounts {
				counter(int64(buffer.Len()))
			}
		}
	}
	return
}

type counterConnectedPacketBatchReadWaiter struct {
	readWaiter N.ConnectedPacketBatchReadWaiter
	readCounts []N.CountFunc
}

func (w *counterConnectedPacketBatchReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	return w.readWaiter.InitializeReadWaiter(options)
}

func (w *counterConnectedPacketBatchReadWaiter) WaitReadConnectedPackets() (buffers []*buf.Buffer, destination M.Socksaddr, err error) {
	buffers, destination, err = w.readWaiter.WaitReadConnectedPackets()
	if err != nil {
		return
	}
	for _, buffer := range buffers {
		if buffer.Len() > 0 {
			for _, counter := range w.readCounts {
				counter(int64(buffer.Len()))
			}
		}
	}
	return
}
