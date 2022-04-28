package mixed

import (
	"context"
	"io"
	"net"
	netHttp "net/http"
	"net/netip"
	"strings"

	"github.com/sagernet/sing"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
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
	bindAddr      netip.Addr
	handler       Handler
	authenticator auth.Authenticator
	udpNat        *udpnat.Server
}

func NewListener(bind netip.AddrPort, authenticator auth.Authenticator, transproxy redir.TransproxyMode, handler Handler) *Listener {
	listener := &Listener{
		bindAddr:      bind.Addr(),
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

func (l *Listener) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	if metadata.Destination != nil {
		return l.handler.NewConnection(ctx, conn, metadata)
	}
	bufConn := buf.NewBufferedConn(conn)
	header, err := bufConn.Peek(1)
	if err != nil {
		return err
	}
	switch header[0] {
	case socks.Version4:
		return E.New("socks4 request dropped (TODO)")
	case socks.Version5:
		return socks.HandleConnection(ctx, bufConn, l.authenticator, l.bindAddr, l.handler, metadata)
	}

	request, err := http.ReadRequest(bufConn.Reader())
	if err != nil {
		return E.Cause(err, "read http request")
	}

	if request.Method == "GET" && request.URL.Path == "/proxy.pac" {
		content := newPAC(M.AddrPortFromNetAddr(conn.LocalAddr()))
		response := &netHttp.Response{
			StatusCode: 200,
			Status:     netHttp.StatusText(200),
			Proto:      request.Proto,
			ProtoMajor: request.ProtoMajor,
			ProtoMinor: request.ProtoMinor,
			Header: netHttp.Header{
				"Content-Type": {"application/x-ns-proxy-autoconfig"},
				"Server":       {sing.VersionStr},
			},
			ContentLength: int64(len(content)),
			Body:          io.NopCloser(strings.NewReader(content)),
		}
		err = response.Write(bufConn)
		if err != nil {
			return E.Cause(err, "write pac response")
		}
		return nil
	}

	return http.HandleRequest(ctx, request, bufConn, l.authenticator, l.handler, metadata)
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
	return common.Close(
		l.TCPListener,
		l.UDPListener,
	)
}
