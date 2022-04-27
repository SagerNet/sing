package socks

import (
	"io"
	"net"
	"net/netip"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/transport/tcp"
)

type Handler interface {
	tcp.Handler
	UDPConnectionHandler
}

type Listener struct {
	bindAddr      netip.Addr
	tcpListener   *tcp.Listener
	authenticator auth.Authenticator
	handler       Handler
}

func NewListener(bind netip.AddrPort, authenticator auth.Authenticator, handler Handler) *Listener {
	listener := &Listener{
		bindAddr:      bind.Addr(),
		handler:       handler,
		authenticator: authenticator,
	}
	listener.tcpListener = tcp.NewTCPListener(bind, listener)
	return listener
}

func (l *Listener) NewConnection(conn net.Conn, metadata M.Metadata) error {
	return HandleConnection(conn, l.authenticator, l.bindAddr, l.handler, metadata)
}

func (l *Listener) Start() error {
	return l.tcpListener.Start()
}

func (l *Listener) Close() error {
	return l.tcpListener.Close()
}

func (l *Listener) HandleError(err error) {
	l.handler.HandleError(err)
}

func HandleConnection(conn net.Conn, authenticator auth.Authenticator, bind netip.Addr, handler Handler, metadata M.Metadata) error {
	authRequest, err := ReadAuthRequest(conn)
	if err != nil {
		return E.Cause(err, "read socks auth request")
	}
	var authMethod byte
	if authenticator == nil {
		authMethod = AuthTypeNotRequired
	} else {
		authMethod = AuthTypeUsernamePassword
	}
	if !common.Contains(authRequest.Methods, authMethod) {
		err = WriteAuthResponse(conn, &AuthResponse{
			Version: authRequest.Version,
			Method:  AuthTypeNoAcceptedMethods,
		})
		if err != nil {
			return E.Cause(err, "write socks auth response")
		}
	}
	err = WriteAuthResponse(conn, &AuthResponse{
		Version: authRequest.Version,
		Method:  authMethod,
	})
	if err != nil {
		return E.Cause(err, "write socks auth response")
	}

	if authMethod == AuthTypeUsernamePassword {
		usernamePasswordAuthRequest, err := ReadUsernamePasswordAuthRequest(conn)
		if err != nil {
			return E.Cause(err, "read user auth request")
		}
		response := &UsernamePasswordAuthResponse{}
		if authenticator.Verify(usernamePasswordAuthRequest.Username, usernamePasswordAuthRequest.Password) {
			response.Status = UsernamePasswordStatusSuccess
		} else {
			response.Status = UsernamePasswordStatusFailure
		}
		err = WriteUsernamePasswordAuthResponse(conn, response)
		if err != nil {
			return E.Cause(err, "write user auth response")
		}
	}

	request, err := ReadRequest(conn)
	if err != nil {
		return E.Cause(err, "read socks request")
	}
	switch request.Command {
	case CommandConnect:
		err = WriteResponse(conn, &Response{
			Version:   request.Version,
			ReplyCode: ReplyCodeSuccess,
			Bind:      M.AddrPortFromNetAddr(conn.LocalAddr()),
		})
		if err != nil {
			return E.Cause(err, "write socks response")
		}
		metadata.Protocol = "socks"
		metadata.Destination = request.Destination
		return handler.NewConnection(conn, metadata)
	case CommandUDPAssociate:
		network := "udp"
		if bind.Is4() {
			network = "udp4"
		}
		udpConn, err := net.ListenUDP(network, net.UDPAddrFromAddrPort(netip.AddrPortFrom(bind, 0)))
		if err != nil {
			return err
		}
		defer udpConn.Close()
		err = WriteResponse(conn, &Response{
			Version:   request.Version,
			ReplyCode: ReplyCodeSuccess,
			Bind:      M.AddrPortFromNetAddr(udpConn.LocalAddr()),
		})
		if err != nil {
			return E.Cause(err, "write socks response")
		}
		metadata.Destination = request.Destination
		go func() {
			err := handler.NewPacketConnection(NewPacketConn(conn, udpConn), metadata)
			if err != nil {
				handler.HandleError(err)
			}
		}()
		return common.Error(io.Copy(io.Discard, conn))
	default:
		err = WriteResponse(conn, &Response{
			Version:   request.Version,
			ReplyCode: ReplyCodeUnsupported,
		})
		if err != nil {
			return E.Cause(err, "write response")
		}
	}
	return nil
}
