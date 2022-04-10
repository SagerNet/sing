package http

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
	_ "unsafe"

	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/transport/tcp"
)

type Handler interface {
	tcp.Handler
}

func HandleConnection(conn *buf.BufferedConn, authenticator auth.Authenticator, handler Handler) error {
	var httpClient *http.Client
	for {
		request, err := readRequest(conn.Reader())
		if err != nil {
			return E.Cause(err, "read http request")
		}

		if authenticator != nil {
			var authOk bool
			authorization := request.Header.Get("Proxy-Authorization")
			if strings.HasPrefix(authorization, "BASIC ") {
				userPassword, _ := base64.URLEncoding.DecodeString(authorization[6:])
				userPswdArr := strings.SplitN(string(userPassword), ":", 2)
				authOk = authenticator.Verify(userPswdArr[0], userPswdArr[1])
			}
			if !authOk {
				err = responseWith(request, http.StatusProxyAuthRequired).Write(conn)
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
			destination, err := M.ParseAddrPort(request.URL.Hostname(), portStr)
			if err != nil {
				if err != nil {
					return err
				}
			}
			_, err = fmt.Fprintf(conn, "HTTP/%d.%d %03d %s\r\n\r\n", request.ProtoMajor, request.ProtoMinor, http.StatusOK, "Connection established")
			if err != nil {
				return E.Cause(err, "write http response")
			}
			return handler.NewConnection(conn, M.Metadata{
				Destination: destination,
			})
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

						destination, err := M.ParseAddress(address)
						if err != nil {
							return nil, err
						}

						left, right := net.Pipe()
						go func() {
							err = handler.NewConnection(right, M.Metadata{
								Destination: destination,
							})
							if err != nil {
								handler.HandleError(err)
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
			handler.HandleError(err)
			return responseWith(request, http.StatusBadGateway).Write(conn)
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
			return err
		}
	}
}

//go:linkname readRequest net/http.ReadRequest
func readRequest(b *bufio.Reader) (req *http.Request, err error)

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
