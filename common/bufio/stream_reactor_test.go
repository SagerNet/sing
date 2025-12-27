//go:build darwin || linux || windows

package bufio

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/sagernet/sing/common/buf"
	N "github.com/sagernet/sing/common/network"

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

// failingConn wraps net.Conn and fails writes after N calls
type failingConn struct {
	net.Conn
	failAfter  int
	writeCount atomic.Int32
}

func (c *failingConn) Write(p []byte) (int, error) {
	if c.writeCount.Add(1) > int32(c.failAfter) {
		return 0, errors.New("simulated write error")
	}
	return c.Conn.Write(p)
}

// errorConn returns error on Read
type errorConn struct {
	net.Conn
	readError error
}

func (c *errorConn) Read(p []byte) (int, error) {
	return 0, c.readError
}

// countingConn wraps net.Conn with read/write counters
type countingConn struct {
	net.Conn
	readCount  atomic.Int64
	writeCount atomic.Int64
}

func (c *countingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.readCount.Add(int64(n))
	return n, err
}

func (c *countingConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.writeCount.Add(int64(n))
	return n, err
}

func (c *countingConn) UnwrapReader() (io.Reader, []N.CountFunc) {
	return c.Conn, []N.CountFunc{func(n int64) { c.readCount.Add(n) }}
}

func (c *countingConn) UnwrapWriter() (io.Writer, []N.CountFunc) {
	return c.Conn, []N.CountFunc{func(n int64) { c.writeCount.Add(n) }}
}

func TestStreamReactor_WriteError(t *testing.T) {
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

	// Wrap destination with failing writer that fails after 3 writes
	failingDest := &failingConn{Conn: client2, failAfter: 3}

	var capturedErr error
	closeDone := make(chan struct{})
	reactor.Copy(ctx, server1, failingDest, func(err error) {
		capturedErr = err
		close(closeDone)
	})

	// Send multiple chunks of data to trigger the write failure
	testData := make([]byte, 1024)
	rand.Read(testData)

	for i := 0; i < 10; i++ {
		_, err := client1.Write(testData)
		if err != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Close source to trigger cleanup
	client1.Close()
	server2.Close()

	select {
	case <-closeDone:
		// Verify error was propagated
		assert.NotNil(t, capturedErr, "expected error to be propagated")
		assert.Contains(t, capturedErr.Error(), "simulated write error")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for close callback")
	}
}

func TestStreamReactor_ReadError(t *testing.T) {
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

	// Wrap source with error-returning reader
	readErr := errors.New("simulated read error")
	errorSrc := &errorConn{Conn: server1, readError: readErr}

	var capturedErr error
	closeDone := make(chan struct{})
	reactor.Copy(ctx, errorSrc, client2, func(err error) {
		capturedErr = err
		close(closeDone)
	})

	select {
	case <-closeDone:
		// Verify error was propagated
		assert.NotNil(t, capturedErr, "expected error to be propagated")
		assert.Contains(t, capturedErr.Error(), "simulated read error")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for close callback")
	}
}

func TestStreamReactor_Counters(t *testing.T) {
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

	// Wrap with counting connections
	countingSrc := &countingConn{Conn: server1}
	countingDst := &countingConn{Conn: client2}

	closeDone := make(chan struct{})
	reactor.Copy(ctx, countingSrc, countingDst, func(err error) {
		close(closeDone)
	})

	// Send data in both directions
	const dataSize = 4096
	uploadData := make([]byte, dataSize)
	downloadData := make([]byte, dataSize)
	rand.Read(uploadData)
	rand.Read(downloadData)

	// Upload: client1 -> server1 -> client2 -> server2
	_, err := client1.Write(uploadData)
	require.NoError(t, err)

	received := make([]byte, dataSize)
	_, err = io.ReadFull(server2, received)
	require.NoError(t, err)
	assert.Equal(t, uploadData, received)

	// Download: server2 -> client2 -> server1 -> client1
	_, err = server2.Write(downloadData)
	require.NoError(t, err)

	received2 := make([]byte, dataSize)
	_, err = io.ReadFull(client1, received2)
	require.NoError(t, err)
	assert.Equal(t, downloadData, received2)

	// Close connections
	client1.Close()
	server2.Close()

	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for close callback")
	}

	// Verify counters (read from source, write to destination)
	// Note: The countingConn tracks actual reads/writes at the connection level
	assert.True(t, countingSrc.readCount.Load() >= dataSize, "source should have read at least %d bytes, got %d", dataSize, countingSrc.readCount.Load())
	assert.True(t, countingDst.writeCount.Load() >= dataSize, "destination should have written at least %d bytes, got %d", dataSize, countingDst.writeCount.Load())
}

func TestStreamReactor_CachedReader(t *testing.T) {
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

	// Create cached data
	cachedData := make([]byte, 512)
	rand.Read(cachedData)
	cachedBuffer := buf.As(cachedData)

	// Wrap source with cached conn
	cachedSrc := NewCachedConn(server1, cachedBuffer)
	defer cachedSrc.Close()

	closeDone := make(chan struct{})
	reactor.Copy(ctx, cachedSrc, client2, func(err error) {
		close(closeDone)
	})

	// The cached data should be sent first before any new data
	// Read cached data from destination
	receivedCached := make([]byte, len(cachedData))
	_, err := io.ReadFull(server2, receivedCached)
	require.NoError(t, err)
	assert.Equal(t, cachedData, receivedCached, "cached data should be received first")

	// Now send new data through the connection
	newData := make([]byte, 256)
	rand.Read(newData)

	_, err = client1.Write(newData)
	require.NoError(t, err)

	receivedNew := make([]byte, len(newData))
	_, err = io.ReadFull(server2, receivedNew)
	require.NoError(t, err)
	assert.Equal(t, newData, receivedNew, "new data should be received after cached data")

	// Cleanup
	client1.Close()
	server2.Close()

	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for close callback")
	}
}

func TestStreamReactor_LargeData(t *testing.T) {
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

	fdServer1 := newFDConn(t, server1)
	fdClient2 := newFDConn(t, client2)

	closeDone := make(chan struct{})
	reactor.Copy(ctx, fdServer1, fdClient2, func(err error) {
		close(closeDone)
	})

	// Test with 10MB of data
	const dataSize = 10 * 1024 * 1024
	uploadData := make([]byte, dataSize)
	rand.Read(uploadData)
	uploadHash := md5.Sum(uploadData)

	downloadData := make([]byte, dataSize)
	rand.Read(downloadData)
	downloadHash := md5.Sum(downloadData)

	var wg sync.WaitGroup
	errChan := make(chan error, 4)

	// Upload goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := client1.Write(uploadData)
		if err != nil {
			errChan <- err
		}
	}()

	// Upload receiver goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		received := make([]byte, dataSize)
		_, err := io.ReadFull(server2, received)
		if err != nil {
			errChan <- err
			return
		}
		receivedHash := md5.Sum(received)
		if receivedHash != uploadHash {
			errChan <- errors.New("upload data mismatch")
		}
	}()

	// Download goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := server2.Write(downloadData)
		if err != nil {
			errChan <- err
		}
	}()

	// Download receiver goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		received := make([]byte, dataSize)
		_, err := io.ReadFull(client1, received)
		if err != nil {
			errChan <- err
			return
		}
		receivedHash := md5.Sum(received)
		if receivedHash != downloadHash {
			errChan <- errors.New("download data mismatch")
		}
	}()

	// Wait for completion
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case err := <-errChan:
		t.Fatalf("transfer error: %v", err)
	case <-done:
		// Success
	case <-time.After(60 * time.Second):
		t.Fatal("timeout during large data transfer")
	}

	// Cleanup
	client1.Close()
	server2.Close()

	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for close callback")
	}
}

func TestStreamReactor_BufferedAtRegistration(t *testing.T) {
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

	// Create buffered conn with pre-populated buffer
	bufferedServer1 := newBufferedConn(t, server1)
	defer bufferedServer1.Close()

	// Pre-populate the buffer with data
	preBufferedData := []byte("pre-buffered data that should be sent immediately")
	bufferedServer1.bufferMu.Lock()
	bufferedServer1.buffer.Write(preBufferedData)
	bufferedServer1.bufferMu.Unlock()

	closeDone := make(chan struct{})
	reactor.Copy(ctx, bufferedServer1, client2, func(err error) {
		close(closeDone)
	})

	// The pre-buffered data should be processed immediately
	received := make([]byte, len(preBufferedData))
	server2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err := io.ReadFull(server2, received)
	require.NoError(t, err)
	assert.Equal(t, preBufferedData, received, "pre-buffered data should be received")

	// Now send additional data
	additionalData := []byte("additional data")
	_, err = client1.Write(additionalData)
	require.NoError(t, err)

	additionalReceived := make([]byte, len(additionalData))
	server2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = io.ReadFull(server2, additionalReceived)
	require.NoError(t, err)
	assert.Equal(t, additionalData, additionalReceived)

	// Cleanup
	client1.Close()
	server2.Close()

	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for close callback")
	}
}
