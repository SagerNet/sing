package bufio

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/common/task"
)

func Copy(destination io.Writer, source io.Reader) (n int64, err error) {
	if source == nil {
		return 0, E.New("nil reader")
	} else if destination == nil {
		return 0, E.New("nil writer")
	}
	originSource := source
	var readCounters, writeCounters []N.CountFunc
	for {
		source, readCounters = N.UnwrapCountReader(source, readCounters)
		destination, writeCounters = N.UnwrapCountWriter(destination, writeCounters)
		if cachedSrc, isCached := source.(N.CachedReader); isCached {
			cachedBuffer := cachedSrc.ReadCached()
			if cachedBuffer != nil {
				if !cachedBuffer.IsEmpty() {
					_, err = destination.Write(cachedBuffer.Bytes())
					if err != nil {
						cachedBuffer.Release()
						return
					}
				}
				cachedBuffer.Release()
				continue
			}
		}
		srcSyscallConn, srcIsSyscall := source.(syscall.Conn)
		dstSyscallConn, dstIsSyscall := destination.(syscall.Conn)
		if srcIsSyscall && dstIsSyscall {
			var handled bool
			handled, n, err = copyDirect(srcSyscallConn, dstSyscallConn, readCounters, writeCounters)
			if handled {
				return
			}
		}
		break
	}
	return CopyExtended(originSource, NewExtendedWriter(destination), NewExtendedReader(source), readCounters, writeCounters)
}

func CopyExtended(originSource io.Reader, destination N.ExtendedWriter, source N.ExtendedReader, readCounters []N.CountFunc, writeCounters []N.CountFunc) (n int64, err error) {
	frontHeadroom := N.CalculateFrontHeadroom(destination)
	rearHeadroom := N.CalculateRearHeadroom(destination)
	readWaiter, isReadWaiter := CreateReadWaiter(source)
	if isReadWaiter {
		needCopy := readWaiter.InitializeReadWaiter(N.ReadWaitOptions{
			FrontHeadroom: frontHeadroom,
			RearHeadroom:  rearHeadroom,
			MTU:           N.CalculateMTU(source, destination),
		})
		if !needCopy || common.LowMemory {
			var handled bool
			handled, n, err = copyWaitWithPool(originSource, destination, readWaiter, readCounters, writeCounters)
			if handled {
				return
			}
		}
	}
	return CopyExtendedWithPool(originSource, destination, source, readCounters, writeCounters)
}

func CopyExtendedBuffer(originSource io.Writer, destination N.ExtendedWriter, source N.ExtendedReader, buffer *buf.Buffer, readCounters []N.CountFunc, writeCounters []N.CountFunc) (n int64, err error) {
	buffer.IncRef()
	defer buffer.DecRef()
	frontHeadroom := N.CalculateFrontHeadroom(destination)
	rearHeadroom := N.CalculateRearHeadroom(destination)
	buffer.Resize(frontHeadroom, 0)
	buffer.Reserve(rearHeadroom)
	var notFirstTime bool
	for {
		err = source.ReadBuffer(buffer)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
				return
			}
			return
		}
		dataLen := buffer.Len()
		buffer.OverCap(rearHeadroom)
		err = destination.WriteBuffer(buffer)
		if err != nil {
			if !notFirstTime {
				err = N.ReportHandshakeFailure(originSource, err)
			}
			return
		}
		n += int64(dataLen)
		for _, counter := range readCounters {
			counter(int64(dataLen))
		}
		for _, counter := range writeCounters {
			counter(int64(dataLen))
		}
		notFirstTime = true
	}
}

func CopyExtendedWithPool(originSource io.Reader, destination N.ExtendedWriter, source N.ExtendedReader, readCounters []N.CountFunc, writeCounters []N.CountFunc) (n int64, err error) {
	frontHeadroom := N.CalculateFrontHeadroom(destination)
	rearHeadroom := N.CalculateRearHeadroom(destination)
	bufferSize := N.CalculateMTU(source, destination)
	if bufferSize > 0 {
		bufferSize += frontHeadroom + rearHeadroom
	} else {
		bufferSize = buf.BufferSize
	}
	var notFirstTime bool
	for {
		buffer := buf.NewSize(bufferSize)
		buffer.Resize(frontHeadroom, 0)
		buffer.Reserve(rearHeadroom)
		err = source.ReadBuffer(buffer)
		if err != nil {
			buffer.Release()
			if errors.Is(err, io.EOF) {
				err = nil
				return
			}
			return
		}
		dataLen := buffer.Len()
		buffer.OverCap(rearHeadroom)
		err = destination.WriteBuffer(buffer)
		if err != nil {
			buffer.Leak()
			if !notFirstTime {
				err = N.ReportHandshakeFailure(originSource, err)
			}
			return
		}
		n += int64(dataLen)
		for _, counter := range readCounters {
			counter(int64(dataLen))
		}
		for _, counter := range writeCounters {
			counter(int64(dataLen))
		}
		notFirstTime = true
	}
}

func CopyConn(ctx context.Context, source net.Conn, destination net.Conn) error {
	return CopyConnContextList([]context.Context{ctx}, source, destination)
}

func CopyConnContextList(contextList []context.Context, source net.Conn, destination net.Conn) error {
	var group task.Group
	if _, dstDuplex := common.Cast[rw.WriteCloser](destination); dstDuplex {
		group.Append("upload", func(ctx context.Context) error {
			err := common.Error(Copy(destination, source))
			if err == nil {
				rw.CloseWrite(destination)
			} else {
				common.Close(destination)
			}
			return err
		})
	} else {
		group.Append("upload", func(ctx context.Context) error {
			defer common.Close(destination)
			return common.Error(Copy(destination, source))
		})
	}
	if _, srcDuplex := common.Cast[rw.WriteCloser](source); srcDuplex {
		group.Append("download", func(ctx context.Context) error {
			err := common.Error(Copy(source, destination))
			if err == nil {
				rw.CloseWrite(source)
			} else {
				common.Close(source)
			}
			return err
		})
	} else {
		group.Append("download", func(ctx context.Context) error {
			defer common.Close(source)
			return common.Error(Copy(source, destination))
		})
	}
	group.Cleanup(func() {
		common.Close(source, destination)
	})
	return group.RunContextList(contextList)
}

func CopyPacket(destinationConn N.PacketWriter, source N.PacketReader) (n int64, err error) {
	var readCounters, writeCounters []N.CountFunc
	var cachedPackets []*N.PacketBuffer
	originSource := source
	for {
		source, readCounters = N.UnwrapCountPacketReader(source, readCounters)
		destinationConn, writeCounters = N.UnwrapCountPacketWriter(destinationConn, writeCounters)
		if cachedReader, isCached := source.(N.CachedPacketReader); isCached {
			packet := cachedReader.ReadCachedPacket()
			if packet != nil {
				cachedPackets = append(cachedPackets, packet)
				continue
			}
		}
		break
	}
	if cachedPackets != nil {
		n, err = WritePacketWithPool(originSource, destinationConn, cachedPackets)
		if err != nil {
			return
		}
	}
	frontHeadroom := N.CalculateFrontHeadroom(destinationConn)
	rearHeadroom := N.CalculateRearHeadroom(destinationConn)
	var (
		handled bool
		copeN   int64
	)
	readWaiter, isReadWaiter := CreatePacketReadWaiter(source)
	if isReadWaiter {
		needCopy := readWaiter.InitializeReadWaiter(N.ReadWaitOptions{
			FrontHeadroom: frontHeadroom,
			RearHeadroom:  rearHeadroom,
			MTU:           N.CalculateMTU(source, destinationConn),
		})
		if !needCopy || common.LowMemory {
			handled, copeN, err = copyPacketWaitWithPool(originSource, destinationConn, readWaiter, readCounters, writeCounters, n > 0)
			if handled {
				n += copeN
				return
			}
		}
	}
	copeN, err = CopyPacketWithPool(originSource, destinationConn, source, readCounters, writeCounters, n > 0)
	n += copeN
	return
}

func CopyPacketWithPool(originSource N.PacketReader, destinationConn N.PacketWriter, source N.PacketReader, readCounters []N.CountFunc, writeCounters []N.CountFunc, notFirstTime bool) (n int64, err error) {
	frontHeadroom := N.CalculateFrontHeadroom(destinationConn)
	rearHeadroom := N.CalculateRearHeadroom(destinationConn)
	bufferSize := N.CalculateMTU(source, destinationConn)
	if bufferSize > 0 {
		bufferSize += frontHeadroom + rearHeadroom
	} else {
		bufferSize = buf.UDPBufferSize
	}
	var destination M.Socksaddr
	for {
		buffer := buf.NewSize(bufferSize)
		buffer.Resize(frontHeadroom, 0)
		buffer.Reserve(rearHeadroom)
		destination, err = source.ReadPacket(buffer)
		if err != nil {
			buffer.Release()
			return
		}
		dataLen := buffer.Len()
		buffer.OverCap(rearHeadroom)
		err = destinationConn.WritePacket(buffer, destination)
		if err != nil {
			buffer.Leak()
			if !notFirstTime {
				err = N.ReportHandshakeFailure(originSource, err)
			}
			return
		}
		n += int64(dataLen)
		for _, counter := range readCounters {
			counter(int64(dataLen))
		}
		for _, counter := range writeCounters {
			counter(int64(dataLen))
		}
		notFirstTime = true
	}
}

func WritePacketWithPool(originSource N.PacketReader, destinationConn N.PacketWriter, packetBuffers []*N.PacketBuffer) (n int64, err error) {
	frontHeadroom := N.CalculateFrontHeadroom(destinationConn)
	rearHeadroom := N.CalculateRearHeadroom(destinationConn)
	var notFirstTime bool
	for _, packetBuffer := range packetBuffers {
		buffer := buf.NewPacket()
		buffer.Resize(frontHeadroom, 0)
		buffer.Reserve(rearHeadroom)
		_, err = buffer.Write(packetBuffer.Buffer.Bytes())
		packetBuffer.Buffer.Release()
		if err != nil {
			buffer.Release()
			continue
		}
		dataLen := buffer.Len()
		buffer.OverCap(rearHeadroom)
		err = destinationConn.WritePacket(buffer, packetBuffer.Destination)
		if err != nil {
			buffer.Leak()
			if !notFirstTime {
				err = N.ReportHandshakeFailure(originSource, err)
			}
			return
		}
		n += int64(dataLen)
	}
	return
}

func CopyPacketConn(ctx context.Context, source N.PacketConn, destination N.PacketConn) error {
	return CopyPacketConnContextList([]context.Context{ctx}, source, destination)
}

func CopyPacketConnContextList(contextList []context.Context, source N.PacketConn, destination N.PacketConn) error {
	var group task.Group
	group.Append("upload", func(ctx context.Context) error {
		return common.Error(CopyPacket(destination, source))
	})
	group.Append("download", func(ctx context.Context) error {
		return common.Error(CopyPacket(source, destination))
	})
	group.Cleanup(func() {
		common.Close(source, destination)
	})
	group.FastFail()
	return group.RunContextList(contextList)
}
