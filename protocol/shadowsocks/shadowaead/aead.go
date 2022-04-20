package shadowaead

import (
	"crypto/cipher"
	"encoding/binary"
	"io"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
)

// https://shadowsocks.org/en/wiki/AEAD-Ciphers.html
const (
	MaxPacketSize          = 16*1024 - 1
	PacketLengthBufferSize = 2
)

type Reader struct {
	upstream io.Reader
	cipher   cipher.AEAD
	buffer   []byte
	nonce    []byte
	index    int
	cached   int
}

func NewReader(upstream io.Reader, cipher cipher.AEAD, maxPacketSize int) *Reader {
	return &Reader{
		upstream: upstream,
		cipher:   cipher,
		buffer:   make([]byte, maxPacketSize+PacketLengthBufferSize+cipher.Overhead()*2),
		nonce:    make([]byte, cipher.NonceSize()),
	}
}

func NewRawReader(upstream io.Reader, cipher cipher.AEAD, buffer []byte, nonce []byte) *Reader {
	return &Reader{
		upstream: upstream,
		cipher:   cipher,
		buffer:   buffer,
		nonce:    nonce,
	}
}

func (r *Reader) Upstream() io.Reader {
	return r.upstream
}

func (r *Reader) Replaceable() bool {
	return false
}

func (r *Reader) SetUpstream(reader io.Reader) {
	r.upstream = reader
}

func (r *Reader) WriteTo(writer io.Writer) (n int64, err error) {
	if r.cached > 0 {
		writeN, writeErr := writer.Write(r.buffer[r.index : r.index+r.cached])
		if writeErr != nil {
			return int64(writeN), writeErr
		}
		n += int64(writeN)
	}
	for {
		start := PacketLengthBufferSize + r.cipher.Overhead()
		_, err = io.ReadFull(r.upstream, r.buffer[:start])
		if err != nil {
			return
		}
		_, err = r.cipher.Open(r.buffer[:0], r.nonce, r.buffer[:start], nil)
		if err != nil {
			return
		}
		increaseNonce(r.nonce)
		length := int(binary.BigEndian.Uint16(r.buffer[:PacketLengthBufferSize]))
		end := length + r.cipher.Overhead()
		_, err = io.ReadFull(r.upstream, r.buffer[:end])
		if err != nil {
			return
		}
		_, err = r.cipher.Open(r.buffer[:0], r.nonce, r.buffer[:end], nil)
		if err != nil {
			return
		}
		increaseNonce(r.nonce)
		writeN, writeErr := writer.Write(r.buffer[:length])
		if writeErr != nil {
			return int64(writeN), writeErr
		}
		n += int64(writeN)
	}
}

func (r *Reader) Read(b []byte) (n int, err error) {
	if r.cached > 0 {
		n = copy(b, r.buffer[r.index:r.index+r.cached])
		r.cached -= n
		r.index += n
		return
	}
	start := PacketLengthBufferSize + r.cipher.Overhead()
	_, err = io.ReadFull(r.upstream, r.buffer[:start])
	if err != nil {
		return 0, err
	}
	_, err = r.cipher.Open(r.buffer[:0], r.nonce, r.buffer[:start], nil)
	if err != nil {
		return 0, err
	}
	increaseNonce(r.nonce)
	length := int(binary.BigEndian.Uint16(r.buffer[:PacketLengthBufferSize]))
	end := length + r.cipher.Overhead()

	if len(b) >= end {
		data := b[:end]
		_, err = io.ReadFull(r.upstream, data)
		if err != nil {
			return 0, err
		}
		_, err = r.cipher.Open(b[:0], r.nonce, data, nil)
		if err != nil {
			return 0, err
		}
		increaseNonce(r.nonce)
		return length, nil
	} else {
		_, err = io.ReadFull(r.upstream, r.buffer[:end])
		if err != nil {
			return 0, err
		}
		_, err = r.cipher.Open(r.buffer[:0], r.nonce, r.buffer[:end], nil)
		if err != nil {
			return 0, err
		}
		increaseNonce(r.nonce)
		n = copy(b, r.buffer[:length])
		r.cached = length - n
		r.index = n
		return
	}
}

type Writer struct {
	upstream      io.Writer
	cipher        cipher.AEAD
	maxPacketSize int
	buffer        []byte
	nonce         []byte
}

func NewWriter(upstream io.Writer, cipher cipher.AEAD, maxPacketSize int) *Writer {
	return &Writer{
		upstream:      upstream,
		cipher:        cipher,
		buffer:        make([]byte, maxPacketSize+PacketLengthBufferSize+cipher.Overhead()*2),
		nonce:         make([]byte, cipher.NonceSize()),
		maxPacketSize: maxPacketSize,
	}
}

func NewRawWriter(upstream io.Writer, cipher cipher.AEAD, maxPacketSize int, buffer []byte, nonce []byte) *Writer {
	return &Writer{
		upstream:      upstream,
		cipher:        cipher,
		maxPacketSize: maxPacketSize,
		buffer:        buffer,
		nonce:         nonce,
	}
}

func (w *Writer) Upstream() io.Writer {
	return w.upstream
}

func (w *Writer) Replaceable() bool {
	return false
}

func (w *Writer) SetWriter(writer io.Writer) {
	w.upstream = writer
}

func (w *Writer) ReadFrom(r io.Reader) (n int64, err error) {
	for {
		offset := w.cipher.Overhead() + PacketLengthBufferSize
		readN, readErr := r.Read(w.buffer[offset : offset+w.maxPacketSize])
		if readErr != nil {
			return 0, readErr
		}
		binary.BigEndian.PutUint16(w.buffer[:PacketLengthBufferSize], uint16(readN))
		w.cipher.Seal(w.buffer[:0], w.nonce, w.buffer[:PacketLengthBufferSize], nil)
		increaseNonce(w.nonce)
		packet := w.cipher.Seal(w.buffer[offset:offset], w.nonce, w.buffer[offset:offset+readN], nil)
		increaseNonce(w.nonce)
		_, err = w.upstream.Write(w.buffer[:offset+len(packet)])
		if err != nil {
			return
		}
		err = common.FlushVar(&w.upstream)
		if err != nil {
			return
		}
		n += int64(readN)
	}
}

func (w *Writer) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return
	}

	for _, data := range buf.ForeachN(p, w.maxPacketSize) {
		binary.BigEndian.PutUint16(w.buffer[:PacketLengthBufferSize], uint16(len(data)))
		w.cipher.Seal(w.buffer[:0], w.nonce, w.buffer[:PacketLengthBufferSize], nil)
		increaseNonce(w.nonce)
		offset := w.cipher.Overhead() + PacketLengthBufferSize
		packet := w.cipher.Seal(w.buffer[offset:offset], w.nonce, data, nil)
		increaseNonce(w.nonce)
		_, err = w.upstream.Write(w.buffer[:offset+len(packet)])
		if err != nil {
			return
		}
		n += len(data)
	}

	return
}

func increaseNonce(nonce []byte) {
	for i := range nonce {
		nonce[i]++
		if nonce[i] != 0 {
			return
		}
	}
}
