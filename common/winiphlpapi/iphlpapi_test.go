//go:build windows

package winiphlpapi_test

import (
	"context"
	"net"
	"syscall"
	"testing"

	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/winiphlpapi"

	"github.com/stretchr/testify/require"
)

func TestFindPidTcp4(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	go listener.Accept()
	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()
	pid, err := winiphlpapi.FindPid(N.NetworkTCP, M.AddrPortFromNet(conn.LocalAddr()))
	require.NoError(t, err)
	require.Equal(t, uint32(syscall.Getpid()), pid)
}

func TestFindPidTcp6(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "[::1]:0")
	require.NoError(t, err)
	defer listener.Close()
	go listener.Accept()
	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()
	pid, err := winiphlpapi.FindPid(N.NetworkTCP, M.AddrPortFromNet(conn.LocalAddr()))
	require.NoError(t, err)
	require.Equal(t, uint32(syscall.Getpid()), pid)
}

func TestFindPidUdp4(t *testing.T) {
	t.Parallel()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer conn.Close()
	pid, err := winiphlpapi.FindPid(N.NetworkUDP, M.AddrPortFromNet(conn.LocalAddr()))
	require.NoError(t, err)
	require.Equal(t, uint32(syscall.Getpid()), pid)
}

func TestFindPidUdp6(t *testing.T) {
	t.Parallel()
	conn, err := net.ListenPacket("udp", "[::1]:0")
	require.NoError(t, err)
	defer conn.Close()
	pid, err := winiphlpapi.FindPid(N.NetworkUDP, M.AddrPortFromNet(conn.LocalAddr()))
	require.NoError(t, err)
	require.Equal(t, uint32(syscall.Getpid()), pid)
}

func TestWaitAck4(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	go listener.Accept()
	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()
	err = winiphlpapi.WriteAndWaitAck(context.Background(), conn, []byte("hello"))
	require.NoError(t, err)
}

func TestWaitAck6(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "[::1]:0")
	require.NoError(t, err)
	defer listener.Close()
	go listener.Accept()
	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()
	err = winiphlpapi.WriteAndWaitAck(context.Background(), conn, []byte("hello"))
	require.NoError(t, err)
}
