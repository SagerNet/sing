package http

import (
	std_bufio "bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	std_http "net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type testLogger struct {
	mu       sync.Mutex
	messages []string
}

var _ logger.ContextLogger = (*testLogger)(nil)

func (l *testLogger) append(args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, fmt.Sprint(args...))
}

func (l *testLogger) Messages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.messages...)
}

func (l *testLogger) Trace(args ...any) {
	l.append(args...)
}

func (l *testLogger) Debug(args ...any) {
}

func (l *testLogger) Info(args ...any) {
}

func (l *testLogger) Warn(args ...any) {
}

func (l *testLogger) Error(args ...any) {
}

func (l *testLogger) Fatal(args ...any) {
}

func (l *testLogger) Panic(args ...any) {
}

func (l *testLogger) TraceContext(ctx context.Context, args ...any) {
	l.append(args...)
}

func (l *testLogger) DebugContext(ctx context.Context, args ...any) {
}

func (l *testLogger) InfoContext(ctx context.Context, args ...any) {
}

func (l *testLogger) WarnContext(ctx context.Context, args ...any) {
}

func (l *testLogger) ErrorContext(ctx context.Context, args ...any) {
}

func (l *testLogger) FatalContext(ctx context.Context, args ...any) {
}

func (l *testLogger) PanicContext(ctx context.Context, args ...any) {
}

type recordingHandler struct {
	calls atomic.Int32
}

func (h *recordingHandler) NewConnectionEx(_ context.Context, conn net.Conn, _ M.Socksaddr, _ M.Socksaddr, onClose N.CloseHandlerFunc) {
	h.calls.Add(1)
	go func() {
		defer conn.Close()
		request, err := std_http.ReadRequest(std_bufio.NewReader(conn))
		if err != nil {
			if onClose != nil {
				onClose(err)
			}
			return
		}
		if request.Body != nil {
			_ = request.Body.Close()
		}
		_, err = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: keep-alive\r\n\r\nOK")
		if onClose != nil {
			onClose(err)
		}
	}()
}

func startHandshake(t *testing.T, authenticator *auth.Authenticator, handler N.TCPConnectionHandlerEx, options *HTTPServerOptions) (net.Conn, *std_bufio.Reader, <-chan error) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	done := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		if options == nil {
			done <- HandleConnectionEx(context.Background(), serverConn, std_bufio.NewReader(serverConn), authenticator, handler, M.Socksaddr{}, nil)
		} else {
			done <- HandleConnectionExWithOptions(context.Background(), serverConn, std_bufio.NewReader(serverConn), authenticator, handler, M.Socksaddr{}, nil, *options)
		}
	}()
	return clientConn, std_bufio.NewReader(clientConn), done
}

func mustWriteRequest(t *testing.T, conn net.Conn, request string) {
	t.Helper()
	_, err := io.WriteString(conn, request)
	if err != nil {
		t.Fatalf("write request: %v", err)
	}
}

func mustReadResponse(t *testing.T, reader *std_bufio.Reader) *std_http.Response {
	t.Helper()
	request, err := std_http.NewRequest("GET", "http://example.com/", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	response, err := std_http.ReadResponse(reader, request)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return response
}

func waitResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for handshake result")
		return nil
	}
}

func TestHandleConnectionEx_AuthMissingHeaderRetryThenSuccess(t *testing.T) {
	authenticator := auth.NewAuthenticator([]auth.User{{Username: "user", Password: "pass"}})
	handler := &recordingHandler{}
	clientConn, reader, done := startHandshake(t, authenticator, handler, nil)
	defer clientConn.Close()

	mustWriteRequest(t, clientConn, "GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\n\r\n")
	firstResponse := mustReadResponse(t, reader)
	if firstResponse.StatusCode != std_http.StatusProxyAuthRequired {
		t.Fatalf("first response status = %d, want %d", firstResponse.StatusCode, std_http.StatusProxyAuthRequired)
	}
	_ = firstResponse.Body.Close()

	authorization := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	mustWriteRequest(t, clientConn, "GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\nProxy-Authorization: Basic "+authorization+"\r\n\r\n")
	secondResponse := mustReadResponse(t, reader)
	if secondResponse.StatusCode != std_http.StatusOK {
		t.Fatalf("second response status = %d, want %d", secondResponse.StatusCode, std_http.StatusOK)
	}
	_ = secondResponse.Body.Close()

	if handler.calls.Load() != 1 {
		t.Fatalf("handler call count = %d, want 1", handler.calls.Load())
	}

	_ = clientConn.Close()
	if err := waitResult(t, done); err == nil {
		t.Fatal("expected handshake to end with read error after client close")
	}
}

func TestHandleConnectionEx_AuthMissingHeaderRetryOnlyOnce(t *testing.T) {
	authenticator := auth.NewAuthenticator([]auth.User{{Username: "user", Password: "pass"}})
	handler := &recordingHandler{}
	clientConn, reader, done := startHandshake(t, authenticator, handler, nil)
	defer clientConn.Close()

	request := "GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\n\r\n"
	mustWriteRequest(t, clientConn, request)
	firstResponse := mustReadResponse(t, reader)
	if firstResponse.StatusCode != std_http.StatusProxyAuthRequired {
		t.Fatalf("first response status = %d, want %d", firstResponse.StatusCode, std_http.StatusProxyAuthRequired)
	}
	_ = firstResponse.Body.Close()

	mustWriteRequest(t, clientConn, request)
	secondResponse := mustReadResponse(t, reader)
	if secondResponse.StatusCode != std_http.StatusProxyAuthRequired {
		t.Fatalf("second response status = %d, want %d", secondResponse.StatusCode, std_http.StatusProxyAuthRequired)
	}
	_ = secondResponse.Body.Close()

	err := waitResult(t, done)
	if err == nil || !strings.Contains(err.Error(), "no Proxy-Authorization header") {
		t.Fatalf("handshake error = %v, want missing Proxy-Authorization header", err)
	}
	if handler.calls.Load() != 0 {
		t.Fatalf("handler call count = %d, want 0", handler.calls.Load())
	}
}

func TestHandleConnectionEx_AuthMissingHeaderRetryHTTP10KeepAlive(t *testing.T) {
	authenticator := auth.NewAuthenticator([]auth.User{{Username: "user", Password: "pass"}})
	handler := &recordingHandler{}
	clientConn, reader, done := startHandshake(t, authenticator, handler, nil)
	defer clientConn.Close()

	mustWriteRequest(t, clientConn, "GET http://example.com/ HTTP/1.0\r\nHost: example.com\r\nConnection: keep-alive\r\n\r\n")
	firstResponse := mustReadResponse(t, reader)
	if firstResponse.StatusCode != std_http.StatusProxyAuthRequired {
		t.Fatalf("first response status = %d, want %d", firstResponse.StatusCode, std_http.StatusProxyAuthRequired)
	}
	_ = firstResponse.Body.Close()

	authorization := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	mustWriteRequest(t, clientConn, "GET http://example.com/ HTTP/1.0\r\nHost: example.com\r\nConnection: keep-alive\r\nProxy-Authorization: Basic "+authorization+"\r\n\r\n")
	secondResponse := mustReadResponse(t, reader)
	if secondResponse.StatusCode != std_http.StatusOK {
		t.Fatalf("second response status = %d, want %d", secondResponse.StatusCode, std_http.StatusOK)
	}
	_ = secondResponse.Body.Close()

	if handler.calls.Load() != 1 {
		t.Fatalf("handler call count = %d, want 1", handler.calls.Load())
	}

	_ = clientConn.Close()
	if err := waitResult(t, done); err == nil {
		t.Fatal("expected handshake to end with read error after client close")
	}
}

func TestHandleConnectionEx_AuthMissingHeaderNoRetryHTTP10DefaultClose(t *testing.T) {
	authenticator := auth.NewAuthenticator([]auth.User{{Username: "user", Password: "pass"}})
	handler := &recordingHandler{}
	clientConn, reader, done := startHandshake(t, authenticator, handler, nil)
	defer clientConn.Close()

	mustWriteRequest(t, clientConn, "GET http://example.com/ HTTP/1.0\r\nHost: example.com\r\n\r\n")
	response := mustReadResponse(t, reader)
	if response.StatusCode != std_http.StatusProxyAuthRequired {
		t.Fatalf("response status = %d, want %d", response.StatusCode, std_http.StatusProxyAuthRequired)
	}
	_ = response.Body.Close()

	err := waitResult(t, done)
	if err == nil || !strings.Contains(err.Error(), "no Proxy-Authorization header") {
		t.Fatalf("handshake error = %v, want missing Proxy-Authorization header", err)
	}
	if handler.calls.Load() != 0 {
		t.Fatalf("handler call count = %d, want 0", handler.calls.Load())
	}
}

func TestHandleConnectionEx_AuthMissingHeaderNoRetryHTTP10CloseWins(t *testing.T) {
	authenticator := auth.NewAuthenticator([]auth.User{{Username: "user", Password: "pass"}})
	handler := &recordingHandler{}
	clientConn, reader, done := startHandshake(t, authenticator, handler, nil)
	defer clientConn.Close()

	mustWriteRequest(t, clientConn, "GET http://example.com/ HTTP/1.0\r\nHost: example.com\r\nConnection: keep-alive, close\r\n\r\n")
	response := mustReadResponse(t, reader)
	if response.StatusCode != std_http.StatusProxyAuthRequired {
		t.Fatalf("response status = %d, want %d", response.StatusCode, std_http.StatusProxyAuthRequired)
	}
	_ = response.Body.Close()

	err := waitResult(t, done)
	if err == nil || !strings.Contains(err.Error(), "no Proxy-Authorization header") {
		t.Fatalf("handshake error = %v, want missing Proxy-Authorization header", err)
	}
	if handler.calls.Load() != 0 {
		t.Fatalf("handler call count = %d, want 0", handler.calls.Load())
	}
}

func TestHandleConnectionEx_AuthMissingHeaderNoRetryWithRequestBody(t *testing.T) {
	authenticator := auth.NewAuthenticator([]auth.User{{Username: "user", Password: "pass"}})
	handler := &recordingHandler{}
	clientConn, reader, done := startHandshake(t, authenticator, handler, nil)
	defer clientConn.Close()

	mustWriteRequest(t, clientConn, "POST http://example.com/ HTTP/1.1\r\nHost: example.com\r\nProxy-Connection: keep-alive\r\nContent-Length: 4\r\n\r\nping")
	response := mustReadResponse(t, reader)
	if response.StatusCode != std_http.StatusProxyAuthRequired {
		t.Fatalf("response status = %d, want %d", response.StatusCode, std_http.StatusProxyAuthRequired)
	}
	_ = response.Body.Close()

	err := waitResult(t, done)
	if err == nil || !strings.Contains(err.Error(), "no Proxy-Authorization header") {
		t.Fatalf("handshake error = %v, want missing Proxy-Authorization header", err)
	}
	if handler.calls.Load() != 0 {
		t.Fatalf("handler call count = %d, want 0", handler.calls.Load())
	}
}

func TestHandleConnectionEx_AuthMissingHeaderRetryTimeout(t *testing.T) {
	options := &HTTPServerOptions{ProxyAuthRetryTimeout: 100 * time.Millisecond}

	authenticator := auth.NewAuthenticator([]auth.User{{Username: "user", Password: "pass"}})
	handler := &recordingHandler{}
	clientConn, reader, done := startHandshake(t, authenticator, handler, options)
	defer clientConn.Close()

	mustWriteRequest(t, clientConn, "GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\n\r\n")
	response := mustReadResponse(t, reader)
	if response.StatusCode != std_http.StatusProxyAuthRequired {
		t.Fatalf("response status = %d, want %d", response.StatusCode, std_http.StatusProxyAuthRequired)
	}
	_ = response.Body.Close()

	err := waitResult(t, done)
	if err == nil || !strings.Contains(err.Error(), "read http request") {
		t.Fatalf("handshake error = %v, want wrapped read timeout", err)
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("handshake error = %v, want net timeout error", err)
	}
	if handler.calls.Load() != 0 {
		t.Fatalf("handler call count = %d, want 0", handler.calls.Load())
	}
}

func TestHandleConnectionEx_AuthMissingHeaderNoRetryOnUpgrade(t *testing.T) {
	authenticator := auth.NewAuthenticator([]auth.User{{Username: "user", Password: "pass"}})
	handler := &recordingHandler{}
	clientConn, reader, done := startHandshake(t, authenticator, handler, nil)
	defer clientConn.Close()

	mustWriteRequest(t, clientConn, "GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\nConnection: upgrade\r\nUpgrade: websocket\r\n\r\n")
	response := mustReadResponse(t, reader)
	if response.StatusCode != std_http.StatusProxyAuthRequired {
		t.Fatalf("response status = %d, want %d", response.StatusCode, std_http.StatusProxyAuthRequired)
	}
	_ = response.Body.Close()

	err := waitResult(t, done)
	if err == nil || !strings.Contains(err.Error(), "no Proxy-Authorization header") {
		t.Fatalf("handshake error = %v, want missing Proxy-Authorization header", err)
	}
	if handler.calls.Load() != 0 {
		t.Fatalf("handler call count = %d, want 0", handler.calls.Load())
	}
}

func TestHandleConnectionEx_LoggerPrintsRequestAndResponse(t *testing.T) {
	testLog := &testLogger{}
	options := &HTTPServerOptions{Logger: testLog}
	authenticator := auth.NewAuthenticator([]auth.User{{Username: "user", Password: "pass"}})
	handler := &recordingHandler{}

	clientConn, reader, done := startHandshake(t, authenticator, handler, options)
	defer clientConn.Close()

	mustWriteRequest(t, clientConn, "GET http://example.com/ HTTP/1.0\r\nHost: example.com\r\n\r\n")
	response := mustReadResponse(t, reader)
	if response.StatusCode != std_http.StatusProxyAuthRequired {
		t.Fatalf("response status = %d, want %d", response.StatusCode, std_http.StatusProxyAuthRequired)
	}
	_ = response.Body.Close()

	err := waitResult(t, done)
	if err == nil || !strings.Contains(err.Error(), "no Proxy-Authorization header") {
		t.Fatalf("handshake error = %v, want missing Proxy-Authorization header", err)
	}

	logs := testLog.Messages()
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "request protocol: HTTP/1.0") {
		t.Fatalf("logs missing request protocol line, logs: %s", joined)
	}
	if !strings.Contains(joined, "response: protocol=HTTP/1.0 status=407") {
		t.Fatalf("logs missing response status line, logs: %s", joined)
	}
}

func TestHandleConnectionEx_LoggerRedactsSensitiveHeaders(t *testing.T) {
	testLog := &testLogger{}
	options := &HTTPServerOptions{Logger: testLog}
	authenticator := auth.NewAuthenticator([]auth.User{{Username: "user", Password: "pass"}})
	handler := &recordingHandler{}

	clientConn, reader, done := startHandshake(t, authenticator, handler, options)
	defer clientConn.Close()

	authorization := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	mustWriteRequest(t, clientConn, "GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\nProxy-Authorization: Basic "+authorization+"\r\n\r\n")
	response := mustReadResponse(t, reader)
	if response.StatusCode != std_http.StatusOK {
		t.Fatalf("response status = %d, want %d", response.StatusCode, std_http.StatusOK)
	}
	_ = response.Body.Close()

	_ = clientConn.Close()
	if err := waitResult(t, done); err == nil {
		t.Fatal("expected handshake to end with read error after client close")
	}

	joined := strings.Join(testLog.Messages(), "\n")
	if !strings.Contains(joined, "request header: Proxy-Authorization: [redacted]") {
		t.Fatalf("logs missing redacted proxy authorization header, logs: %s", joined)
	}
	if strings.Contains(joined, authorization) || strings.Contains(joined, "user:pass") {
		t.Fatalf("logs unexpectedly contain sensitive credentials, logs: %s", joined)
	}
}
