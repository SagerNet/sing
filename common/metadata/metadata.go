package metadata

import (
	"context"
	"net"

	"github.com/sagernet/sing/common/buf"
)

type Metadata struct {
	Protocol    string
	Source      *AddrPort
	Destination *AddrPort
}

type TCPConnectionHandler interface {
	NewConnection(ctx context.Context, conn net.Conn, metadata Metadata) error
}

type UDPHandler interface {
	NewPacket(packet *buf.Buffer, metadata Metadata) error
}
