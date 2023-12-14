package bufio

import (
	"context"
	"net"
	"testing"
	"time"

	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/task"

	"github.com/stretchr/testify/require"
)

func TCPPipe(t *testing.T) (net.Conn, net.Conn) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	var (
		group      task.Group
		serverConn net.Conn
		clientConn net.Conn
	)
	group.Append0(func(ctx context.Context) error {
		var serverErr error
		serverConn, serverErr = listener.Accept()
		require.NoError(t, serverErr)
		return nil
	})
	group.Append0(func(ctx context.Context) error {
		var clientErr error
		clientConn, clientErr = net.Dial("tcp", listener.Addr().String())
		require.NoError(t, clientErr)
		return nil
	})
	err = group.Run()
	require.NoError(t, err)
	listener.Close()
	return serverConn, clientConn
}

func UDPPipe(t *testing.T) (net.PacketConn, net.PacketConn, M.Socksaddr) {
	serverConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	clientConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	return serverConn, clientConn, M.SocksaddrFromNet(clientConn.LocalAddr())
}

func Timeout(t *testing.T) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			t.Fatal("timeout")
		}
	}()
	return cancel
}
