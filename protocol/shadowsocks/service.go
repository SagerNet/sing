package shadowsocks

import (
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/protocol/socks"
)

type Service interface {
	M.TCPConnectionHandler
	socks.UDPHandler
}

type Handler interface {
	M.TCPConnectionHandler
	socks.UDPConnectionHandler
	E.Handler
}
