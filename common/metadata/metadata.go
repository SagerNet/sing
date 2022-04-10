package metadata

import (
	"net"

	"github.com/sagernet/sing/common/buf"
)

type Metadata struct {
	Source      *AddrPort
	Destination *AddrPort
}

type TCPConnectionHandler interface {
	NewConnection(conn net.Conn, metadata Metadata) error
}

type UDPHandler interface {
	NewPacket(packet *buf.Buffer, metadata Metadata) error
}
