package socks

import (
	std_bufio "bufio"
	"context"
	"io"
	"net"
	"net/netip"
	"os"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/bufio"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/varbin"
	"github.com/sagernet/sing/protocol/socks/socks4"
	"github.com/sagernet/sing/protocol/socks/socks5"
)

// Deprecated: Use HandlerEx instead.
//
//nolint:staticcheck
type Handler interface {
	N.TCPConnectionHandler
	N.UDPConnectionHandler
}

type HandlerEx interface {
	N.TCPConnectionHandlerEx
	N.UDPConnectionHandlerEx
}

func ClientHandshake4(conn io.ReadWriter, command byte, destination M.Socksaddr, username string) (socks4.Response, error) {
	err := socks4.WriteRequest(conn, socks4.Request{
		Command:     command,
		Destination: destination,
		Username:    username,
	})
	if err != nil {
		return socks4.Response{}, err
	}
	response, err := socks4.ReadResponse(varbin.StubReader(conn))
	if err != nil {
		return socks4.Response{}, err
	}
	if response.ReplyCode != socks4.ReplyCodeGranted {
		err = E.New("socks4: request rejected, code= ", response.ReplyCode)
	}
	return response, err
}

func ClientHandshake5(conn io.ReadWriter, command byte, destination M.Socksaddr, username string, password string) (socks5.Response, error) {
	reader := varbin.StubReader(conn)
	var method byte
	if username == "" {
		method = socks5.AuthTypeNotRequired
	} else {
		method = socks5.AuthTypeUsernamePassword
	}
	err := socks5.WriteAuthRequest(conn, socks5.AuthRequest{
		Methods: []byte{method},
	})
	if err != nil {
		return socks5.Response{}, err
	}
	authResponse, err := socks5.ReadAuthResponse(reader)
	if err != nil {
		return socks5.Response{}, err
	}
	if authResponse.Method == socks5.AuthTypeUsernamePassword {
		err = socks5.WriteUsernamePasswordAuthRequest(conn, socks5.UsernamePasswordAuthRequest{
			Username: username,
			Password: password,
		})
		if err != nil {
			return socks5.Response{}, err
		}
		usernamePasswordResponse, err := socks5.ReadUsernamePasswordAuthResponse(reader)
		if err != nil {
			return socks5.Response{}, err
		}
		if usernamePasswordResponse.Status != socks5.UsernamePasswordStatusSuccess {
			return socks5.Response{}, E.New("socks5: incorrect user name or password")
		}
	} else if authResponse.Method != socks5.AuthTypeNotRequired {
		return socks5.Response{}, E.New("socks5: unsupported auth method: ", authResponse.Method)
	}
	err = socks5.WriteRequest(conn, socks5.Request{
		Command:     command,
		Destination: destination,
	})
	if err != nil {
		return socks5.Response{}, err
	}
	response, err := socks5.ReadResponse(reader)
	if err != nil {
		return socks5.Response{}, err
	}
	if response.ReplyCode != socks5.ReplyCodeSuccess {
		err = E.New("socks5: request rejected, code=", response.ReplyCode)
	}
	return response, err
}

// Deprecated: use HandleConnectionEx instead.
func HandleConnection(ctx context.Context, conn net.Conn, authenticator *auth.Authenticator, handler Handler, metadata M.Metadata) error {
	return HandleConnection0(ctx, conn, std_bufio.NewReader(conn), authenticator, handler, metadata)
}

// Deprecated: Use HandleConnectionEx instead.
func HandleConnection0(ctx context.Context, conn net.Conn, reader *std_bufio.Reader, authenticator *auth.Authenticator, handler Handler, metadata M.Metadata) error {
	return HandleConnectionEx(ctx, conn, reader, authenticator, handler, nil, metadata.Source, metadata.Destination, nil)
}

func HandleConnectionEx(
	ctx context.Context, conn net.Conn, reader *std_bufio.Reader,
	authenticator *auth.Authenticator,
	//nolint:staticcheck
	handler Handler,
	handlerEx HandlerEx,
	source M.Socksaddr, destination M.Socksaddr,
	onClose N.CloseHandlerFunc,
) error {
	version, err := reader.ReadByte()
	if err != nil {
		return err
	}
	switch version {
	case socks4.Version:
		var request socks4.Request
		request, err = socks4.ReadRequest0(reader)
		if err != nil {
			return err
		}
		switch request.Command {
		case socks4.CommandConnect:
			if authenticator != nil && !authenticator.Verify(request.Username, "") {
				err = socks4.WriteResponse(conn, socks4.Response{
					ReplyCode: socks4.ReplyCodeRejectedOrFailed,
				})
				if err != nil {
					return err
				}
				return E.New("socks4: authentication failed, username=", request.Username)
			}
			destination = request.Destination
			if handlerEx != nil {
				handlerEx.NewConnectionEx(auth.ContextWithUser(ctx, request.Username), NewLazyConn(conn, version), source, destination, onClose)
			} else {
				err = socks4.WriteResponse(conn, socks4.Response{
					ReplyCode:   socks4.ReplyCodeGranted,
					Destination: M.SocksaddrFromNet(conn.LocalAddr()),
				})
				if err != nil {
					return err
				}
				//nolint:staticcheck
				return handler.NewConnection(auth.ContextWithUser(ctx, request.Username), conn, M.Metadata{Protocol: "socks4", Source: source, Destination: destination})
			}
			return nil
		default:
			err = socks4.WriteResponse(conn, socks4.Response{
				ReplyCode: socks4.ReplyCodeRejectedOrFailed,
			})
			if err != nil {
				return err
			}
			return E.New("socks4: unsupported command ", request.Command)
		}
	case socks5.Version:
		var authRequest socks5.AuthRequest
		authRequest, err = socks5.ReadAuthRequest0(reader)
		if err != nil {
			return err
		}
		var authMethod byte
		if authenticator != nil && !common.Contains(authRequest.Methods, socks5.AuthTypeUsernamePassword) {
			err = socks5.WriteAuthResponse(conn, socks5.AuthResponse{
				Method: socks5.AuthTypeNoAcceptedMethods,
			})
			if err != nil {
				return err
			}
		}
		if authenticator != nil {
			authMethod = socks5.AuthTypeUsernamePassword
		} else {
			authMethod = socks5.AuthTypeNotRequired
		}
		err = socks5.WriteAuthResponse(conn, socks5.AuthResponse{
			Method: authMethod,
		})
		if err != nil {
			return err
		}
		if authMethod == socks5.AuthTypeUsernamePassword {
			var usernamePasswordAuthRequest socks5.UsernamePasswordAuthRequest
			usernamePasswordAuthRequest, err = socks5.ReadUsernamePasswordAuthRequest(reader)
			if err != nil {
				return err
			}
			ctx = auth.ContextWithUser(ctx, usernamePasswordAuthRequest.Username)
			response := socks5.UsernamePasswordAuthResponse{}
			if authenticator.Verify(usernamePasswordAuthRequest.Username, usernamePasswordAuthRequest.Password) {
				response.Status = socks5.UsernamePasswordStatusSuccess
			} else {
				response.Status = socks5.UsernamePasswordStatusFailure
			}
			err = socks5.WriteUsernamePasswordAuthResponse(conn, response)
			if err != nil {
				return err
			}
			if response.Status != socks5.UsernamePasswordStatusSuccess {
				return E.New("socks5: authentication failed, username=", usernamePasswordAuthRequest.Username, ", password=", usernamePasswordAuthRequest.Password)
			}
		}
		var request socks5.Request
		request, err = socks5.ReadRequest(reader)
		if err != nil {
			return err
		}
		switch request.Command {
		case socks5.CommandConnect:
			destination = request.Destination
			if handlerEx != nil {
				handlerEx.NewConnectionEx(ctx, NewLazyConn(conn, version), source, destination, onClose)
				return nil
			} else {
				err = socks5.WriteResponse(conn, socks5.Response{
					ReplyCode: socks5.ReplyCodeSuccess,
					Bind:      M.SocksaddrFromNet(conn.LocalAddr()),
				})
				if err != nil {
					return err
				}
				//nolint:staticcheck
				return handler.NewConnection(ctx, conn, M.Metadata{Protocol: "socks5", Source: source, Destination: destination})
			}
		case socks5.CommandUDPAssociate:
			var udpConn *net.UDPConn
			udpConn, err = net.ListenUDP(M.NetworkFromNetAddr("udp", M.AddrFromNet(conn.LocalAddr())), net.UDPAddrFromAddrPort(netip.AddrPortFrom(M.AddrFromNet(conn.LocalAddr()), 0)))
			if err != nil {
				return err
			}
			if handlerEx == nil {
				defer udpConn.Close()
				err = socks5.WriteResponse(conn, socks5.Response{
					ReplyCode: socks5.ReplyCodeSuccess,
					Bind:      M.SocksaddrFromNet(udpConn.LocalAddr()),
				})
				if err != nil {
					return err
				}
				destination = request.Destination
				associatePacketConn := NewAssociatePacketConn(bufio.NewServerPacketConn(udpConn), destination, conn)
				var innerError error
				done := make(chan struct{})
				go func() {
					//nolint:staticcheck
					innerError = handler.NewPacketConnection(ctx, associatePacketConn, M.Metadata{Protocol: "socks5", Source: source, Destination: destination})
					close(done)
				}()
				err = common.Error(io.Copy(io.Discard, conn))
				associatePacketConn.Close()
				<-done
				return E.Errors(innerError, err)
			} else {
				handlerEx.NewPacketConnectionEx(ctx, NewLazyAssociatePacketConn(bufio.NewServerPacketConn(udpConn), destination, conn), source, destination, onClose)
				return nil
			}
		default:
			err = socks5.WriteResponse(conn, socks5.Response{
				ReplyCode: socks5.ReplyCodeUnsupported,
			})
			if err != nil {
				return err
			}
			return E.New("socks5: unsupported command ", request.Command)
		}
	}
	return os.ErrInvalid
}
