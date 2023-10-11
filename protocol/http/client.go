package http

import (
	std_bufio "bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

var _ N.Dialer = (*Client)(nil)

type Client struct {
	dialer     N.Dialer
	serverAddr M.Socksaddr
	username   string
	password   string
	path       string
	headers    http.Header
}

type Options struct {
	Dialer   N.Dialer
	Server   M.Socksaddr
	Username string
	Password string
	Path     string
	Headers  http.Header
}

func NewClient(options Options) *Client {
	client := &Client{
		dialer:     options.Dialer,
		serverAddr: options.Server,
		username:   options.Username,
		password:   options.Password,
		path:       options.Path,
		headers:    options.Headers,
	}
	if options.Dialer == nil {
		client.dialer = N.SystemDialer
	}
	return client
}

func (c *Client) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	network = N.NetworkName(network)
	switch network {
	case N.NetworkTCP:
	case N.NetworkUDP:
		return nil, os.ErrInvalid
	default:
		return nil, E.Extend(N.ErrUnknownNetwork, network)
	}
	var conn net.Conn
	conn, err := c.dialer.DialContext(ctx, N.NetworkTCP, c.serverAddr)
	if err != nil {
		return nil, err
	}
	URL := destination.String()
	HeaderString := "CONNECT " + URL + " HTTP/1.1\r\n"
	tempHeaders := map[string][]string{
		"Host":             {destination.String()},
		"User-Agent":       {"Go-http-client/1.1"},
		"Proxy-Connection": {"Keep-Alive"},
	}

	for key, valueList := range c.headers {
		tempHeaders[key] = valueList
	}

	if c.path != "" {
		tempHeaders["Path"] = []string{c.path}
	}

	if c.username != "" {
		auth := c.username + ":" + c.password
		if _, ok := tempHeaders["Proxy-Authorization"]; ok {
			tempHeaders["Proxy-Authorization"][len(tempHeaders["Proxy-Authorization"])] = "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
		} else {
			tempHeaders["Proxy-Authorization"] = []string{"Basic " + base64.StdEncoding.EncodeToString([]byte(auth))}
		}
	}
	for key, valueList := range tempHeaders {
		HeaderString += key + ": " + strings.Join(valueList, "; ") + "\r\n"
	}

	HeaderString += "\r\n"

	_, err = fmt.Fprintf(conn, "%s", HeaderString)

	if err != nil {
		conn.Close()
		return nil, err
	}

	reader := std_bufio.NewReader(conn)

	response, err := http.ReadResponse(reader, nil)

	if err != nil {
		conn.Close()
		return nil, err
	}

	if response.StatusCode == http.StatusOK {
		if reader.Buffered() > 0 {
			buffer := buf.NewSize(reader.Buffered())
			_, err = buffer.ReadFullFrom(reader, buffer.FreeLen())
			if err != nil {
				conn.Close()
				return nil, err
			}
			conn = bufio.NewCachedConn(conn, buffer)
		}
		return conn, nil
	} else {
		conn.Close()
		switch response.StatusCode {
		case http.StatusProxyAuthRequired:
			return nil, E.New("authentication required")
		case http.StatusMethodNotAllowed:
			return nil, E.New("method not allowed")
		default:
			return nil, E.New("unexpected status: ", response.Status)
		}
	}
}

func (c *Client) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	return nil, os.ErrInvalid
}
