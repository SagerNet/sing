package http

import (
	std_bufio "bufio"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	E "github.com/sagernet/sing/common/exceptions"
	F "github.com/sagernet/sing/common/format"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/pipe"
)

const defaultProxyAuthRetryTimeout = 5 * time.Second

type HTTPServerOptions struct {
	ProxyAuthRetryTimeout time.Duration
	Logger                logger.ContextLogger
}

func normalizeHTTPServerOptions(options HTTPServerOptions) HTTPServerOptions {
	if options.ProxyAuthRetryTimeout <= 0 {
		options.ProxyAuthRetryTimeout = defaultProxyAuthRetryTimeout
	}
	if options.Logger == nil {
		options.Logger = logger.NOP()
	}
	return options
}

func HandleConnectionEx(
	ctx context.Context,
	conn net.Conn,
	reader *std_bufio.Reader,
	authenticator *auth.Authenticator,
	handler N.TCPConnectionHandlerEx,
	source M.Socksaddr,
	onClose N.CloseHandlerFunc,
) error {
	return HandleConnectionExWithOptions(ctx, conn, reader, authenticator, handler, source, onClose, HTTPServerOptions{})
}

func HandleConnectionExWithOptions(
	ctx context.Context,
	conn net.Conn,
	reader *std_bufio.Reader,
	authenticator *auth.Authenticator,
	handler N.TCPConnectionHandlerEx,
	source M.Socksaddr,
	onClose N.CloseHandlerFunc,
	options HTTPServerOptions,
) error {
	options = normalizeHTTPServerOptions(options)
	missingProxyAuthorizationRetried := false
	waitingRetryProxyAuthentication := false
	for {
		request, err := ReadRequest(reader)
		if err != nil {
			return E.Cause(err, "read http request")
		}
		printRequestHeaders(ctx, options.Logger, request)
		if waitingRetryProxyAuthentication {
			waitingRetryProxyAuthentication = false
			err = conn.SetReadDeadline(time.Time{})
			if err != nil {
				return E.Cause(err, "clear retry-proxy-authentication timeout")
			}
		}
		retryMissingProxyAuthorization := shouldRetryMissingProxyAuthorization(request)
		if authenticator != nil {
			username, password, authOk := ParseBasicAuth(request.Header.Get("Proxy-Authorization"))
			authOk = authOk && authenticator.Verify(username, password)
			if authOk {
				ctx = auth.ContextWithUser(ctx, username)
			} else {
				// Since no one else is using the library, use a fixed realm until rewritten
				proxyAuthRequiredResponse := responseWith(
					request, http.StatusProxyAuthRequired,
					"Proxy-Authenticate", `Basic realm="sing-box", charset="UTF-8"`,
				)
				printResponseHeaders(ctx, options.Logger, proxyAuthRequiredResponse)
				err = writeResponseBuffered(conn, proxyAuthRequiredResponse)
				if err != nil {
					return E.Cause(err, "write proxy authentication required response")
				}
				authorization := request.Header.Get("Proxy-Authorization")
				switch {
				case username != "":
					return E.New("http: authentication failed, username=", username, ", password=", password)
				case authorization != "":
					return E.New("http: authentication failed, Proxy-Authorization=", authorization)
				} else {
					if retryMissingProxyAuthorization && !missingProxyAuthorizationRetried {
						missingProxyAuthorizationRetried = true
						err = conn.SetReadDeadline(time.Now().Add(options.ProxyAuthRetryTimeout))
						if err != nil {
							return E.Cause(err, "set retry-proxy-authentication timeout")
						}
						waitingRetryProxyAuthentication = true
						continue
					}
					return E.New("http: authentication failed, no Proxy-Authorization header")
				}
			}
		}

		if sourceAddress := SourceAddress(request); sourceAddress.IsValid() {
			source = sourceAddress
		}

		if request.Method == "CONNECT" {
			destination := M.ParseSocksaddrHostPortStr(request.URL.Hostname(), request.URL.Port()).Unwrap()
			if destination.Port == 0 {
				switch request.URL.Scheme {
				case "https", "wss":
					destination.Port = 443
				default:
					destination.Port = 80
				}
			}
			_, err = conn.Write([]byte(F.ToString("HTTP/", request.ProtoMajor, ".", request.ProtoMinor, " 200 Connection established\r\n\r\n")))
			if err != nil {
				return E.Cause(err, "write http response")
			}
			var requestConn net.Conn
			if reader.Buffered() > 0 {
				buffer := buf.NewSize(reader.Buffered())
				_, err = buffer.ReadFullFrom(reader, reader.Buffered())
				if err != nil {
					return err
				}
				requestConn = bufio.NewCachedConn(conn, buffer)
			} else {
				requestConn = conn
			}
			handler.NewConnectionEx(ctx, requestConn, source, destination, onClose)
			return nil
		} else if strings.ToLower(request.Header.Get("Connection")) == "upgrade" {
			destination := M.ParseSocksaddrHostPortStr(request.URL.Hostname(), request.URL.Port()).Unwrap()
			if destination.Port == 0 {
				switch request.URL.Scheme {
				case "https", "wss":
					destination.Port = 443
				default:
					destination.Port = 80
				}
			}
			serverConn, clientConn := pipe.Pipe()
			go func() {
				handler.NewConnectionEx(ctx, clientConn, source, destination, func(it error) {
					if it != nil {
						common.Close(serverConn, clientConn)
					}
				})
			}()
			err = request.Write(serverConn)
			if err != nil {
				return E.Cause(err, "http: write upgrade request")
			}
			if reader.Buffered() > 0 {
				_, err = io.CopyN(serverConn, reader, int64(reader.Buffered()))
				if err != nil {
					return err
				}
			}
			return bufio.CopyConn(ctx, conn, serverConn)
		} else {
			err = handleHTTPConnection(ctx, handler, conn, request, source, options.Logger)
			if err != nil {
				return err
			}
		}
	}
}

func handleHTTPConnection(
	ctx context.Context,
	handler N.TCPConnectionHandlerEx,
	conn net.Conn,
	request *http.Request,
	source M.Socksaddr,
	contextLogger logger.ContextLogger,
) error {
	keepAlive := isProxyKeepAlive(request)
	request.RequestURI = ""

	removeHopByHopHeaders(request.Header)
	removeExtraHTTPHostPort(request)

	if hostStr := request.Header.Get("Host"); hostStr != "" {
		if hostStr != request.URL.Host {
			request.Host = hostStr
		}
	}

	if request.URL.Scheme == "" || request.URL.Host == "" {
		badRequestResponse := responseWith(request, http.StatusBadRequest)
		printResponseHeaders(ctx, contextLogger, badRequestResponse)
		return badRequestResponse.Write(conn)
	}

	var innerErr common.TypedValue[error]
	httpClient := &http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				input, output := pipe.Pipe()
				go handler.NewConnectionEx(ctx, output, source, M.ParseSocksaddr(address).Unwrap(), func(it error) {
					innerErr.Store(it)
					common.Close(input, output)
				})
				return input, nil
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer httpClient.CloseIdleConnections()

	requestCtx, cancel := context.WithCancel(ctx)
	response, err := httpClient.Do(request.WithContext(requestCtx))
	if err != nil {
		cancel()
		badGatewayResponse := responseWith(request, http.StatusBadGateway)
		printResponseHeaders(ctx, contextLogger, badGatewayResponse)
		return E.Errors(innerErr.Load(), err, badGatewayResponse.Write(conn))
	}

	removeHopByHopHeaders(response.Header)

	if keepAlive {
		response.Header.Set("Proxy-Connection", "keep-alive")
		response.Header.Set("Connection", "keep-alive")
		response.Header.Set("Keep-Alive", "timeout=4")
	}

	response.Close = !keepAlive
	printResponseHeaders(ctx, contextLogger, response)

	err = response.Write(conn)
	if err != nil {
		cancel()
		return E.Errors(innerErr.Load(), err)
	}

	cancel()
	if !keepAlive {
		return conn.Close()
	}
	return nil
}

func isProxyKeepAlive(request *http.Request) bool {
	connection := request.Header.Get("Connection")
	proxyConnection := request.Header.Get("Proxy-Connection")

	if request.ProtoMajor > 1 || (request.ProtoMajor == 1 && request.ProtoMinor >= 1) {
		// HTTP/1.1+ connections are persistent unless explicitly closed.
		return !hasHeaderToken(connection, "close") && !hasHeaderToken(proxyConnection, "close")
	}

	if request.ProtoMajor == 1 && request.ProtoMinor == 0 {
		// HTTP/1.0 defaults to close unless keep-alive is requested.
		if hasHeaderToken(connection, "close") || hasHeaderToken(proxyConnection, "close") {
			return false
		}
		return hasHeaderToken(connection, "keep-alive") || hasHeaderToken(proxyConnection, "keep-alive")
	}

	return false
}

func hasHeaderToken(headerValue string, token string) bool {
	for _, h := range strings.Split(headerValue, ",") {
		if strings.EqualFold(strings.TrimSpace(h), token) {
			return true
		}
	}
	return false
}

func shouldRetryMissingProxyAuthorization(request *http.Request) bool {
	return isProxyKeepAlive(request) &&
		!request.Close &&
		!hasHeaderToken(request.Header.Get("Connection"), "upgrade") &&
		request.ContentLength == 0 &&
		len(request.TransferEncoding) == 0
}

func printRequestHeaders(ctx context.Context, contextLogger logger.ContextLogger, request *http.Request) {
	contextLogger.TraceContext(ctx, "request protocol: ", request.Proto)
	printHeaders(ctx, contextLogger, "request", request.Header)
}

func printResponseHeaders(ctx context.Context, contextLogger logger.ContextLogger, response *http.Response) {
	contextLogger.TraceContext(ctx, "response: protocol=", response.Proto, " status=", response.StatusCode)
	printHeaders(ctx, contextLogger, "response", response.Header)
}

func printHeaders(ctx context.Context, contextLogger logger.ContextLogger, kind string, header http.Header) {
	keys := make([]string, 0, len(header))
	for key := range header {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		redacted := shouldRedactHeaderValue(key)
		for _, value := range header[key] {
			if redacted {
				value = "[redacted]"
			}
			contextLogger.TraceContext(ctx, kind, " header: ", key, ": ", value)
		}
	}
}

func shouldRedactHeaderValue(headerKey string) bool {
	return strings.EqualFold(headerKey, "Authorization") ||
		strings.EqualFold(headerKey, "Proxy-Authorization") ||
		strings.EqualFold(headerKey, "Cookie") ||
		strings.EqualFold(headerKey, "Set-Cookie")
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
	for h := range strings.SplitSeq(connections, ",") {
		header.Del(strings.TrimSpace(h))
	}
}

func removeExtraHTTPHostPort(req *http.Request) {
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}

	if pHost, port, err := net.SplitHostPort(host); err == nil && port == "80" {
		if M.ParseAddr(pHost).Is6() {
			pHost = "[" + pHost + "]"
		}
		host = pHost
	}

	req.Host = host
	req.URL.Host = host
}

func writeResponseBuffered(conn net.Conn, response *http.Response) error {
	var responseBuffer bytes.Buffer
	err := response.Write(&responseBuffer)
	if err != nil {
		return err
	}
	n, err := conn.Write(responseBuffer.Bytes())
	if err != nil {
		return err
	}
	if n != responseBuffer.Len() {
		return io.ErrShortWrite
	}
	return nil
}

func responseWith(request *http.Request, statusCode int, headers ...string) *http.Response {
	var header http.Header
	if len(headers) > 0 {
		header = make(http.Header)
		for i := 0; i < len(headers); i += 2 {
			header.Add(headers[i], headers[i+1])
		}
	}
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Proto:      request.Proto,
		ProtoMajor: request.ProtoMajor,
		ProtoMinor: request.ProtoMinor,
		Header:     header,
	}
}
