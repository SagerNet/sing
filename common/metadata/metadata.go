package metadata

import (
	"context"
	"net"
)

type Metadata struct {
	Protocol    string
	Source      *AddrPort
	Destination *AddrPort
}

type TCPConnectionHandler interface {
	NewConnection(ctx context.Context, conn net.Conn, metadata Metadata) error
}
