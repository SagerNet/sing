package bufio

import (
	"errors"
	"io"
	"os"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func copyDirect(source io.Reader, destination io.Writer, readCounters []N.CountFunc, writeCounters []N.CountFunc) (handed bool, n int64, err error) {
	if !N.SyscallAvailableForRead(source) || !N.SyscallAvailableForWrite(destination) {
		return
	}
	sourceReader, sourceConn := N.SyscallConnForRead(source)
	destinationWriter, destinationConn := N.SyscallConnForWrite(destination)
	if sourceConn == nil || destinationConn == nil {
		return
	}
	handed, n, err = splice(sourceConn, sourceReader, destinationConn, destinationWriter, readCounters, writeCounters)
	return
}

func copyWaitWithPool(session *CopySession, destination N.ExtendedWriter, source N.ExtendedReader, readWaiter N.ReadWaiter, options N.ReadWaitOptions) (handled bool, n int64, err error) {
	handled = true
	var (
		buffer       *buf.Buffer
		notFirstTime bool
	)
	for {
		buffer, err = readWaiter.WaitReadBuffer()
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
				return
			}
			return
		}
		dataLen := buffer.Len()
		err = destination.WriteBuffer(buffer)
		if err != nil {
			buffer.Leak()
			if !notFirstTime {
				err = N.ReportHandshakeFailure(session.originSource, err)
			}
			return
		}
		n += int64(dataLen)
		if err = session.Transfer(int64(dataLen)); err != nil {
			return
		}
		notFirstTime = true
		if !options.IncreaseBuffer && session.options.IncreaseBufferAfter > 0 && n >= session.options.IncreaseBufferAfter {
			options.IncreaseBuffer = true
			vectorisedReadWaiter, isVectorisedReadWaiter := CreateVectorisedReadWaiter(source)
			vectorisedWriter, isVectorisedWriter := CreateVectorisedWriter(destination)
			if !isVectorisedReadWaiter || !isVectorisedWriter {
				readWaiter.InitializeReadWaiter(options)
				continue
			} else {
				vectorisedReadWaiter.InitializeReadWaiter(options)
			}
			n, err = copyWaitVectorisedWithPool(session, vectorisedWriter, vectorisedReadWaiter, n)
			return
		}
	}
}

func copyWaitVectorisedWithPool(session *CopySession, vectorisedWriter N.VectorisedWriter, readWaiter N.VectorisedReadWaiter, inputN int64) (n int64, err error) {
	n += inputN
	var buffers []*buf.Buffer
	for {
		buffers, err = readWaiter.WaitReadBuffers()
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
				return
			}
			return
		}
		var dataLen int
		for _, buffer := range buffers {
			dataLen += buffer.Len()
		}
		err = vectorisedWriter.WriteVectorised(buffers)
		if err != nil {
			for _, buffer := range buffers {
				buffer.Leak()
			}
			return
		}
		n += int64(dataLen)
		if err = session.Transfer(int64(dataLen)); err != nil {
			return
		}
	}
}

func copyPacketWaitWithPool(session *packetCopySession, destinationConn N.PacketWriter, source N.PacketReadWaiter, notFirstTime bool) (handled bool, n int64, err error) {
	handled = true
	var (
		buffer      *buf.Buffer
		destination M.Socksaddr
	)
	for {
		buffer, destination, err = source.WaitReadPacket()
		if err != nil {
			return
		}
		dataLen := buffer.Len()
		err = destinationConn.WritePacket(buffer, destination)
		if err != nil {
			buffer.Leak()
			if !notFirstTime {
				handshakeErr := N.ReportHandshakeFailure(session.originSource, err)
				if handshakeErr != nil {
					err = handshakeErr
				}
			}
			return
		}
		n += int64(dataLen)
		if err = session.Transfer(int64(dataLen)); err != nil {
			return
		}
		notFirstTime = true
	}
}

func copyPacketBatchWaitWithPool(session *packetCopySession, destinationConn N.PacketBatchWriter, source N.PacketBatchReadWaiter, notFirstTime bool) (handled bool, n int64, err error) {
	handled = true
	for {
		var (
			buffers      []*buf.Buffer
			destinations []M.Socksaddr
		)
		buffers, destinations, err = source.WaitReadPackets()
		if err != nil {
			return handled, n, err
		}
		if len(buffers) == 0 || len(buffers) != len(destinations) {
			buf.ReleaseMulti(buffers)
			return handled, n, os.ErrInvalid
		}
		dataLens := make([]int, len(buffers))
		for index, buffer := range buffers {
			dataLens[index] = buffer.Len()
		}
		err = destinationConn.WritePacketBatch(buffers, destinations)
		if err != nil {
			if !notFirstTime {
				handshakeErr := N.ReportHandshakeFailure(session.originSource, err)
				if handshakeErr != nil {
					err = handshakeErr
				}
			}
			return
		}
		for _, dataLen := range dataLens {
			n += int64(dataLen)
		}
		if err = session.TransferBatch(dataLens); err != nil {
			return
		}
		notFirstTime = true
	}
}

func copyPacketBatchToConnectedWaitWithPool(session *packetCopySession, destinationConn N.ConnectedPacketBatchWriter, source N.PacketBatchReadWaiter, notFirstTime bool) (handled bool, n int64, err error) {
	handled = true
	for {
		var (
			buffers      []*buf.Buffer
			destinations []M.Socksaddr
		)
		buffers, destinations, err = source.WaitReadPackets()
		if err != nil {
			return handled, n, err
		}
		if len(buffers) == 0 || len(buffers) != len(destinations) {
			buf.ReleaseMulti(buffers)
			return handled, n, os.ErrInvalid
		}
		dataLens := make([]int, len(buffers))
		for index, buffer := range buffers {
			dataLens[index] = buffer.Len()
		}
		err = destinationConn.WriteConnectedPacketBatch(buffers)
		if err != nil {
			if !notFirstTime {
				handshakeErr := N.ReportHandshakeFailure(session.originSource, err)
				if handshakeErr != nil {
					err = handshakeErr
				}
			}
			return
		}
		for _, dataLen := range dataLens {
			n += int64(dataLen)
		}
		if err = session.TransferBatch(dataLens); err != nil {
			return
		}
		notFirstTime = true
	}
}

func copyConnectedPacketBatchWaitWithPool(session *packetCopySession, destinationConn N.PacketBatchWriter, source N.ConnectedPacketBatchReadWaiter, notFirstTime bool) (handled bool, n int64, err error) {
	handled = true
	for {
		var (
			buffers     []*buf.Buffer
			destination M.Socksaddr
		)
		buffers, destination, err = source.WaitReadConnectedPackets()
		if err != nil {
			return handled, n, err
		}
		if len(buffers) == 0 {
			buf.ReleaseMulti(buffers)
			return handled, n, os.ErrInvalid
		}
		destinations := make([]M.Socksaddr, len(buffers))
		dataLens := make([]int, len(buffers))
		for index, buffer := range buffers {
			destinations[index] = destination
			dataLens[index] = buffer.Len()
		}
		err = destinationConn.WritePacketBatch(buffers, destinations)
		if err != nil {
			if !notFirstTime {
				handshakeErr := N.ReportHandshakeFailure(session.originSource, err)
				if handshakeErr != nil {
					err = handshakeErr
				}
			}
			return
		}
		for _, dataLen := range dataLens {
			n += int64(dataLen)
		}
		if err = session.TransferBatch(dataLens); err != nil {
			return
		}
		notFirstTime = true
	}
}

func copyConnectedPacketBatchToConnectedWaitWithPool(session *packetCopySession, destinationConn N.ConnectedPacketBatchWriter, source N.ConnectedPacketBatchReadWaiter, notFirstTime bool) (handled bool, n int64, err error) {
	handled = true
	for {
		var buffers []*buf.Buffer
		buffers, _, err = source.WaitReadConnectedPackets()
		if err != nil {
			return handled, n, err
		}
		if len(buffers) == 0 {
			buf.ReleaseMulti(buffers)
			return handled, n, os.ErrInvalid
		}
		dataLens := make([]int, len(buffers))
		for index, buffer := range buffers {
			dataLens[index] = buffer.Len()
		}
		err = destinationConn.WriteConnectedPacketBatch(buffers)
		if err != nil {
			if !notFirstTime {
				handshakeErr := N.ReportHandshakeFailure(session.originSource, err)
				if handshakeErr != nil {
					err = handshakeErr
				}
			}
			return
		}
		for _, dataLen := range dataLens {
			n += int64(dataLen)
		}
		if err = session.TransferBatch(dataLens); err != nil {
			return
		}
		notFirstTime = true
	}
}
