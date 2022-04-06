package session

import (
	"net"
	"strconv"

	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/socksaddr"
)

type Network int

const (
	NetworkTCP Network = iota
	NetworkUDP
)

type InstanceContext struct{}

type Context struct {
	InstanceContext
	Network         Network
	Source          socksaddr.Addr
	Destination     socksaddr.Addr
	SourcePort      uint16
	DestinationPort uint16
}

func (c Context) DestinationNetAddr() string {
	return net.JoinHostPort(c.Destination.String(), strconv.Itoa(int(c.DestinationPort)))
}

func AddressFromNetAddr(netAddr net.Addr) (addr socksaddr.Addr, port uint16) {
	var ip net.IP
	switch addr := netAddr.(type) {
	case *net.TCPAddr:
		ip = addr.IP
		port = uint16(addr.Port)
	case *net.UDPAddr:
		ip = addr.IP
		port = uint16(addr.Port)
	}
	return socksaddr.AddrFromIP(ip), port
}

type Conn struct {
	Conn    net.Conn
	Context *Context
}

type PacketConn struct {
	Conn    net.PacketConn
	Context *Context
}

type Packet struct {
	Context   *Context
	Data      *buf.Buffer
	WriteBack func(buffer *buf.Buffer, addr *net.UDPAddr) error
	Release   func()
}

type Handler interface {
	HandleConnection(conn *Conn)
	HandlePacket(packet *Packet)
}
