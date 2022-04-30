package trojan

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/protocol/socks"
)

const (
	KeyLength  = 56
	CommandTCP = 1
	CommandUDP = 3
)

var CRLF = []byte{'\r', '\n'}

type ClientConn struct {
	net.Conn
	key           [KeyLength]byte
	destination   *M.AddrPort
	headerWritten bool
}

func NewClientConn(conn net.Conn, key [KeyLength]byte, destination *M.AddrPort) *ClientConn {
	return &ClientConn{
		Conn:        conn,
		key:         key,
		destination: destination,
	}
}

func (c *ClientConn) Write(p []byte) (n int, err error) {
	if c.headerWritten {
		return c.Conn.Write(p)
	}
	err = ClientHandshake(c.Conn, c.key, c.destination, p)
	if err != nil {
		return
	}
	n = len(p)
	c.headerWritten = true
	return
}

func (c *ClientConn) ReadFrom(r io.Reader) (n int64, err error) {
	if !c.headerWritten {
		return rw.ReadFrom0(c, r)
	}
	return io.Copy(c.Conn, r)
}

func (c *ClientConn) WriteTo(w io.Writer) (n int64, err error) {
	return io.Copy(w, c.Conn)
}

type ClientPacketConn struct {
	net.Conn
	key           [KeyLength]byte
	headerWritten bool
}

func NewClientPacketConn(conn net.Conn, key [KeyLength]byte) *ClientPacketConn {
	return &ClientPacketConn{
		Conn: conn,
		key:  key,
	}
}

func (c *ClientPacketConn) ReadPacket(buffer *buf.Buffer) (*M.AddrPort, error) {
	return ReadPacket(c.Conn, buffer)
}

func (c *ClientPacketConn) WritePacket(buffer *buf.Buffer, destination *M.AddrPort) error {
	if !c.headerWritten {
		return ClientHandshakePacket(c.Conn, c.key, destination, buffer)
	}
	return WritePacket(c.Conn, buffer, destination)
}

func (c *ClientPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	buffer := buf.With(p)
	destination, err := c.ReadPacket(buffer)
	if err != nil {
		return
	}
	n = buffer.Len()
	addr = destination.UDPAddr()
	return
}

func (c *ClientPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	err = c.WritePacket(buf.With(p), M.AddrPortFromNetAddr(addr))
	if err == nil {
		n = len(p)
	}
	return
}

func Key(password string) [KeyLength]byte {
	var key [KeyLength]byte
	hash := sha256.New224()
	common.Must1(hash.Write([]byte(password)))
	hex.Encode(key[:], hash.Sum(nil))
	return key
}

func ClientHandshakeRaw(conn net.Conn, key [KeyLength]byte, command byte, destination *M.AddrPort, payload []byte) error {
	_, err := conn.Write(key[:])
	if err != nil {
		return err
	}
	_, err = conn.Write(CRLF[:])
	if err != nil {
		return err
	}
	_, err = conn.Write([]byte{command})
	if err != nil {
		return err
	}
	err = socks.AddressSerializer.WriteAddrPort(conn, destination)
	if err != nil {
		return err
	}
	_, err = conn.Write(CRLF[:])
	if err != nil {
		return err
	}
	if len(payload) > 0 {
		_, err = conn.Write(payload)
		if err != nil {
			return err
		}
	}
	return nil
}

func ClientHandshake(conn net.Conn, key [KeyLength]byte, destination *M.AddrPort, payload []byte) error {
	headerLen := KeyLength + socks.AddressSerializer.AddrPortLen(destination) + 5
	var header *buf.Buffer
	var writeHeader bool
	if len(payload) > 0 && headerLen+len(payload) < 65535 {
		buffer := buf.Make(headerLen + len(payload))
		header = buf.With(common.Dup(buffer))
	} else {
		buffer := buf.Make(headerLen)
		header = buf.With(common.Dup(buffer))
		writeHeader = true
	}
	common.Must1(header.Write(key[:]))
	common.Must1(header.Write(CRLF))
	common.Must(header.WriteByte(CommandTCP))
	common.Must(socks.AddressSerializer.WriteAddrPort(header, destination))
	common.Must1(header.Write(CRLF))
	common.Must1(header.Write(payload))

	_, err := conn.Write(header.Bytes())
	if err != nil {
		return E.Cause(err, "write request")
	}

	if writeHeader {
		_, err = conn.Write(payload)
		if err != nil {
			return E.Cause(err, "write payload")
		}
	}
	return nil
}

func ClientHandshakePacket(conn net.Conn, key [KeyLength]byte, destination *M.AddrPort, payload *buf.Buffer) error {
	headerLen := KeyLength + 2*socks.AddressSerializer.AddrPortLen(destination) + 9
	payloadLen := payload.Len()
	var header *buf.Buffer
	var writeHeader bool
	if payload.Start() >= headerLen {
		header = buf.With(payload.ExtendHeader(headerLen))
	} else {
		buffer := buf.Make(headerLen)
		header = buf.With(common.Dup(buffer))
		writeHeader = true
	}
	common.Must1(header.Write(key[:]))
	common.Must1(header.Write(CRLF))
	common.Must(header.WriteByte(CommandUDP))
	common.Must(socks.AddressSerializer.WriteAddrPort(header, destination))
	common.Must1(header.Write(CRLF))
	common.Must(socks.AddressSerializer.WriteAddrPort(header, destination))
	common.Must(binary.Write(header, binary.BigEndian, uint16(payloadLen)))
	common.Must1(header.Write(CRLF))

	if writeHeader {
		_, err := conn.Write(header.Bytes())
		if err != nil {
			return E.Cause(err, "write request")
		}
	}

	_, err := conn.Write(payload.Bytes())
	if err != nil {
		return E.Cause(err, "write payload")
	}
	return nil
}

func ReadPacket(conn net.Conn, buffer *buf.Buffer) (*M.AddrPort, error) {
	destination, err := socks.AddressSerializer.ReadAddrPort(conn)
	if err != nil {
		return nil, E.Cause(err, "read destination")
	}

	var length uint16
	err = binary.Read(conn, binary.BigEndian, &length)
	if err != nil {
		return nil, E.Cause(err, "read chunk length")
	}

	if buffer.FreeLen() < int(length) {
		return nil, io.ErrShortBuffer
	}

	err = rw.SkipN(conn, 2)
	if err != nil {
		return nil, E.Cause(err, "skip crlf")
	}

	_, err = buffer.ReadFullFrom(conn, int(length))
	return destination, err
}

func WritePacket(conn net.Conn, buffer *buf.Buffer, destination *M.AddrPort) error {
	headerOverload := socks.AddressSerializer.AddrPortLen(destination) + 4
	var header *buf.Buffer
	var writeHeader bool
	bufferLen := buffer.Len()
	if buffer.Start() >= headerOverload {
		header = buf.With(buffer.ExtendHeader(headerOverload))
	} else {
		writeHeader = true
		_buffer := buf.Make(headerOverload)
		header = buf.With(common.Dup(_buffer))
	}
	common.Must(socks.AddressSerializer.WriteAddrPort(header, destination))
	common.Must(binary.Write(header, binary.BigEndian, uint16(bufferLen)))
	common.Must1(header.Write(CRLF))
	if writeHeader {
		_, err := conn.Write(header.Bytes())
		if err != nil {
			return E.Cause(err, "write packet header")
		}
	}
	_, err := conn.Write(buffer.Bytes())
	if err != nil {
		return E.Cause(err, "write packet")
	}
	return nil
}
