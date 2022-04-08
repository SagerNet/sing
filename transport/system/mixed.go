package system

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/redir"
	"github.com/sagernet/sing/common/socksaddr"
	"github.com/sagernet/sing/protocol/socks"
)

type MixedListener struct {
	*SocksListener
	UDPListener *UDPListener
	mcfg        *MixedConfig
	laddr       *net.TCPAddr
}

type MixedConfig struct {
	SocksConfig
	Redirect bool
	TProxy   bool
}

func NewMixedListener(bind netip.AddrPort, config *MixedConfig, handler SocksHandler) *MixedListener {
	listener := &MixedListener{SocksListener: NewSocksListener(bind, &config.SocksConfig, handler), mcfg: config}
	listener.TCPListener.Handler = listener
	if config.TProxy {
		listener.UDPListener = NewUDPListener(bind, listener)
	}
	return listener
}

func (l *MixedListener) HandleTCP(conn net.Conn) error {
	if l.mcfg.Redirect {
		var destination netip.AddrPort
		destination, err := redir.GetOriginalDestination(conn)
		if err == nil {
			return l.Handler.NewConnection(socksaddr.AddrFromAddr(destination.Addr()), destination.Port(), conn)
		}
	} else if l.mcfg.TProxy {
		lAddr := conn.LocalAddr().(*net.TCPAddr)
		rAddr := conn.RemoteAddr().(*net.TCPAddr)

		if lAddr.Port != l.laddr.Port || !lAddr.IP.Equal(rAddr.IP) && !lAddr.IP.IsLoopback() && !lAddr.IP.IsPrivate() {
			addr, port := socksaddr.AddrFromNetAddr(lAddr)
			return l.Handler.NewConnection(addr, port, conn)
		}
	}

	bufConn := buf.NewBufferedConn(conn)
	hdr, err := bufConn.ReadByte()
	if err != nil {
		return err
	}
	err = bufConn.UnreadByte()
	if err != nil {
		return err
	}

	if hdr == socks.Version4 || hdr == socks.Version5 {
		return l.SocksListener.HandleTCP(bufConn)
	}

	var httpClient *http.Client
	for {
		request, err := readRequest(bufConn.Reader())
		if err != nil {
			return exceptions.Cause(err, "read http request")
		}

		if l.Username != "" {
			var authOk bool
			authorization := request.Header.Get("Proxy-Authorization")
			if strings.HasPrefix(authorization, "BASIC ") {
				userPassword, _ := base64.URLEncoding.DecodeString(authorization[6:])
				if string(userPassword) == l.Username+":"+l.Password {
					authOk = true
				}
			}
			if !authOk {
				err = responseWith(request, http.StatusProxyAuthRequired).Write(conn)
				if err != nil {
					return err
				}
			}
		}

		if request.Method == "CONNECT" {
			host := request.URL.Hostname()
			portStr := request.URL.Port()
			if portStr == "" {
				portStr = "80"
			}
			port, err := strconv.Atoi(portStr)
			if err != nil {
				err = responseWith(request, http.StatusBadRequest).Write(conn)
				if err != nil {
					return err
				}
			}
			_, err = fmt.Fprintf(conn, "HTTP/%d.%d %03d %s\r\n\r\n", request.ProtoMajor, request.ProtoMinor, http.StatusOK, "Connection established")
			if err != nil {
				return exceptions.Cause(err, "write http response")
			}
			return l.Handler.NewConnection(socksaddr.ParseAddr(host), uint16(port), bufConn)
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
							return nil, exceptions.New("unsupported network ", network)
						}

						addr, port, err := socksaddr.ParseAddrPort(address)
						if err != nil {
							return nil, err
						}

						left, right := net.Pipe()
						go func() {
							err = l.Handler.NewConnection(addr, port, right)
							if err != nil {
								l.OnError(err)
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
			l.OnError(exceptions.Cause(err, "http proxy"))
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
			l.OnError(exceptions.Cause(err, "http proxy"))
			return err
		}
	}
}

func (l *MixedListener) HandleUDP(buffer *buf.Buffer, sourceAddr net.Addr) error {
	return nil
}

// removeHopByHopHeaders remove hop-by-hop header
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

// removeExtraHTTPHostPort remove extra host port (example.com:80 --> example.com)
// It resolves the behavior of some HTTP servers that do not handle host:80 (e.g. baidu.com)
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

func (l *MixedListener) Start() error {
	err := l.TCPListener.Start()
	if err != nil {
		return err
	}
	if l.mcfg.TProxy {
		rawConn, err := l.TCPListener.TCPListener.SyscallConn()
		if err != nil {
			return err
		}
		var rawFd uintptr
		err = rawConn.Control(func(fd uintptr) {
			rawFd = fd
		})
		if err != nil {
			return err
		}
		err = syscall.SetsockoptInt(int(rawFd), syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
		if err != nil {
			return exceptions.Cause(err, "failed to configure TCP tproxy")
		}
	}
	if l.mcfg.TProxy {
		l.laddr = l.TCPListener.Addr().(*net.TCPAddr)
	}
	return nil
}

func (l *MixedListener) Close() error {
	return l.TCPListener.Close()
}

func (l *MixedListener) OnError(err error) {
	l.Handler.OnError(exceptions.Cause(err, "mixed server"))
}
