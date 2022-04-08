package system

import (
	"io"
	"net"
	"net/netip"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/socksaddr"
	"github.com/sagernet/sing/protocol/socks"
)

type SocksHandler interface {
	NewConnection(addr socksaddr.Addr, port uint16, conn net.Conn) error
	NewPacketConnection(conn socks.PacketConn, addr socksaddr.Addr, port uint16) error
	OnError(err error)
}

type SocksConfig struct {
	Username string
	Password string
}

type SocksListener struct {
	Handler SocksHandler
	*TCPListener
	*SocksConfig
}

func NewSocksListener(bind netip.AddrPort, config *SocksConfig, handler SocksHandler) *SocksListener {
	listener := &SocksListener{
		SocksConfig: config,
		Handler:     handler,
	}
	listener.TCPListener = NewTCPListener(bind, listener)
	return listener
}

func (l *SocksListener) HandleTCP(conn net.Conn) error {
	authRequest, err := socks.ReadAuthRequest(conn)
	if err != nil {
		return exceptions.Cause(err, "read socks auth request")
	}
	var authMethod byte
	if l.Username == "" {
		authMethod = socks.AuthTypeNotRequired
	} else {
		authMethod = socks.AuthTypeUsernamePassword
	}
	if !common.Contains(authRequest.Methods, authMethod) {
		err = socks.WriteAuthResponse(conn, &socks.AuthResponse{
			Version: authRequest.Version,
			Method:  socks.AuthTypeNoAcceptedMethods,
		})
		if err != nil {
			return exceptions.Cause(err, "write socks auth response")
		}
	}
	err = socks.WriteAuthResponse(conn, &socks.AuthResponse{
		Version: authRequest.Version,
		Method:  socks.AuthTypeNotRequired,
	})
	if err != nil {
		return exceptions.Cause(err, "write socks auth response")
	}

	if authMethod == socks.AuthTypeUsernamePassword {
		usernamePasswordAuthRequest, err := socks.ReadUsernamePasswordAuthRequest(conn)
		if err != nil {
			return exceptions.Cause(err, "read user auth request")
		}
		response := socks.UsernamePasswordAuthResponse{}
		if usernamePasswordAuthRequest.Username != l.Username {
			response.Status = socks.UsernamePasswordStatusFailure
		} else if usernamePasswordAuthRequest.Password != l.Password {
			response.Status = socks.UsernamePasswordStatusFailure
		} else {
			response.Status = socks.UsernamePasswordStatusSuccess
		}
		err = socks.WriteUsernamePasswordAuthResponse(conn, &response)
		if err != nil {
			return exceptions.Cause(err, "write user auth response")
		}
	}

	request, err := socks.ReadRequest(conn)
	if err != nil {
		return exceptions.Cause(err, "read socks request")
	}
	switch request.Command {
	case socks.CommandConnect:
		localAddr, localPort := socksaddr.AddressFromNetAddr(l.TCPListener.TCPListener.Addr())
		err = socks.WriteResponse(conn, &socks.Response{
			Version:   request.Version,
			ReplyCode: socks.ReplyCodeSuccess,
			BindAddr:  localAddr,
			BindPort:  localPort,
		})
		if err != nil {
			return exceptions.Cause(err, "write socks response")
		}
		return l.Handler.NewConnection(request.Addr, request.Port, conn)
	case socks.CommandUDPAssociate:
		udpConn, err := net.ListenUDP("udp4", nil)
		if err != nil {
			return err
		}
		defer udpConn.Close()
		localAddr, localPort := socksaddr.AddressFromNetAddr(udpConn.LocalAddr())
		err = socks.WriteResponse(conn, &socks.Response{
			Version:   request.Version,
			ReplyCode: socks.ReplyCodeSuccess,
			BindAddr:  localAddr,
			BindPort:  localPort,
		})
		if err != nil {
			return exceptions.Cause(err, "write socks response")
		}
		go func() {
			err := l.Handler.NewPacketConnection(socks.NewPacketConn(conn, udpConn), request.Addr, request.Port)
			if err != nil {
				l.OnError(err)
			}
		}()
		return common.Error(io.Copy(io.Discard, conn))
	default:
		err = socks.WriteResponse(conn, &socks.Response{
			Version:   request.Version,
			ReplyCode: socks.ReplyCodeUnsupported,
		})
		if err != nil {
			return exceptions.Cause(err, "write response")
		}
	}
	return nil
}

func (l *SocksListener) Start() error {
	return l.TCPListener.Start()
}

func (l *SocksListener) Close() error {
	return l.TCPListener.Close()
}

func (l *SocksListener) OnError(err error) {
	l.Handler.OnError(exceptions.Cause(err, "socks server"))
}
