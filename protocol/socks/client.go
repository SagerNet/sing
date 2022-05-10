package socks

import (
	"context"
	"net"
	"net/url"
	"os"
	"strings"

	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/protocol/socks/socks4"
	"github.com/sagernet/sing/protocol/socks/socks5"
)

type Version uint8

const (
	Version4 Version = iota
	Version4A
	Version5
)

type Client struct {
	version    Version
	dialer     N.ContextDialer
	serverAddr M.Socksaddr
	username   string
	password   string
}

func NewClient(dialer N.ContextDialer, serverAddr M.Socksaddr, version Version, username string, password string) *Client {
	return &Client{
		version:    version,
		dialer:     dialer,
		serverAddr: serverAddr,
		username:   username,
		password:   password,
	}
}

func NewClientFromURL(dialer N.ContextDialer, rawURL string) (*Client, error) {
	var client Client
	if !strings.Contains(rawURL, "://") {
		rawURL = "socks://" + rawURL
	}
	proxyURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	client.dialer = dialer
	client.serverAddr = M.ParseSocksaddr(proxyURL.Host)
	switch proxyURL.Scheme {
	case "socks4":
		client.version = Version4
	case "socks4a":
		client.version = Version4A
	case "socks", "socks5", "":
		client.version = Version5
	default:
		return nil, E.New("socks: unknown scheme: ", proxyURL.Scheme)
	}
	if proxyURL.User != nil {
		if client.version == Version5 {
			client.username = proxyURL.User.Username()
			client.password, _ = proxyURL.User.Password()
		} else {
			client.username = proxyURL.User.String()
		}
	}
	return &client, nil
}

func (c *Client) DialContext(ctx context.Context, network string, address M.Socksaddr) (net.Conn, error) {
	var command byte
	if strings.HasPrefix(network, "tcp") {
		command = socks4.CommandConnect
	} else {
		if c.version != Version5 {
			return nil, E.New("socks4: udp unsupported")
		}
		command = socks5.CommandUDPAssociate
	}
	tcpConn, err := c.dialer.DialContext(ctx, "tcp", c.serverAddr)
	if err != nil {
		return nil, err
	}
	if c.version == Version4 && address.Family().IsFqdn() {
		tcpAddr, err := net.ResolveTCPAddr(network, address.String())
		if err != nil {
			tcpConn.Close()
			return nil, err
		}
		address = M.SocksaddrFromNet(tcpAddr)
	}
	switch c.version {
	case Version4, Version4A:
		_, err = ClientHandshake4(tcpConn, command, address, c.username)
		if err != nil {
			tcpConn.Close()
			return nil, err
		}
		return tcpConn, nil
	case Version5:
		response, err := ClientHandshake5(tcpConn, command, address, c.username, c.password)
		if err != nil {
			tcpConn.Close()
			return nil, err
		}
		if command == socks5.CommandConnect {
			return tcpConn, nil
		}
		udpConn, err := c.dialer.DialContext(ctx, "udp", response.Bind)
		if err != nil {
			tcpConn.Close()
			return nil, err
		}
		return NewAssociateConn(tcpConn, udpConn, address), nil
	}
	return nil, os.ErrInvalid
}

func (c *Client) BindContext(ctx context.Context, address M.Socksaddr) (net.Conn, error) {
	tcpConn, err := c.dialer.DialContext(ctx, "tcp", c.serverAddr)
	if err != nil {
		return nil, err
	}
	switch c.version {
	case Version4, Version4A:
		_, err = ClientHandshake4(tcpConn, socks4.CommandBind, address, c.username)
		if err != nil {
			tcpConn.Close()
			return nil, err
		}
		return tcpConn, nil
	case Version5:
		_, err = ClientHandshake5(tcpConn, socks5.CommandBind, address, c.username, c.password)
		if err != nil {
			tcpConn.Close()
			return nil, err
		}
		return tcpConn, nil
	}
	return nil, os.ErrInvalid
}
