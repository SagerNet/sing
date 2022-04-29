package shadowsocks

import (
	"context"
	"net"

	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/udpnat"
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

type NoneService struct {
	handler Handler
	udp     *udpnat.Service[string]
}

func NewNoneService(handler Handler) Service {
	s := &NoneService{
		handler: handler,
	}
	s.udp = udpnat.New[string](s)
	return s
}

func (s *NoneService) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	destination, err := socks.AddressSerializer.ReadAddrPort(conn)
	if err != nil {
		return err
	}
	metadata.Protocol = "shadowsocks"
	metadata.Destination = destination
	return s.handler.NewConnection(ctx, conn, metadata)
}

func (s *NoneService) NewPacket(conn socks.PacketConn, buffer *buf.Buffer, metadata M.Metadata) error {
	destination, err := socks.AddressSerializer.ReadAddrPort(buffer)
	if err != nil {
		return err
	}
	metadata.Protocol = "shadowsocks"
	metadata.Destination = destination
	return s.udp.NewPacket(metadata.Source.String(), func() socks.PacketWriter {
		return &serverPacketWriter{conn, metadata.Source}
	}, buffer, metadata)
}

type serverPacketWriter struct {
	socks.PacketConn
	sourceAddr *M.AddrPort
}

func (s *serverPacketWriter) WritePacket(buffer *buf.Buffer, destination *M.AddrPort) error {
	header := buf.With(buffer.ExtendHeader(socks.AddressSerializer.AddrPortLen(destination)))
	err := socks.AddressSerializer.WriteAddrPort(header, destination)
	if err != nil {
		return err
	}
	return s.PacketConn.WritePacket(buffer, s.sourceAddr)
}

func (s *NoneService) NewPacketConnection(conn socks.PacketConn, metadata M.Metadata) error {
	return s.handler.NewPacketConnection(conn, metadata)
}

func (s *NoneService) HandleError(err error) {
	s.handler.HandleError(err)
}
