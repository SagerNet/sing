package socks

import (
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func (c *AssociatePacketConn) CreatePacketBatchWriter() (N.PacketBatchWriter, bool) {
	writer, created := bufio.CreateConnectedPacketBatchWriter(bufio.NewUnbindPacketConn(c.conn))
	if !created {
		return nil, false
	}
	return &associatePacketBatchWriter{writer: writer}, true
}

type associatePacketBatchWriter struct {
	writer N.ConnectedPacketBatchWriter
}

func (c *AssociatePacketConn) CreatePacketBatchReadWaiter() (N.PacketBatchReadWaiter, bool) {
	reader, created := bufio.CreateConnectedPacketBatchReadWaiter(bufio.NewUnbindPacketConn(c.conn))
	if !created {
		return nil, false
	}
	return &associatePacketBatchReadWaiter{conn: c, reader: reader}, true
}

type associatePacketBatchReadWaiter struct {
	conn   *AssociatePacketConn
	reader N.ConnectedPacketBatchReadWaiter
}

func (r *associatePacketBatchReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) bool {
	return r.reader.InitializeReadWaiter(options)
}

func (r *associatePacketBatchReadWaiter) WaitReadPackets() ([]*buf.Buffer, []M.Socksaddr, error) {
	buffers, _, err := r.reader.WaitReadConnectedPackets()
	if err != nil {
		return nil, nil, err
	}
	destinations := make([]M.Socksaddr, len(buffers))
	for index, buffer := range buffers {
		destinations[index], err = (associatePacketOffload{}).DecodePacket(buffer)
		if err != nil {
			buf.ReleaseMulti(buffers)
			return nil, nil, err
		}
	}
	r.conn.remoteAddr = destinations[len(destinations)-1]
	return buffers, destinations, nil
}

func (r *associatePacketBatchReadWaiter) Upstream() any {
	return r.reader
}

func (c *LazyAssociatePacketConn) CreatePacketBatchReadWaiter() (N.PacketBatchReadWaiter, bool) {
	reader, created := c.AssociatePacketConn.CreatePacketBatchReadWaiter()
	if !created {
		return nil, false
	}
	return &lazyAssociatePacketBatchReadWaiter{conn: c, reader: reader}, true
}

type lazyAssociatePacketBatchReadWaiter struct {
	conn   *LazyAssociatePacketConn
	reader N.PacketBatchReadWaiter
}

func (r *lazyAssociatePacketBatchReadWaiter) InitializeReadWaiter(options N.ReadWaitOptions) bool {
	return r.reader.InitializeReadWaiter(options)
}

func (r *lazyAssociatePacketBatchReadWaiter) WaitReadPackets() ([]*buf.Buffer, []M.Socksaddr, error) {
	err := r.conn.HandshakeSuccess()
	if err != nil {
		return nil, nil, err
	}
	return r.reader.WaitReadPackets()
}

func (r *lazyAssociatePacketBatchReadWaiter) Upstream() any { return r.reader }

func (w *associatePacketBatchWriter) WritePacketBatch(buffers []*buf.Buffer, destinations []M.Socksaddr) error {
	for index, buffer := range buffers {
		destination := destinations[index]
		headerLen := 3 + M.SocksaddrSerializer.AddrPortLen(destination)
		if buffer.Start() < headerLen {
			newBuffer := buf.NewSize(headerLen + buffer.Len())
			newBuffer.Resize(headerLen, 0)
			common.Must1(newBuffer.Write(buffer.Bytes()))
			buffer.Release()
			buffer = newBuffer
			buffers[index] = buffer
		}
		header := buf.With(buffer.ExtendHeader(headerLen))
		common.Must(header.WriteZeroN(3))
		err := M.SocksaddrSerializer.WriteAddrPort(header, destination)
		if err != nil {
			buf.ReleaseMulti(buffers)
			return err
		}
	}
	return w.writer.WriteConnectedPacketBatch(buffers)
}

func (w *associatePacketBatchWriter) Upstream() any {
	return w.writer
}

func (c *LazyAssociatePacketConn) CreatePacketBatchWriter() (N.PacketBatchWriter, bool) {
	writer, created := c.AssociatePacketConn.CreatePacketBatchWriter()
	if !created {
		return nil, false
	}
	return &lazyAssociatePacketBatchWriter{c, writer}, true
}

type lazyAssociatePacketBatchWriter struct {
	conn   *LazyAssociatePacketConn
	writer N.PacketBatchWriter
}

func (w *lazyAssociatePacketBatchWriter) WritePacketBatch(buffers []*buf.Buffer, destinations []M.Socksaddr) error {
	err := w.conn.HandshakeSuccess()
	if err != nil {
		buf.ReleaseMulti(buffers)
		return err
	}
	return w.writer.WritePacketBatch(buffers, destinations)
}

func (w *lazyAssociatePacketBatchWriter) Upstream() any { return w.writer }

var (
	_ N.PacketBatchReadWaitCreator = (*AssociatePacketConn)(nil)
	_ N.PacketBatchWriteCreator    = (*AssociatePacketConn)(nil)
	_ N.PacketBatchReadWaitCreator = (*LazyAssociatePacketConn)(nil)
	_ N.PacketBatchWriteCreator    = (*LazyAssociatePacketConn)(nil)
	_ N.PacketBatchReadWaiter      = (*associatePacketBatchReadWaiter)(nil)
	_ N.PacketBatchWriter          = (*associatePacketBatchWriter)(nil)
	_ N.PacketBatchReadWaiter      = (*lazyAssociatePacketBatchReadWaiter)(nil)
	_ N.PacketBatchWriter          = (*lazyAssociatePacketBatchWriter)(nil)
)
