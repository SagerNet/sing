package socks

import (
	"bytes"
	"io"
	"net"

	"github.com/sirupsen/logrus"
	"net/netip"
	"sing/common"
	"sing/common/buf"
	"sing/common/exceptions"
	"sing/common/session"
	"sing/common/socksaddr"
	"sing/protocol/socks"
	"sing/transport"
	"sing/transport/system"
)

var _ transport.Inbound = (*Inbound)(nil)

type Inbound struct {
	lAddr              netip.AddrPort
	username, password string
	tcpListener        *system.TCPListener
	udpListener        *system.UDPListener
	handler            session.Handler
}

func (h *Inbound) Init(ctx *transport.InboundContext) {
}

type InboundConfig struct {
	Listen   string `json:"listen"`
	Port     uint16 `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

func NewListener(handler session.Handler, config *InboundConfig) (*Inbound, error) {
	addr, err := netip.ParseAddr(config.Listen)
	if err != nil {
		return nil, exceptions.Cause(err, "invalid listen address: ", config.Listen)
	}
	lAddr := netip.AddrPortFrom(addr, config.Port)
	inbound := new(Inbound)
	inbound.username, inbound.password = config.Username, config.Password
	inbound.handler = handler
	inbound.tcpListener = system.NewTCPListener(lAddr, inbound)
	inbound.udpListener = system.NewUDPListener(lAddr, inbound)
	return inbound, nil
}

func (h *Inbound) Start() error {
	err := h.tcpListener.Start()
	if err != nil {
		return err
	}
	return h.udpListener.Start()
}

func (h *Inbound) HandleTCP(conn net.Conn) error {
	authRequest, err := socks.ReadAuthRequest(conn)
	if err != nil {
		return exceptions.Cause(err, "read socks auth request")
	}
	if h.username != "" {
		if bytes.IndexByte(authRequest.Methods, socks.AuthTypeNotRequired) > 0 {
			err = socks.WriteAuthResponse(conn, &socks.AuthResponse{
				Version: authRequest.Version,
				Method:  socks.AuthTypeNotRequired,
			})
			if err != nil {
				return exceptions.Cause(err, "write socks auth response")
			}
		} else {
			socks.WriteAuthResponse(conn, &socks.AuthResponse{
				Version: authRequest.Version,
				Method:  socks.AuthTypeNoAcceptedMethods,
			})
			return exceptions.New("no accepted methods, requested = ", authRequest.Methods, ", except no auth")
		}
	} else if bytes.IndexByte(authRequest.Methods, socks.AuthTypeNotRequired) == -1 {
		socks.WriteAuthResponse(conn, &socks.AuthResponse{
			Version: authRequest.Version,
			Method:  socks.AuthTypeNoAcceptedMethods,
		})
		return exceptions.New("no accepted methods, requested = ", authRequest.Methods, ", except password")
	} else {
		err = socks.WriteAuthResponse(conn, &socks.AuthResponse{
			Version: authRequest.Version,
			Method:  socks.AuthTypeUsernamePassword,
		})
		if err != nil {
			return exceptions.Cause(err, "write socks auth response: ", err)
		}
		usernamePasswordRequest, err := socks.ReadUsernamePasswordAuthRequest(conn)
		if err != nil {
			return exceptions.Cause(err, "read username-password request")
		}
		if usernamePasswordRequest.Username != h.username {
			socks.WriteUsernamePasswordAuthResponse(conn, &socks.UsernamePasswordAuthResponse{Status: socks.UsernamePasswordStatusFailure})
			return exceptions.New("auth failed: excepted username ", h.username, ", got ", usernamePasswordRequest.Username)
		} else if usernamePasswordRequest.Password != h.password {
			socks.WriteUsernamePasswordAuthResponse(conn, &socks.UsernamePasswordAuthResponse{Status: socks.UsernamePasswordStatusFailure})
			return exceptions.New("auth failed: excepted password ", h.password, ", got ", usernamePasswordRequest.Password)
		}
		err = socks.WriteUsernamePasswordAuthResponse(conn, &socks.UsernamePasswordAuthResponse{Status: socks.UsernamePasswordStatusSuccess})
		if err != nil {
			return exceptions.Cause(err, "write username-password response")
		}
	}
	request, err := socks.ReadRequest(conn)
	if err != nil {
		return exceptions.Cause(err, "read request")
	}
	switch request.Command {
	case socks.CommandBind:
		socks.WriteResponse(conn, &socks.Response{
			Version:   request.Version,
			ReplyCode: socks.ReplyCodeUnsupported,
		})
		return exceptions.New("bind unsupported")
	case socks.CommandUDPAssociate:
		addr, port := session.AddressFromNetAddr(h.udpListener.LocalAddr())
		err = socks.WriteResponse(conn, &socks.Response{
			Version:   request.Version,
			ReplyCode: socks.ReplyCodeSuccess,
			BindAddr:  addr,
			BindPort:  port,
		})
		if err != nil {
			return exceptions.Cause(err, "write response")
		}
		io.Copy(io.Discard, conn)
		return nil
	}
	context := new(session.Context)
	context.Network = session.NetworkTCP
	context.Source, context.SourcePort = socksaddr.AddressFromNetAddr(conn.RemoteAddr())
	context.Destination, context.DestinationPort = request.Addr, request.Port
	h.handler.HandleConnection(&session.Conn{
		Conn:    conn,
		Context: context,
	})
	return nil
}

func (h *Inbound) HandleUDP(buffer *buf.Buffer, sourceAddr net.Addr) error {
	associatePacket, err := socks.DecodeAssociatePacket(buffer)
	if err != nil {
		return exceptions.Cause(err, "decode associate packet")
	}
	context := new(session.Context)
	context.Network = session.NetworkUDP
	context.Source, context.SourcePort = socksaddr.AddressFromNetAddr(sourceAddr)
	context.Destination, context.DestinationPort = associatePacket.Addr, associatePacket.Port
	h.handler.HandlePacket(&session.Packet{
		Context: context,
		Data:    buffer,
		Release: nil,
		WriteBack: func(buffer *buf.Buffer, addr *net.UDPAddr) error {
			header := new(socks.AssociatePacket)
			header.Addr, header.Port = socksaddr.AddressFromNetAddr(addr)
			header.Data = buffer.Bytes()
			packet := buf.FullNew()
			defer packet.Release()
			err := socks.EncodeAssociatePacket(header, packet)
			buffer.Release()
			if err != nil {
				return err
			}
			return common.Error(h.udpListener.WriteTo(packet.Bytes(), sourceAddr))
		},
	})
	return nil
}

func (h *Inbound) OnError(err error) {
	logrus.Warn("socks: ", err)
}

func (h *Inbound) Close() error {
	h.tcpListener.Close()
	h.udpListener.Close()
	return nil
}
