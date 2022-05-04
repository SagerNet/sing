package metadata

import (
	"context"
	"net"
)

type Metadata struct {
	Protocol    string
	Source      Socksaddr
	Destination Socksaddr
}

type TCPConnectionHandler interface {
	NewConnection(ctx context.Context, conn net.Conn, metadata Metadata) error
}
