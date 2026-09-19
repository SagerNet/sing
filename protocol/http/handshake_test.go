package http

import (
	std_bufio "bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"github.com/stretchr/testify/require"
)

const stallAfter = 128 * 1024

type stallConn struct {
	net.Conn
	remaining int
	release   <-chan struct{}
}

func (c *stallConn) Read(p []byte) (int, error) {
	if c.remaining <= 0 {
		<-c.release
	} else if len(p) > c.remaining {
		p = p[:c.remaining]
	}
	n, err := c.Conn.Read(p)
	c.remaining -= n
	return n, err
}

type readCounter struct {
	io.Reader
	count int
}

func (r *readCounter) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.count += n
	return n, err
}

type dialHandler struct {
	release <-chan struct{}
}

func (h *dialHandler) NewConnectionEx(ctx context.Context, conn net.Conn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	backendConn, err := net.Dial("tcp", destination.String())
	if err != nil {
		N.CloseOnHandshakeFailure(conn, onClose, err)
		return
	}
	err = bufio.CopyConn(ctx, &stallConn{Conn: conn, remaining: stallAfter, release: h.release}, backendConn)
	if onClose != nil {
		onClose(err)
	}
}

func TestKeepAliveAfterEarlyResponse(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { backendListener.Close() })
	go func() {
		for {
			backendConn, acceptErr := backendListener.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				defer backendConn.Close()
				counter := &readCounter{Reader: backendConn}
				reader := std_bufio.NewReader(counter)
				request, readErr := http.ReadRequest(reader)
				if readErr != nil {
					return
				}
				if request.URL.Path == "/upload" {
					if request.ContentLength > stallAfter {
						_, readErr = io.CopyN(io.Discard, reader, int64(stallAfter-counter.count+reader.Buffered()))
						if readErr != nil {
							return
						}
					}
					backendConn.Write([]byte("HTTP/1.1 413 Payload Too Large\r\nContent-Length: 5\r\n\r\nerror"))
					<-release
					return
				}
				backendConn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))
			}()
		}
	}()
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { proxyListener.Close() })
	go func() {
		for {
			proxyConn, acceptErr := proxyListener.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				HandleConnectionEx(context.Background(), proxyConn, std_bufio.NewReader(proxyConn), nil, &dialHandler{release: release}, M.SocksaddrFromNet(proxyConn.RemoteAddr()), nil)
				proxyConn.Close()
			}()
		}
	}()
	backend := backendListener.Addr().String()
	dialProxy := func(t *testing.T) net.Conn {
		client, dialErr := net.Dial("tcp", proxyListener.Addr().String())
		require.NoError(t, dialErr)
		t.Cleanup(func() { client.Close() })
		dialErr = client.SetDeadline(time.Now().Add(10 * time.Second))
		require.NoError(t, dialErr)
		return client
	}
	expectResponse := func(t *testing.T, reader *std_bufio.Reader, statusCode int, content string) {
		response, readErr := http.ReadResponse(reader, nil)
		require.NoError(t, readErr)
		require.Equal(t, statusCode, response.StatusCode)
		responseContent, readErr := io.ReadAll(response.Body)
		require.NoError(t, readErr)
		require.Equal(t, content, string(responseContent))
	}
	t.Run("upstream_blocked", func(t *testing.T) {
		t.Parallel()
		client := dialProxy(t)
		clientReader := std_bufio.NewReader(client)
		body := bytes.Repeat([]byte{'\n'}, 300*1024)
		for range 3 {
			_, writeErr := fmt.Fprintf(client, "POST http://%s/upload HTTP/1.1\r\nHost: %s\r\nProxy-Connection: keep-alive\r\nContent-Length: %d\r\n\r\n%s", backend, backend, len(body), body)
			require.NoError(t, writeErr)
			expectResponse(t, clientReader, http.StatusRequestEntityTooLarge, "error")
			_, writeErr = fmt.Fprintf(client, "GET http://%s/next HTTP/1.1\r\nHost: %s\r\nProxy-Connection: keep-alive\r\n\r\n", backend, backend)
			require.NoError(t, writeErr)
			expectResponse(t, clientReader, http.StatusOK, "ok")
		}
	})
	t.Run("client_stalled", func(t *testing.T) {
		t.Parallel()
		client := dialProxy(t)
		body := bytes.Repeat([]byte{'\n'}, 64*1024)
		_, writeErr := fmt.Fprintf(client, "POST http://%s/upload HTTP/1.1\r\nHost: %s\r\nProxy-Connection: keep-alive\r\nContent-Length: %d\r\n\r\n%s", backend, backend, len(body), body[:len(body)/2])
		require.NoError(t, writeErr)
		clientReader := std_bufio.NewReader(client)
		expectResponse(t, clientReader, http.StatusRequestEntityTooLarge, "error")
		for offset := len(body) / 2; offset < len(body); offset += 8 * 1024 {
			_, writeErr = client.Write(body[offset : offset+8*1024])
			require.NoError(t, writeErr)
			time.Sleep(20 * time.Millisecond)
		}
		_, writeErr = fmt.Fprintf(client, "GET http://%s/next HTTP/1.1\r\nHost: %s\r\nProxy-Connection: keep-alive\r\n\r\n", backend, backend)
		require.NoError(t, writeErr)
		expectResponse(t, clientReader, http.StatusOK, "ok")
	})
}
