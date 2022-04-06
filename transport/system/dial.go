package system

import (
	"context"
	"net"
)

type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

var Dial DialFunc = new(net.Dialer).DialContext
