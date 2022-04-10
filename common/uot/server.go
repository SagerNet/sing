package uot

import (
	"encoding/binary"
	"io"
	"net"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
)

type ServerConn struct {
	net.PacketConn
	inputReader, outputReader *io.PipeReader
	inputWriter, outputWriter *io.PipeWriter
}

func NewServerConn(packetConn net.PacketConn) net.Conn {
	c := &ServerConn{
		PacketConn: packetConn,
	}
	c.inputReader, c.inputWriter = io.Pipe()
	c.outputReader, c.outputWriter = io.Pipe()
	go c.loopInput()
	go c.loopOutput()
	return c
}

func (c *ServerConn) Read(b []byte) (n int, err error) {
	return c.outputReader.Read(b)
}

func (c *ServerConn) Write(b []byte) (n int, err error) {
	return c.inputWriter.Write(b)
}

func (c *ServerConn) RemoteAddr() net.Addr {
	return &common.DummyAddr{}
}

func (c *ServerConn) loopInput() {
	buffer := buf.New()
	defer buffer.Release()
	for {
		destination, err := AddrParser.ReadAddrPort(c.inputReader)
		if err != nil {
			break
		}
		if destination.Addr.Family().IsFqdn() {
			ip, err := LookupAddress(destination.Addr.Fqdn())
			if err != nil {
				break
			}
			destination.Addr = M.AddrFromIP(ip)
		}
		var length uint16
		err = binary.Read(c.inputReader, binary.BigEndian, &length)
		if err != nil {
			break
		}
		buffer.FullReset()
		_, err = buffer.ReadFullFrom(c.inputReader, int(length))
		if err != nil {
			break
		}
		_, err = c.WriteTo(buffer.Bytes(), destination.UDPAddr())
		if err != nil {
			break
		}
	}
	c.Close()
}

func (c *ServerConn) loopOutput() {
	buffer := buf.New()
	defer buffer.Release()
	for {
		buffer.FullReset()
		n, addr, err := buffer.ReadPacketFrom(c)
		if err != nil {
			break
		}
		destination := M.AddrPortFromNetAddr(addr)
		err = AddrParser.WriteAddrPort(c.outputWriter, destination)
		if err != nil {
			break
		}
		err = binary.Write(c.outputWriter, binary.BigEndian, uint16(n))
		if err != nil {
			break
		}
		_, err = buffer.WriteTo(c.outputWriter)
		if err != nil {
			break
		}
	}
	c.Close()
}

func (c *ServerConn) Close() error {
	c.inputReader.Close()
	c.inputWriter.Close()
	c.outputReader.Close()
	c.outputWriter.Close()
	c.PacketConn.Close()
	return nil
}
