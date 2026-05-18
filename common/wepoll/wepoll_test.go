//go:build windows

package wepoll

import (
	"net"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func createTestIOCP(t *testing.T) windows.Handle {
	iocp, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		windows.CloseHandle(iocp)
	})
	return iocp
}

func getSocketHandle(t *testing.T, conn net.PacketConn) windows.Handle {
	syscallConn, ok := conn.(syscall.Conn)
	require.True(t, ok)
	rawConn, err := syscallConn.SyscallConn()
	require.NoError(t, err)
	var fd uintptr
	err = rawConn.Control(func(f uintptr) { fd = f })
	require.NoError(t, err)
	return windows.Handle(fd)
}

func getTCPSocketHandle(t *testing.T, conn net.Conn) windows.Handle {
	syscallConn, ok := conn.(syscall.Conn)
	require.True(t, ok)
	rawConn, err := syscallConn.SyscallConn()
	require.NoError(t, err)
	var fd uintptr
	err = rawConn.Control(func(f uintptr) { fd = f })
	require.NoError(t, err)
	return windows.Handle(fd)
}

func TestNewAFD(t *testing.T) {
	t.Parallel()

	iocp := createTestIOCP(t)

	afd, err := NewAFD(iocp, "test")
	require.NoError(t, err)
	require.NotNil(t, afd)

	err = afd.Close()
	require.NoError(t, err)
}

func TestNewAFD_MultipleTimes(t *testing.T) {
	t.Parallel()

	iocp := createTestIOCP(t)

	afd1, err := NewAFD(iocp, "test1")
	require.NoError(t, err)
	defer afd1.Close()

	afd2, err := NewAFD(iocp, "test2")
	require.NoError(t, err)
	defer afd2.Close()

	afd3, err := NewAFD(iocp, "test3")
	require.NoError(t, err)
	defer afd3.Close()
}

func TestGetBaseSocket_UDP(t *testing.T) {
	t.Parallel()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer conn.Close()

	handle := getSocketHandle(t, conn)
	baseHandle, err := GetBaseSocket(handle)
	require.NoError(t, err)
	require.NotEqual(t, windows.InvalidHandle, baseHandle)
}

func TestGetBaseSocket_TCP(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	handle := getTCPSocketHandle(t, conn)
	baseHandle, err := GetBaseSocket(handle)
	require.NoError(t, err)
	require.NotEqual(t, windows.InvalidHandle, baseHandle)
}

func TestAFD_Poll_ReceiveEvent(t *testing.T) {
	t.Parallel()

	iocp := createTestIOCP(t)

	afd, err := NewAFD(iocp, "poll_test")
	require.NoError(t, err)
	defer afd.Close()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer conn.Close()

	handle := getSocketHandle(t, conn)
	baseHandle, err := GetBaseSocket(handle)
	require.NoError(t, err)

	var state struct {
		iosb     windows.IO_STATUS_BLOCK
		pollInfo AFDPollInfo
	}

	var pinner Pinner
	pinner.Pin(&state)
	defer pinner.Unpin()

	events := uint32(AFD_POLL_RECEIVE | AFD_POLL_DISCONNECT | AFD_POLL_ABORT)
	err = afd.Poll(baseHandle, events, &state.iosb, &state.pollInfo)
	require.NoError(t, err)

	sender, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer sender.Close()

	_, err = sender.WriteTo([]byte("test data"), conn.LocalAddr())
	require.NoError(t, err)

	entries := make([]OverlappedEntry, 1)
	var numRemoved uint32
	err = GetQueuedCompletionStatusEx(iocp, &entries[0], 1, &numRemoved, 5000, false)
	require.NoError(t, err)
	require.Equal(t, uint32(1), numRemoved)
	require.Equal(t, uintptr(unsafe.Pointer(&state.iosb)), uintptr(unsafe.Pointer(entries[0].Overlapped)))
}

func TestAFD_Cancel(t *testing.T) {
	t.Parallel()

	iocp := createTestIOCP(t)

	afd, err := NewAFD(iocp, "cancel_test")
	require.NoError(t, err)
	defer afd.Close()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer conn.Close()

	handle := getSocketHandle(t, conn)
	baseHandle, err := GetBaseSocket(handle)
	require.NoError(t, err)

	var state struct {
		iosb     windows.IO_STATUS_BLOCK
		pollInfo AFDPollInfo
	}

	var pinner Pinner
	pinner.Pin(&state)
	defer pinner.Unpin()

	events := uint32(AFD_POLL_RECEIVE)
	err = afd.Poll(baseHandle, events, &state.iosb, &state.pollInfo)
	require.NoError(t, err)

	err = afd.Cancel(&state.iosb)
	require.NoError(t, err)

	entries := make([]OverlappedEntry, 1)
	var numRemoved uint32
	err = GetQueuedCompletionStatusEx(iocp, &entries[0], 1, &numRemoved, 1000, false)
	require.NoError(t, err)
	require.Equal(t, uint32(1), numRemoved)
}

func TestAFD_Close(t *testing.T) {
	t.Parallel()

	iocp := createTestIOCP(t)

	afd, err := NewAFD(iocp, "close_test")
	require.NoError(t, err)

	err = afd.Close()
	require.NoError(t, err)
}

func TestGetQueuedCompletionStatusEx_Timeout(t *testing.T) {
	t.Parallel()

	iocp := createTestIOCP(t)

	entries := make([]OverlappedEntry, 1)
	var numRemoved uint32

	start := time.Now()
	err := GetQueuedCompletionStatusEx(iocp, &entries[0], 1, &numRemoved, 100, false)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.GreaterOrEqual(t, elapsed, 50*time.Millisecond)
}

func TestGetQueuedCompletionStatusEx_MultipleEntries(t *testing.T) {
	t.Parallel()

	iocp := createTestIOCP(t)

	afd, err := NewAFD(iocp, "multi_test")
	require.NoError(t, err)
	defer afd.Close()

	const numConns = 3
	conns := make([]net.PacketConn, numConns)
	states := make([]struct {
		iosb     windows.IO_STATUS_BLOCK
		pollInfo AFDPollInfo
	}, numConns)
	pinners := make([]Pinner, numConns)

	for i := 0; i < numConns; i++ {
		conn, err := net.ListenPacket("udp", "127.0.0.1:0")
		require.NoError(t, err)
		defer conn.Close()
		conns[i] = conn

		handle := getSocketHandle(t, conn)
		baseHandle, err := GetBaseSocket(handle)
		require.NoError(t, err)

		pinners[i].Pin(&states[i])
		defer pinners[i].Unpin()

		events := uint32(AFD_POLL_RECEIVE)
		err = afd.Poll(baseHandle, events, &states[i].iosb, &states[i].pollInfo)
		require.NoError(t, err)
	}

	sender, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer sender.Close()

	for i := 0; i < numConns; i++ {
		_, err = sender.WriteTo([]byte("test"), conns[i].LocalAddr())
		require.NoError(t, err)
	}

	entries := make([]OverlappedEntry, 8)
	var numRemoved uint32
	received := 0
	for received < numConns {
		err = GetQueuedCompletionStatusEx(iocp, &entries[0], 8, &numRemoved, 5000, false)
		require.NoError(t, err)
		received += int(numRemoved)
	}
	require.Equal(t, numConns, received)
}

func TestAFD_Poll_DisconnectEvent(t *testing.T) {
	t.Parallel()

	iocp := createTestIOCP(t)

	afd, err := NewAFD(iocp, "disconnect_test")
	require.NoError(t, err)
	defer afd.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
		conn.Close()
	}()

	client, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer client.Close()

	handle := getTCPSocketHandle(t, client)
	baseHandle, err := GetBaseSocket(handle)
	require.NoError(t, err)

	var state struct {
		iosb     windows.IO_STATUS_BLOCK
		pollInfo AFDPollInfo
	}

	var pinner Pinner
	pinner.Pin(&state)
	defer pinner.Unpin()

	events := uint32(AFD_POLL_RECEIVE | AFD_POLL_DISCONNECT | AFD_POLL_ABORT)
	err = afd.Poll(baseHandle, events, &state.iosb, &state.pollInfo)
	require.NoError(t, err)

	entries := make([]OverlappedEntry, 1)
	var numRemoved uint32
	err = GetQueuedCompletionStatusEx(iocp, &entries[0], 1, &numRemoved, 5000, false)
	require.NoError(t, err)
	require.Equal(t, uint32(1), numRemoved)

	<-serverDone
}
