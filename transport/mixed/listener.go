package mixed

import (
	"net"
	"net/netip"

	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/redir"
	"github.com/sagernet/sing/common/udpnat"
	"github.com/sagernet/sing/protocol/http"
	"github.com/sagernet/sing/protocol/socks"
	"github.com/sagernet/sing/transport/tcp"
	"github.com/sagernet/sing/transport/udp"
)

type Handler interface {
	socks.Handler
}

type Listener struct {
	TCPListener   *tcp.Listener
	UDPListener   *udp.Listener
	handler       Handler
	authenticator auth.Authenticator
	udpNat        *udpnat.Server
}

func NewListener(bind netip.AddrPort, authenticator auth.Authenticator, transproxy redir.TransproxyMode, handler Handler) *Listener {
	listener := &Listener{
		handler:       handler,
		authenticator: authenticator,
	}

	listener.TCPListener = tcp.NewTCPListener(bind, listener, tcp.WithTransproxyMode(transproxy))
	if transproxy == redir.ModeTProxy {
		listener.UDPListener = udp.NewUDPListener(bind, listener, udp.WithTransproxyMode(transproxy))
		listener.udpNat = udpnat.NewServer(handler)
	}
	return listener
}

func (l *Listener) NewConnection(conn net.Conn, metadata M.Metadata) error {
	if metadata.Destination != nil {
		return l.handler.NewConnection(conn, metadata)
	}
	bufConn := buf.NewBufferedConn(conn)
	header, err := bufConn.Peek(1)
	if err != nil {
		return err
	}
	switch header[0] {
	case socks.Version4, socks.Version5:
		return socks.HandleConnection(bufConn, l.authenticator, l.handler)
	default:
		return http.HandleConnection(bufConn, l.authenticator, l.handler)
	}
}

func (l *Listener) NewPacket(packet *buf.Buffer, metadata M.Metadata) error {
	return l.udpNat.HandleUDP(packet, metadata)
}

func (l *Listener) HandleError(err error) {
	l.handler.HandleError(err)
}

func (l *Listener) Start() error {
	err := l.TCPListener.Start()
	if err != nil {
		return err
	}
	if l.UDPListener != nil {
		err = l.UDPListener.Start()
	}
	return err
}

func (l *Listener) Close() error {
	l.TCPListener.Close()
	if l.UDPListener != nil {
		l.UDPListener.Close()
	}
	return nil
}
