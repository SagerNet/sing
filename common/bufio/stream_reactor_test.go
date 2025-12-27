//go:build darwin || linux || windows

package bufio

import (
	"context"
	"crypto/rand"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/sagernet/sing/common/buf"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fdConn wraps a net.Conn to implement StreamNotifier
type fdConn struct {
	net.Conn
	fd int
}

func newFDConn(t *testing.T, conn net.Conn) *fdConn {
	syscallConn, ok := conn.(syscall.Conn)
	require.True(t, ok, "connection must implement syscall.Conn")
	rawConn, err := syscallConn.SyscallConn()
	require.NoError(t, err)
	var fd int
	err = rawConn.Control(func(f uintptr) { fd = int(f) })
	require.NoError(t, err)
	return &fdConn{
		Conn: conn,
		fd:   fd,
	}
}

func (c *fdConn) FD() int {
	return c.fd
}

func (c *fdConn) Buffered() int {
	return 0
}

// bufferedConn wraps a net.Conn with a buffer for testing StreamNotifier
type bufferedConn struct {
	net.Conn
	buffer   *buf.Buffer
	bufferMu sync.Mutex
	fd       int
}

func newBufferedConn(t *testing.T, conn net.Conn) *bufferedConn {
	bc := &bufferedConn{
		Conn:   conn,
		buffer: buf.New(),
	}
	if syscallConn, ok := conn.(syscall.Conn); ok {
		rawConn, err := syscallConn.SyscallConn()
		if err == nil {
			rawConn.Control(func(f uintptr) { bc.fd = int(f) })
		}
	}
	return bc
}

func (c *bufferedConn) Read(p []byte) (n int, err error) {
	c.bufferMu.Lock()
	if c.buffer.Len() > 0 {
		n = copy(p, c.buffer.Bytes())
		c.buffer.Advance(n)
		c.bufferMu.Unlock()
		return n, nil
	}
	c.bufferMu.Unlock()
	return c.Conn.Read(p)
}

func (c *bufferedConn) FD() int {
	return c.fd
}

func (c *bufferedConn) Buffered() int {
	c.bufferMu.Lock()
	defer c.bufferMu.Unlock()
	return c.buffer.Len()
}

func (c *bufferedConn) Close() error {
	c.buffer.Release()
	return c.Conn.Close()
}

func TestStreamReactor_Basic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	reactor := NewStreamReactor(ctx)
	defer reactor.Close()

	// Create a pair of connected TCP connections
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	var serverConn net.Conn
	var serverErr error
	serverDone := make(chan struct{})
	go func() {
		serverConn, serverErr = listener.Accept()
		close(serverDone)
	}()

	clientConn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer clientConn.Close()

	<-serverDone
	require.NoError(t, serverErr)
	defer serverConn.Close()

	// Create another pair for the destination
	listener2, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener2.Close()

	var destServerConn net.Conn
	var destServerErr error
	destServerDone := make(chan struct{})
	go func() {
		destServerConn, destServerErr = listener2.Accept()
		close(destServerDone)
	}()

	destClientConn, err := net.Dial("tcp", listener2.Addr().String())
	require.NoError(t, err)
	defer destClientConn.Close()

	<-destServerDone
	require.NoError(t, destServerErr)
	defer destServerConn.Close()

	// Test data transfer
	testData := make([]byte, 1024)
	rand.Read(testData)

	closeDone := make(chan struct{})
	reactor.Copy(ctx, serverConn, destClientConn, func(err error) {
		close(closeDone)
	})

	// Write from client to server, should pass through to dest
	_, err = clientConn.Write(testData)
	require.NoError(t, err)

	// Read from destServerConn
	received := make([]byte, len(testData))
	_, err = io.ReadFull(destServerConn, received)
	require.NoError(t, err)
	assert.Equal(t, testData, received)

	// Test reverse direction
	reverseData := make([]byte, 512)
	rand.Read(reverseData)

	_, err = destServerConn.Write(reverseData)
	require.NoError(t, err)

	reverseReceived := make([]byte, len(reverseData))
	_, err = io.ReadFull(clientConn, reverseReceived)
	require.NoError(t, err)
	assert.Equal(t, reverseData, reverseReceived)

	// Close and wait
	clientConn.Close()
	destServerConn.Close()

	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for close callback")
	}
}

func TestStreamReactor_FDNotifier(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	reactor := NewStreamReactor(ctx)
	defer reactor.Close()

	// Create TCP connection pairs
	server1, client1 := createTCPPair(t)
	defer server1.Close()
	defer client1.Close()

	server2, client2 := createTCPPair(t)
	defer server2.Close()
	defer client2.Close()

	// Wrap with FD notifier
	fdServer1 := newFDConn(t, server1)
	fdClient2 := newFDConn(t, client2)

	closeDone := make(chan struct{})
	reactor.Copy(ctx, fdServer1, fdClient2, func(err error) {
		close(closeDone)
	})

	// Test data transfer
	testData := make([]byte, 2048)
	rand.Read(testData)

	_, err := client1.Write(testData)
	require.NoError(t, err)

	received := make([]byte, len(testData))
	_, err = io.ReadFull(server2, received)
	require.NoError(t, err)
	assert.Equal(t, testData, received)

	client1.Close()
	server2.Close()

	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for close callback")
	}
}

func TestStreamReactor_BufferedReader(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	reactor := NewStreamReactor(ctx)
	defer reactor.Close()

	server1, client1 := createTCPPair(t)
	defer server1.Close()
	defer client1.Close()

	server2, client2 := createTCPPair(t)
	defer server2.Close()
	defer client2.Close()

	// Use buffered conn
	bufferedServer1 := newBufferedConn(t, server1)
	defer bufferedServer1.Close()

	closeDone := make(chan struct{})
	reactor.Copy(ctx, bufferedServer1, client2, func(err error) {
		close(closeDone)
	})

	// Send data
	testData := make([]byte, 1024)
	rand.Read(testData)

	_, err := client1.Write(testData)
	require.NoError(t, err)

	received := make([]byte, len(testData))
	_, err = io.ReadFull(server2, received)
	require.NoError(t, err)
	assert.Equal(t, testData, received)

	client1.Close()
	server2.Close()

	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for close callback")
	}
}

func TestStreamReactor_HalfClose(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	reactor := NewStreamReactor(ctx)
	defer reactor.Close()

	server1, client1 := createTCPPair(t)
	defer server1.Close()
	defer client1.Close()

	server2, client2 := createTCPPair(t)
	defer server2.Close()
	defer client2.Close()

	closeDone := make(chan struct{})
	reactor.Copy(ctx, server1, client2, func(err error) {
		close(closeDone)
	})

	// Send data in one direction
	testData := make([]byte, 512)
	rand.Read(testData)

	_, err := client1.Write(testData)
	require.NoError(t, err)

	received := make([]byte, len(testData))
	_, err = io.ReadFull(server2, received)
	require.NoError(t, err)
	assert.Equal(t, testData, received)

	// Close client1's write side
	if tcpConn, ok := client1.(*net.TCPConn); ok {
		tcpConn.CloseWrite()
	} else {
		client1.Close()
	}

	// The other direction should still work for a moment
	reverseData := make([]byte, 256)
	rand.Read(reverseData)

	_, err = server2.Write(reverseData)
	require.NoError(t, err)

	// Eventually both will close
	server2.Close()
	client1.Close()

	select {
	case <-closeDone:
		// closeErr should be nil for graceful close
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for close callback")
	}
}

func TestStreamReactor_MultipleConnections(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	reactor := NewStreamReactor(ctx)
	defer reactor.Close()

	const numConnections = 10
	var wg sync.WaitGroup
	var completedCount atomic.Int32

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			server1, client1 := createTCPPair(t)
			defer server1.Close()
			defer client1.Close()

			server2, client2 := createTCPPair(t)
			defer server2.Close()
			defer client2.Close()

			closeDone := make(chan struct{})
			reactor.Copy(ctx, server1, client2, func(err error) {
				close(closeDone)
			})

			// Send unique data
			testData := make([]byte, 256)
			rand.Read(testData)

			_, err := client1.Write(testData)
			require.NoError(t, err)

			received := make([]byte, len(testData))
			_, err = io.ReadFull(server2, received)
			require.NoError(t, err)
			assert.Equal(t, testData, received)

			client1.Close()
			server2.Close()

			select {
			case <-closeDone:
				completedCount.Add(1)
			case <-time.After(5 * time.Second):
				t.Errorf("connection %d: timeout waiting for close callback", idx)
			}
		}(i)
	}

	wg.Wait()
	assert.Equal(t, int32(numConnections), completedCount.Load())
}

func TestStreamReactor_ReactorClose(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	reactor := NewStreamReactor(ctx)

	server1, client1 := createTCPPair(t)
	defer server1.Close()
	defer client1.Close()

	server2, client2 := createTCPPair(t)
	defer server2.Close()
	defer client2.Close()

	closeDone := make(chan struct{})
	reactor.Copy(ctx, server1, client2, func(err error) {
		close(closeDone)
	})

	// Send some data first
	testData := make([]byte, 128)
	rand.Read(testData)

	_, err := client1.Write(testData)
	require.NoError(t, err)

	received := make([]byte, len(testData))
	_, err = io.ReadFull(server2, received)
	require.NoError(t, err)

	// Close the reactor
	reactor.Close()

	// Close connections
	client1.Close()
	server2.Close()

	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for close callback after reactor close")
	}
}

// Helper function to create a connected TCP pair
func createTCPPair(t *testing.T) (net.Conn, net.Conn) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	var serverConn net.Conn
	var serverErr error
	serverDone := make(chan struct{})
	go func() {
		serverConn, serverErr = listener.Accept()
		close(serverDone)
	}()

	clientConn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)

	<-serverDone
	require.NoError(t, serverErr)

	return serverConn, clientConn
}
