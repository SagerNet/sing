package mixed

import (
	std_bufio "bufio"
	"context"
	"io"
	"net"
	netHttp "net/http"
	"net/netip"
	"strings"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/redir"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/common/udpnat"
	"github.com/sagernet/sing/protocol/http"
	"github.com/sagernet/sing/protocol/socks"
	"github.com/sagernet/sing/protocol/socks/socks4"
	"github.com/sagernet/sing/protocol/socks/socks5"
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
	udpNat        *udpnat.Service[netip.AddrPort]
}

func NewListener(bind netip.AddrPort, authenticator auth.Authenticator, transproxy redir.TransproxyMode, udpTimeout int64, handler Handler) *Listener {
	listener := &Listener{
		bindAddr:      bind.Addr(),
		handler:       handler,
		authenticator: authenticator,
	}

	listener.TCPListener = tcp.NewTCPListener(bind, listener, tcp.WithTransproxyMode(transproxy))
	if transproxy == redir.ModeTProxy {
		listener.UDPListener = udp.NewUDPListener(bind, listener, udp.WithTransproxyMode(transproxy))
		listener.udpNat = udpnat.New[netip.AddrPort](udpTimeout, handler)
	}
	return listener
}

func (l *Listener) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	if metadata.Destination.IsValid() {
		return l.handler.NewConnection(ctx, conn, metadata)
	}
	headerType, err := rw.ReadByte(conn)
	switch headerType {
	case socks4.Version, socks5.Version:
		return socks.HandleConnection0(ctx, conn, headerType, l.authenticator, l.handler, metadata)
	}

	reader := std_bufio.NewReader(&bufio.BufferedReader{
		Reader: conn,
		Buffer: buf.As([]byte{headerType}),
	})

	request, err := http.ReadRequest(reader)
	if err != nil {
		return E.Cause(err, "read http request")
	}

	if request.Method == "GET" && request.URL.Path == "/proxy.pac" {
		content := newPAC(M.AddrPortFromNet(conn.LocalAddr()))
		response := &netHttp.Response{
			StatusCode: 200,
			Status:     netHttp.StatusText(200),
			Proto:      request.Proto,
			ProtoMajor: request.ProtoMajor,
			ProtoMinor: request.ProtoMinor,
			Header: netHttp.Header{
				"Content-Type": {"application/x-ns-proxy-autoconfig"},
			},
			ContentLength: int64(len(content)),
			Body:          io.NopCloser(strings.NewReader(content)),
		}
		err = response.Write(conn)
		if err != nil {
			return E.Cause(err, "write pac response")
		}
		return nil
	}

	if reader.Buffered() > 0 {
		_buffer := buf.StackNewSize(reader.Buffered())
		defer common.KeepAlive(_buffer)
		buffer := common.Dup(_buffer)
		_, err = buffer.ReadFullFrom(reader, reader.Buffered())
		if err != nil {
			return err
		}

		conn = &bufio.BufferedConn{
			Conn:   conn,
			Buffer: buffer,
		}
	}

	return http.HandleRequest(ctx, request, conn, l.authenticator, l.handler, metadata)
}

func (l *Listener) WriteIsThreadUnsafe() {
}

func (l *Listener) NewPacket(ctx context.Context, conn N.PacketConn, buffer *buf.Buffer, metadata M.Metadata) error {
	l.udpNat.NewPacket(ctx, metadata.Source.AddrPort(), func() N.PacketWriter {
		return &tproxyPacketWriter{metadata.Source.UDPAddr()}
	}, buffer, metadata)
	return nil
}

type tproxyPacketWriter struct {
	source *net.UDPAddr
}

func (w *tproxyPacketWriter) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	defer buffer.Release()
	udpConn, err := redir.DialUDP("udp", destination.UDPAddr(), w.source)
	if err != nil {
		return E.Cause(err, "tproxy udp write back")
	}
	defer udpConn.Close()
	return common.Error(udpConn.Write(buffer.Bytes()))
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
