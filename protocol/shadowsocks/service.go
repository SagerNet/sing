package shadowsocks

import (
	"context"

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

type MultiUserService[U comparable] interface {
	Service
	AddUser(user U, key []byte)
	RemoveUser(user U)
}

type UserContext[U comparable] struct {
	context.Context
	User U
}
