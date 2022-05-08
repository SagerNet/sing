package socks5

import (
	"context"
	"io"
	"net"
	"net/netip"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
)

func ClientHandshake(conn io.ReadWriter, version byte, command byte, destination M.Socksaddr, username string, password string) (*Response, error) {
	var method byte
	if common.IsBlank(username) {
		method = AuthTypeNotRequired
	} else {
		method = AuthTypeUsernamePassword
	}
	err := WriteAuthRequest(conn, &AuthRequest{
		Version: version,
		Methods: []byte{method},
	})
	if err != nil {
		return nil, err
	}
	authResponse, err := ReadAuthResponse(conn)
	if err != nil {
		return nil, err
	}
	if authResponse.Method != method {
		return nil, E.New("not requested method, request ", method, ", return ", method)
	}
	if method == AuthTypeUsernamePassword {
		err = WriteUsernamePasswordAuthRequest(conn, &UsernamePasswordAuthRequest{
			Username: username,
			Password: password,
		})
		if err != nil {
			return nil, err
		}
		usernamePasswordResponse, err := ReadUsernamePasswordAuthResponse(conn)
		if err != nil {
			return nil, err
		}
		if usernamePasswordResponse.Status == UsernamePasswordStatusFailure {
			return nil, &UsernamePasswordAuthFailureException{}
		}
	}
	err = WriteRequest(conn, &Request{
		Version:     version,
		Command:     command,
		Destination: destination,
	})
	if err != nil {
		return nil, err
	}
	return ReadResponse(conn)
}

func ClientFastHandshake(writer io.Writer, version byte, command byte, destination M.Socksaddr, username string, password string) error {
	var method byte
	if common.IsBlank(username) {
		method = AuthTypeNotRequired
	} else {
		method = AuthTypeUsernamePassword
	}
	err := WriteAuthRequest(writer, &AuthRequest{
		Version: version,
		Methods: []byte{method},
	})
	if err != nil {
		return err
	}
	if method == AuthTypeUsernamePassword {
		err = WriteUsernamePasswordAuthRequest(writer, &UsernamePasswordAuthRequest{
			Username: username,
			Password: password,
		})
		if err != nil {
			return err
		}
	}
	return WriteRequest(writer, &Request{
		Version:     version,
		Command:     command,
		Destination: destination,
	})
}

func ClientFastHandshakeFinish(reader io.Reader) (*Response, error) {
	response, err := ReadAuthResponse(reader)
	if err != nil {
		return nil, err
	}
	if response.Method == AuthTypeUsernamePassword {
		usernamePasswordResponse, err := ReadUsernamePasswordAuthResponse(reader)
		if err != nil {
			return nil, err
		}
		if usernamePasswordResponse.Status == UsernamePasswordStatusFailure {
			return nil, &UsernamePasswordAuthFailureException{}
		}
	}
	return ReadResponse(reader)
}

func HandleConnection(ctx context.Context, conn net.Conn, authenticator auth.Authenticator, bind netip.Addr, handler Handler, metadata M.Metadata) error {
	authRequest, err := ReadAuthRequest(conn)
	if err != nil {
		return E.Cause(err, "read socks auth request")
	}
	return handleConnection(authRequest, ctx, conn, authenticator, bind, handler, metadata)
}

func HandleConnection0(ctx context.Context, conn net.Conn, authenticator auth.Authenticator, bind netip.Addr, handler Handler, metadata M.Metadata) error {
	authRequest, err := ReadAuthRequest0(conn)
	if err != nil {
		return E.Cause(err, "read socks auth request")
	}
	return handleConnection(authRequest, ctx, conn, authenticator, bind, handler, metadata)
}

func handleConnection(authRequest *AuthRequest, ctx context.Context, conn net.Conn, authenticator auth.Authenticator, bind netip.Addr, handler Handler, metadata M.Metadata) error {
	request, err := serverHandshake(authRequest, conn, authenticator)
	if err != nil {
		return E.Cause(err, "read socks request")
	}
	switch request.Command {
	case CommandConnect:
		err = WriteResponse(conn, &Response{
			Version:   request.Version,
			ReplyCode: ReplyCodeSuccess,
			Bind:      M.SocksaddrFromNet(conn.LocalAddr()),
		})
		if err != nil {
			return E.Cause(err, "write socks response")
		}
		metadata.Protocol = "socks5"
		metadata.Destination = request.Destination
		return handler.NewConnection(ctx, conn, metadata)
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
			Bind:      M.SocksaddrFromNet(udpConn.LocalAddr()),
		})
		if err != nil {
			return E.Cause(err, "write socks response")
		}
		metadata.Protocol = "socks5"
		metadata.Destination = request.Destination
		go func() {
			err := handler.NewPacketConnection(ctx, NewAssociatePacketConn(conn, udpConn, request.Destination), metadata)
			if err != nil {
				handler.HandleError(err)
			}
			conn.Close()
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

func ServerHandshake(conn net.Conn, authenticator auth.Authenticator) (*Request, error) {
	authRequest, err := ReadAuthRequest(conn)
	if err != nil {
		return nil, E.Cause(err, "read socks auth request")
	}
	return serverHandshake(authRequest, conn, authenticator)
}

func ServerHandshake0(conn net.Conn, authenticator auth.Authenticator) (*Request, error) {
	authRequest, err := ReadAuthRequest0(conn)
	if err != nil {
		return nil, E.Cause(err, "read socks auth request")
	}
	return serverHandshake(authRequest, conn, authenticator)
}

func serverHandshake(authRequest *AuthRequest, conn net.Conn, authenticator auth.Authenticator) (*Request, error) {
	var authMethod byte
	if authenticator == nil {
		authMethod = AuthTypeNotRequired
	} else {
		authMethod = AuthTypeUsernamePassword
	}
	if !common.Contains(authRequest.Methods, authMethod) {
		err := WriteAuthResponse(conn, &AuthResponse{
			Version: authRequest.Version,
			Method:  AuthTypeNoAcceptedMethods,
		})
		if err != nil {
			return nil, E.Cause(err, "write socks auth response")
		}
	}
	err := WriteAuthResponse(conn, &AuthResponse{
		Version: authRequest.Version,
		Method:  authMethod,
	})
	if err != nil {
		return nil, E.Cause(err, "write socks auth response")
	}

	if authMethod == AuthTypeUsernamePassword {
		usernamePasswordAuthRequest, err := ReadUsernamePasswordAuthRequest(conn)
		if err != nil {
			return nil, E.Cause(err, "read user auth request")
		}
		response := &UsernamePasswordAuthResponse{}
		if authenticator.Verify(usernamePasswordAuthRequest.Username, usernamePasswordAuthRequest.Password) {
			response.Status = UsernamePasswordStatusSuccess
		} else {
			response.Status = UsernamePasswordStatusFailure
		}
		err = WriteUsernamePasswordAuthResponse(conn, response)
		if err != nil {
			return nil, E.Cause(err, "write user auth response")
		}
	}

	request, err := ReadRequest(conn)
	if err != nil {
		return nil, E.Cause(err, "read socks request")
	}
	return request, nil
}
