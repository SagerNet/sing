package bufio

import (
	"bytes"
	"io"
	"net"
	"os"
	"testing"
	"time"

	N "github.com/sagernet/sing/common/network"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestSplicePartialWriteCounters(t *testing.T) {
	t.Parallel()
	var connections [4]*net.UnixConn
	for i := range 2 {
		socketPair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
		require.NoError(t, err)
		for j, descriptor := range socketPair {
			file := os.NewFile(uintptr(descriptor), "splice-test")
			conn, connErr := net.FileConn(file)
			file.Close()
			require.NoError(t, connErr)
			t.Cleanup(func() { conn.Close() })
			connErr = conn.SetDeadline(time.Now().Add(5 * time.Second))
			require.NoError(t, connErr)
			connections[i*2+j] = conn.(*net.UnixConn)
		}
	}
	input, source := connections[0], connections[1]
	destination, output := connections[2], connections[3]
	payload := bytes.Repeat([]byte{0x42}, 128*1024)
	err := input.SetWriteBuffer(len(payload) * 2)
	require.NoError(t, err)
	_, err = input.Write(payload)
	require.NoError(t, err)
	err = input.CloseWrite()
	require.NoError(t, err)
	err = destination.SetWriteBuffer(1024)
	require.NoError(t, err)
	sourceRaw, err := source.SyscallConn()
	require.NoError(t, err)
	destinationRaw, err := destination.SyscallConn()
	require.NoError(t, err)
	err = destination.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
	require.NoError(t, err)
	var readBytes, writeBytes int64
	handled, n, err := splice(sourceRaw, nil, destinationRaw, nil, []N.CountFunc{
		func(n int64) { readBytes += n },
	}, []N.CountFunc{
		func(n int64) { writeBytes += n },
	})
	require.True(t, handled)
	require.Error(t, err)
	err = destination.CloseWrite()
	require.NoError(t, err)
	received, err := io.ReadAll(output)
	require.NoError(t, err)
	remaining, err := io.ReadAll(source)
	require.NoError(t, err)
	consumed := len(payload) - len(remaining)
	require.NotEmpty(t, received)
	require.Greater(t, consumed, len(received))
	require.Equal(t, payload[:len(received)], received)
	require.Equal(t, int64(consumed), readBytes)
	require.Equal(t, int64(len(received)), writeBytes)
	require.Equal(t, int64(len(received)), n)
}
