package bufio

import (
	"errors"
	"io"
	"syscall"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func copyDirect(source syscall.Conn, destination syscall.Conn, readCounters []N.CountFunc, writeCounters []N.CountFunc) (handed bool, n int64, err error) {
	rawSource, err := source.SyscallConn()
	if err != nil {
		return
	}
	rawDestination, err := destination.SyscallConn()
	if err != nil {
		return
	}
	handed, n, err = splice(rawSource, rawDestination, readCounters, writeCounters)
	return
}

func copyWaitWithPool(originSource io.Reader, destination N.ExtendedWriter, source N.ExtendedReader, readWaiter N.ReadWaiter, vectorisedReadWaiter N.VectorisedReadWaiter, isVectorisedReadWaiter bool, options N.ReadWaitOptions, readCounters []N.CountFunc, writeCounters []N.CountFunc, increaseBufferAfter int64) (handled bool, n int64, err error) {
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
		if increaseBufferAfter > 0 && n >= increaseBufferAfter {
			if !isVectorisedReadWaiter {
				n, err = CopyExtendedChanWithPool(destination, source, readCounters, writeCounters, options, n)
			} else {
				n, err = copyWaitChanWithPool(destination, vectorisedReadWaiter, options, readCounters, writeCounters, n)
			}
			return
		}
	}
}

func copyWaitChanWithPool(destination N.ExtendedWriter, readWaiter N.VectorisedReadWaiter, options N.ReadWaitOptions, readCounters []N.CountFunc, writeCounters []N.CountFunc, inputN int64) (n int64, err error) {
	readWaiter.InitializeReadWaiter(options)
	vectorisedWriter, isVectorisedWriter := CreateVectorisedWriter(N.UnwrapWriter(destination))
	n += inputN
	sendChan := make(chan []*buf.Buffer, options.BatchSize)
	errChan := make(chan error, 1)
	go func() {
		var (
			buffers []*buf.Buffer
			readErr error
		)
		for {
			buffers, readErr = readWaiter.WaitReadBuffers()
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					errChan <- nil
				} else {
					errChan <- readErr
				}
				return
			}
			var dataLen int
			for _, buffer := range buffers {
				dataLen += buffer.Len()
			}
			sendChan <- buffers
			n += int64(dataLen)
			for _, counter := range readCounters {
				counter(int64(dataLen))
			}
		}
	}()
	for {
		select {
		case buffers := <-sendChan:
			if !isVectorisedWriter {
				var dataLen int
				for i, buffer := range buffers {
					dataLen += buffer.Len()
					err = destination.WriteBuffer(buffer)
					if err != nil {
						for _, buffer = range buffers[i:] {
							buffer.Leak()
						}
						return
					}
				}
				for _, counter := range writeCounters {
					counter(int64(dataLen))
				}
			} else {
			fetch:
				for {
					select {
					case newBuffers := <-sendChan:
						buffers = append(buffers, newBuffers...)
					default:
						break fetch
					}
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
				for _, counter := range writeCounters {
					counter(int64(dataLen))
				}
			}
		case err = <-errChan:
			return
		}
	}
}

func copyPacketWaitWithPool(originSource N.PacketReader, destinationConn N.PacketWriter, source N.PacketReadWaiter, readCounters []N.CountFunc, writeCounters []N.CountFunc, notFirstTime bool) (handled bool, n int64, err error) {
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
