package shadowsocks

import (
	"net"

	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/protocol/socks"
)

type Service interface {
	M.TCPConnectionHandler
}

type MultiUserService interface {
	Service
	AddUser(key []byte)
	RemoveUser(key []byte)
}

type Handler interface {
	M.TCPConnectionHandler
}

type NoneService struct {
	handler Handler
}

func NewNoneService(handler Handler) Service {
	return &NoneService{
		handler: handler,
	}
}

func (s *NoneService) NewConnection(conn net.Conn, metadata M.Metadata) error {
	destination, err := socks.AddressSerializer.ReadAddrPort(conn)
	if err != nil {
		return err
	}
	metadata.Protocol = "shadowsocks"
	metadata.Destination = destination
	return s.handler.NewConnection(conn, metadata)
}
