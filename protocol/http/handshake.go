package http

import (
	std_bufio "bufio"
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	E "github.com/sagernet/sing/common/exceptions"
	F "github.com/sagernet/sing/common/format"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type Handler interface {
	N.TCPConnectionHandler
	N.UDPConnectionHandler
}

func HandleConnection(ctx context.Context, conn net.Conn, authenticator auth.Authenticator, handler Handler, metadata M.Metadata) error {
	reader := std_bufio.NewReader(conn)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return E.Cause(err, "read http request")
	}
	if reader.Buffered() > 0 {
		_buffer := buf.StackNewSize(reader.Buffered())
		defer common.KeepAlive(_buffer)
		buffer := common.Dup(_buffer)
		defer buffer.Release()
		_, err = buffer.ReadFullFrom(reader, reader.Buffered())
		if err != nil {
			return err
		}
		conn = bufio.NewCachedConn(conn, buffer)
	}
	return HandleRequest(ctx, request, conn, authenticator, handler, metadata)
}

func HandleRequest(ctx context.Context, request *http.Request, conn net.Conn, authenticator auth.Authenticator, handler Handler, metadata M.Metadata) error {
	var httpClient *http.Client
	for {
		if authenticator != nil {
			var authOk bool
			authorization := request.Header.Get("Proxy-Authorization")
			if strings.HasPrefix(authorization, "BASIC ") {
				userPassword, _ := base64.URLEncoding.DecodeString(authorization[6:])
				userPswdArr := strings.SplitN(string(userPassword), ":", 2)
				authOk = authenticator.Verify(userPswdArr[0], userPswdArr[1])
			}
			if !authOk {
				err := responseWith(request, http.StatusProxyAuthRequired).Write(conn)
				if err != nil {
					return err
				}
			}
		}

		if request.Method == "CONNECT" {
			portStr := request.URL.Port()
			if portStr == "" {
				portStr = "80"
			}
			destination := M.ParseSocksaddrHostPortStr(request.URL.Hostname(), portStr)
			_, err := conn.Write([]byte(F.ToString("HTTP/", request.ProtoMajor, ".", request.ProtoMinor, " 200 Connection established\r\n\r\n")))
			if err != nil {
				return E.Cause(err, "write http response")
			}
			metadata.Protocol = "http"
			metadata.Destination = destination
			return handler.NewConnection(ctx, conn, metadata)
		}

		keepAlive := strings.TrimSpace(strings.ToLower(request.Header.Get("Proxy-Connection"))) == "keep-alive"

		host := request.Header.Get("Host")
		if host != "" {
			request.Host = host
		}

		request.RequestURI = ""

		removeHopByHopHeaders(request.Header)
		removeExtraHTTPHostPort(request)

		if request.URL.Scheme == "" || request.URL.Host == "" {
			return responseWith(request, http.StatusBadRequest).Write(conn)
		}

		var innerErr error
		if httpClient == nil {
			httpClient = &http.Client{
				Transport: &http.Transport{
					MaxIdleConns:          100,
					IdleConnTimeout:       90 * time.Second,
					TLSHandshakeTimeout:   10 * time.Second,
					ExpectContinueTimeout: 1 * time.Second,
					DialContext: func(context context.Context, network, address string) (net.Conn, error) {
						if network != "tcp" && network != "tcp4" && network != "tcp6" {
							return nil, E.New("unsupported network ", network)
						}
						metadata.Destination = M.ParseSocksaddr(address)
						metadata.Protocol = "http"
						left, right := net.Pipe()
						go func() {
							err := handler.NewConnection(ctx, right, metadata)
							if err != nil {
								innerErr = err
								common.Close(left, right)
							}
						}()
						return left, nil
					},
				},
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}
		}

		response, err := httpClient.Do(request)
		if err != nil {
			return common.AnyError(innerErr, err, responseWith(request, http.StatusBadGateway).Write(conn))
		}

		removeHopByHopHeaders(response.Header)

		if keepAlive {
			response.Header.Set("Proxy-Connection", "keep-alive")
			response.Header.Set("Connection", "keep-alive")
			response.Header.Set("Keep-Alive", "timeout=4")
		}

		response.Close = !keepAlive

		err = response.Write(conn)
		if err != nil {
			return common.AnyError(innerErr, err)
		}

		if !keepAlive {
			return conn.Close()
		}
	}
}

func removeHopByHopHeaders(header http.Header) {
	// Strip hop-by-hop header based on RFC:
	// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html#sec13.5.1
	// https://www.mnot.net/blog/2011/07/11/what_proxies_must_do

	header.Del("Proxy-Connection")
	header.Del("Proxy-Authenticate")
	header.Del("Proxy-Authorization")
	header.Del("TE")
	header.Del("Trailers")
	header.Del("Transfer-Encoding")
	header.Del("Upgrade")

	connections := header.Get("Connection")
	header.Del("Connection")
	if len(connections) == 0 {
		return
	}
	for _, h := range strings.Split(connections, ",") {
		header.Del(strings.TrimSpace(h))
	}
}

func removeExtraHTTPHostPort(req *http.Request) {
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}

	if pHost, port, err := net.SplitHostPort(host); err == nil && port == "80" {
		host = pHost
	}

	req.Host = host
	req.URL.Host = host
}

func responseWith(request *http.Request, statusCode int) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Proto:      request.Proto,
		ProtoMajor: request.ProtoMajor,
		ProtoMinor: request.ProtoMinor,
		Header:     http.Header{},
	}
}
