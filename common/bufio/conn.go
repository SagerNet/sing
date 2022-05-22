package bufio

import (
	"context"
	"io"
	"net"
	"os"
	"runtime"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/common/task"
)

type PacketConnStub struct{}

func (s *PacketConnStub) RemoteAddr() net.Addr {
	return &common.DummyAddr{}
}

func (s *PacketConnStub) SetDeadline(t time.Time) error {
	return os.ErrInvalid
}

func (s *PacketConnStub) SetReadDeadline(t time.Time) error {
	return os.ErrInvalid
}

func (s *PacketConnStub) SetWriteDeadline(t time.Time) error {
	return os.ErrInvalid
}

func Copy(dst io.Writer, src io.Reader) (n int64, err error) {
	if wt, ok := src.(io.WriterTo); ok {
		return wt.WriteTo(dst)
	}
	if rt, ok := dst.(io.ReaderFrom); ok {
		return rt.ReadFrom(src)
	}
	extendedSrc, srcExtended := src.(N.ExtendedReader)
	extendedDst, dstExtended := dst.(N.ExtendedWriter)
	if !srcExtended && !dstExtended {
		_buffer := buf.StackNewMax()
		defer runtime.KeepAlive(_buffer)
		buffer := common.Dup(_buffer)
		for {
			buffer.FullReset()
			_, err = buffer.ReadFrom(src)
			if err != nil {
				return
			}
			var cn int
			cn, err = dst.Write(buffer.Bytes())
			n += int64(cn)
			if err != nil {
				return
			}
		}
	} else if !srcExtended {
		return CopyExtended(extendedDst, &ExtendedReaderWrapper{src})
	} else if !dstExtended {
		return CopyExtended(&ExtendedWriterWrapper{dst}, extendedSrc)
	} else {
		return CopyExtended(extendedDst, extendedSrc)
	}
}

func CopyExtended(dst N.ExtendedWriter, src N.ExtendedReader) (n int64, err error) {
	_buffer := buf.StackNewMax()
	defer runtime.KeepAlive(_buffer)
	buffer := common.Dup(_buffer)
	data := buffer.Cut(buf.ReversedHeader, buf.ReversedHeader)
	for {
		data.Reset()
		err = src.ReadBuffer(data)
		if err != nil {
			return
		}
		dataLen := data.Len()
		buffer.Resize(buf.ReversedHeader+data.Start(), dataLen)
		err = dst.WriteBuffer(buffer)
		if err != nil {
			return
		}
		n += int64(dataLen)
	}
}

func CopyConn(ctx context.Context, conn net.Conn, dest net.Conn) error {
	err := task.Run(ctx, func() error {
		defer rw.CloseRead(conn)
		defer rw.CloseWrite(dest)
		return common.Error(Copy(dest, conn))
	}, func() error {
		defer rw.CloseRead(dest)
		defer rw.CloseWrite(conn)
		return common.Error(Copy(conn, dest))
	})
	conn.Close()
	dest.Close()
	return err
}

func CopyExtendedConn(ctx context.Context, conn N.ExtendedConn, dest N.ExtendedConn) error {
	return task.Run(ctx, func() error {
		defer rw.CloseRead(conn)
		defer rw.CloseWrite(dest)
		_buffer := buf.StackNewMax()
		defer runtime.KeepAlive(_buffer)
		buffer := common.Dup(_buffer)
		data := buffer.Cut(buf.ReversedHeader, buf.ReversedHeader)
		for {
			data.Reset()
			_, err := data.ReadFrom(conn)
			if err != nil {
				return err
			}
			buffer.Resize(buf.ReversedHeader+data.Start(), data.Len())
			err = dest.WriteBuffer(buffer)
			if err != nil {
				return err
			}
		}
	}, func() error {
		defer rw.CloseRead(dest)
		defer rw.CloseWrite(conn)
		_buffer := buf.StackNewMax()
		defer runtime.KeepAlive(_buffer)
		buffer := common.Dup(_buffer)
		data := buffer.Cut(buf.ReversedHeader, buf.ReversedHeader)
		for {
			data.Reset()
			_, err := data.ReadFrom(dest)
			if err != nil {
				return err
			}
			buffer.Resize(buf.ReversedHeader+data.Start(), data.Len())
			err = conn.WriteBuffer(buffer)
			if err != nil {
				return err
			}
		}
	})
}

func CopyPacketConn(ctx context.Context, conn N.PacketConn, dest N.PacketConn) error {
	defer common.Close(conn, dest)
	return task.Run(ctx, func() error {
		_buffer := buf.StackNewMax()
		defer runtime.KeepAlive(_buffer)
		buffer := common.Dup(_buffer)
		data := buffer.Cut(buf.ReversedHeader, buf.ReversedHeader)
		for {
			data.FullReset()
			destination, err := conn.ReadPacket(data)
			if err != nil {
				return err
			}
			buffer.Resize(buf.ReversedHeader+data.Start(), data.Len())
			err = dest.WritePacket(buffer, destination)
			if err != nil {
				return err
			}
		}
	}, func() error {
		_buffer := buf.StackNewMax()
		defer runtime.KeepAlive(_buffer)
		buffer := common.Dup(_buffer)
		data := buffer.Cut(buf.ReversedHeader, buf.ReversedHeader)
		for {
			data.FullReset()
			destination, err := dest.ReadPacket(data)
			if err != nil {
				return err
			}
			buffer.Resize(buf.ReversedHeader+data.Start(), data.Len())
			err = conn.WritePacket(buffer, destination)
			if err != nil {
				return err
			}
		}
	})
}

func CopyNetPacketConn(ctx context.Context, conn N.PacketConn, dest net.PacketConn) error {
	if udpConn, ok := dest.(*net.UDPConn); ok {
		return CopyPacketConn(ctx, conn, &UDPConnWrapper{udpConn})
	} else {
		return CopyPacketConn(ctx, conn, &PacketConnWrapper{dest})
	}
}

type UDPConnWrapper struct {
	*net.UDPConn
}

func (w *UDPConnWrapper) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	n, addr, err := w.ReadFromUDPAddrPort(buffer.FreeBytes())
	if err != nil {
		return M.Socksaddr{}, err
	}
	buffer.Truncate(n)
	return M.SocksaddrFromNetIP(addr), nil
}

func (w *UDPConnWrapper) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	if destination.Family().IsFqdn() {
		udpAddr, err := net.ResolveUDPAddr("udp", destination.String())
		if err != nil {
			return err
		}
		return common.Error(w.UDPConn.WriteTo(buffer.Bytes(), udpAddr))
	}
	return common.Error(w.UDPConn.WriteToUDP(buffer.Bytes(), destination.UDPAddr()))
}

type PacketConnWrapper struct {
	net.PacketConn
}

func (p *PacketConnWrapper) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	_, addr, err := buffer.ReadPacketFrom(p)
	if err != nil {
		return M.Socksaddr{}, err
	}
	return M.SocksaddrFromNet(addr), err
}

func (p *PacketConnWrapper) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	return common.Error(p.WriteTo(buffer.Bytes(), destination.UDPAddr()))
}

type BindPacketConn struct {
	net.PacketConn
	Addr net.Addr
}

func (c *BindPacketConn) Read(b []byte) (n int, err error) {
	n, _, err = c.ReadFrom(b)
	return
}

func (c *BindPacketConn) Write(b []byte) (n int, err error) {
	return c.WriteTo(b, c.Addr)
}

func (c *BindPacketConn) RemoteAddr() net.Addr {
	return c.Addr
}

type ExtendedReaderWrapper struct {
	io.Reader
}

func (r *ExtendedReaderWrapper) ReadBuffer(buffer *buf.Buffer) error {
	n, err := r.Read(buffer.FreeBytes())
	if err != nil {
		return err
	}
	buffer.Truncate(n)
	return nil
}

type ExtendedWriterWrapper struct {
	io.Writer
}

func (w *ExtendedWriterWrapper) WriteBuffer(buffer *buf.Buffer) error {
	return common.Error(w.Write(buffer.Bytes()))
}
