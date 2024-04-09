package bufio

import (
	"net"
	"net/netip"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type NATPacketConn interface {
	N.NetPacketConn
	UpdateDestination(destinationAddress netip.Addr)
}

func NewUnidirectionalNATPacketConn(conn N.NetPacketConn, origin M.Socksaddr, destination M.Socksaddr) NATPacketConn {
	return &unidirectionalNATPacketConn{
		NetPacketConn: conn,
		origin:        origin,
		destination:   destination,
	}
}

func NewNATPacketConn(conn N.NetPacketConn, origin M.Socksaddr, destination M.Socksaddr) NATPacketConn {
	return &bidirectionalNATPacketConn{
		NetPacketConn: conn,
		origin:        origin,
		destination:   destination,
	}
}

type unidirectionalNATPacketConn struct {
	N.NetPacketConn
	origin      M.Socksaddr
	destination M.Socksaddr
}

func (c *unidirectionalNATPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	destination := M.SocksaddrFromNet(addr)
	if destination == c.destination {
		destination = c.origin
	}
	if c.destination.Port == c.origin.Port && destination.AddrString() == c.destination.AddrString() {
		destination.Addr = c.origin.Addr
		destination.Fqdn = c.origin.Fqdn
	}
	addr = destination.UDPAddr()
	return c.NetPacketConn.WriteTo(p, addr)
}

func (c *unidirectionalNATPacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	if destination == c.destination {
		destination = c.origin
	}
	if c.destination.Port == c.origin.Port && destination.AddrString() == c.destination.AddrString() {
		destination.Addr = c.origin.Addr
		destination.Fqdn = c.origin.Fqdn
	}
	return c.NetPacketConn.WritePacket(buffer, destination)
}

func (c *unidirectionalNATPacketConn) UpdateDestination(destinationAddress netip.Addr) {
	c.destination = M.SocksaddrFrom(destinationAddress, c.destination.Port)
}

func (c *unidirectionalNATPacketConn) Upstream() any {
	return c.NetPacketConn
}

type bidirectionalNATPacketConn struct {
	N.NetPacketConn
	origin      M.Socksaddr
	destination M.Socksaddr
}

func (c *bidirectionalNATPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, addr, err = c.NetPacketConn.ReadFrom(p)
	destination := M.SocksaddrFromNet(addr)
	if err == nil && destination == c.origin {
		destination = c.destination
	}
	if c.destination.Port == c.origin.Port && destination.AddrString() == c.origin.AddrString() {
		destination.Addr = c.destination.Addr
		destination.Fqdn = c.destination.Fqdn
	}
	addr = destination.UDPAddr()
	return
}

func (c *bidirectionalNATPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	destination := M.SocksaddrFromNet(addr)
	if destination == c.destination {
		destination = c.origin
	}
	if c.destination.Port == c.origin.Port && destination.AddrString() == c.destination.AddrString() {
		destination.Addr = c.origin.Addr
		destination.Fqdn = c.origin.Fqdn
	}
	addr = destination.UDPAddr()
	return c.NetPacketConn.WriteTo(p, addr)
}

func (c *bidirectionalNATPacketConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	destination, err = c.NetPacketConn.ReadPacket(buffer)
	if destination == c.origin {
		destination = c.destination
	}
	if c.destination.Port == c.origin.Port && destination.AddrString() == c.origin.AddrString() {
		destination.Addr = c.destination.Addr
		destination.Fqdn = c.destination.Fqdn
	}
	return
}

func (c *bidirectionalNATPacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	if destination == c.destination {
		destination = c.origin
	}
	if c.destination.Port == c.origin.Port && destination.AddrString() == c.destination.AddrString() {
		destination.Addr = c.origin.Addr
		destination.Fqdn = c.origin.Fqdn
	}
	return c.NetPacketConn.WritePacket(buffer, destination)
}

func (c *bidirectionalNATPacketConn) UpdateDestination(destinationAddress netip.Addr) {
	c.destination = M.SocksaddrFrom(destinationAddress, c.destination.Port)
}

func (c *bidirectionalNATPacketConn) Upstream() any {
	return c.NetPacketConn
}
