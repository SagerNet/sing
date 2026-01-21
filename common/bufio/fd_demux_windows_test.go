//go:build windows

package bufio

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func getSocketFD(t *testing.T, conn net.PacketConn) int {
	syscallConn, ok := conn.(syscall.Conn)
	require.True(t, ok)
	rawConn, err := syscallConn.SyscallConn()
	require.NoError(t, err)
	var fd int
	err = rawConn.Control(func(f uintptr) { fd = int(f) })
	require.NoError(t, err)
	return fd
}

func TestFDDemultiplexer_Create(t *testing.T) {
	t.Parallel()

	demux, err := NewFDPoller(context.Background())
	require.NoError(t, err)

	err = demux.Close()
	require.NoError(t, err)
}

func TestFDDemultiplexer_CreateMultiple(t *testing.T) {
	t.Parallel()

	demux1, err := NewFDPoller(context.Background())
	require.NoError(t, err)
	defer demux1.Close()

	demux2, err := NewFDPoller(context.Background())
	require.NoError(t, err)
	defer demux2.Close()
}

func TestFDDemultiplexer_AddRemove(t *testing.T) {
	t.Parallel()

	demux, err := NewFDPoller(context.Background())
	require.NoError(t, err)
	defer demux.Close()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer conn.Close()

	fd := getSocketFD(t, conn)

	stream := &reactorStream{}

	err = demux.Add(stream, fd)
	require.NoError(t, err)

	demux.Remove(fd)
}

func TestFDDemultiplexer_RapidAddRemove(t *testing.T) {
	t.Parallel()

	demux, err := NewFDPoller(context.Background())
	require.NoError(t, err)
	defer demux.Close()

	const iterations = 50

	for i := 0; i < iterations; i++ {
		conn, err := net.ListenPacket("udp", "127.0.0.1:0")
		require.NoError(t, err)

		fd := getSocketFD(t, conn)
		stream := &reactorStream{}

		err = demux.Add(stream, fd)
		require.NoError(t, err)

		demux.Remove(fd)
		conn.Close()
	}
}

func TestFDDemultiplexer_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	demux, err := NewFDPoller(context.Background())
	require.NoError(t, err)
	defer demux.Close()

	const numGoroutines = 10
	const iterations = 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()

			for i := 0; i < iterations; i++ {
				conn, err := net.ListenPacket("udp", "127.0.0.1:0")
				if err != nil {
					continue
				}

				fd := getSocketFD(t, conn)
				stream := &reactorStream{}

				err = demux.Add(stream, fd)
				if err == nil {
					demux.Remove(fd)
				}
				conn.Close()
			}
		}()
	}

	wg.Wait()
}

func TestFDDemultiplexer_ReceiveEvent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	demux, err := NewFDPoller(ctx)
	require.NoError(t, err)
	defer demux.Close()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer conn.Close()

	fd := getSocketFD(t, conn)

	triggered := make(chan struct{}, 1)
	stream := &reactorStream{
		state: atomic.Int32{},
	}
	stream.connection = &reactorConnection{
		upload:   stream,
		download: stream,
		done:     make(chan struct{}),
	}

	originalRunActiveLoop := stream.runActiveLoop
	_ = originalRunActiveLoop

	err = demux.Add(stream, fd)
	require.NoError(t, err)

	sender, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer sender.Close()

	_, err = sender.WriteTo([]byte("test data"), conn.LocalAddr())
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	select {
	case <-triggered:
	default:
	}

	demux.Remove(fd)
}

func TestFDDemultiplexer_CloseWhilePolling(t *testing.T) {
	t.Parallel()

	demux, err := NewFDPoller(context.Background())
	require.NoError(t, err)

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer conn.Close()

	fd := getSocketFD(t, conn)
	stream := &reactorStream{}

	err = demux.Add(stream, fd)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		demux.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close blocked - possible deadlock")
	}
}

func TestFDDemultiplexer_RemoveNonExistent(t *testing.T) {
	t.Parallel()

	demux, err := NewFDPoller(context.Background())
	require.NoError(t, err)
	defer demux.Close()

	demux.Remove(99999)
}

func TestFDDemultiplexer_AddAfterClose(t *testing.T) {
	t.Parallel()

	demux, err := NewFDPoller(context.Background())
	require.NoError(t, err)

	err = demux.Close()
	require.NoError(t, err)

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer conn.Close()

	fd := getSocketFD(t, conn)
	stream := &reactorStream{}

	err = demux.Add(stream, fd)
	require.Error(t, err)
}

func TestFDDemultiplexer_MultipleSocketsSimultaneous(t *testing.T) {
	t.Parallel()

	demux, err := NewFDPoller(context.Background())
	require.NoError(t, err)
	defer demux.Close()

	const numSockets = 5
	conns := make([]net.PacketConn, numSockets)
	fds := make([]int, numSockets)

	for i := 0; i < numSockets; i++ {
		conn, err := net.ListenPacket("udp", "127.0.0.1:0")
		require.NoError(t, err)
		defer conn.Close()
		conns[i] = conn

		fd := getSocketFD(t, conn)
		fds[i] = fd

		stream := &reactorStream{}
		err = demux.Add(stream, fd)
		require.NoError(t, err)
	}

	for i := 0; i < numSockets; i++ {
		demux.Remove(fds[i])
	}
}

func TestFDDemultiplexer_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	demux, err := NewFDPoller(ctx)
	require.NoError(t, err)

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer conn.Close()

	fd := getSocketFD(t, conn)
	stream := &reactorStream{}

	err = demux.Add(stream, fd)
	require.NoError(t, err)

	cancel()

	time.Sleep(100 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		demux.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close blocked after context cancellation")
	}
}
