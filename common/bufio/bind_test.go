package bufio

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/sagernet/sing/common/buf"
	N "github.com/sagernet/sing/common/network"

	"github.com/stretchr/testify/require"
)

func TestServerPacketConnConcurrentReadWrite(t *testing.T) {
	t.Parallel()
	for _, readWaiter := range []bool{false, true} {
		name := "ReadBuffer"
		if readWaiter {
			name = "ReadWaiter"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			serverConn, err := net.ListenPacket("udp", "127.0.0.1:0")
			require.NoError(t, err)
			defer serverConn.Close()
			clientConn, err := net.ListenPacket("udp", "127.0.0.1:0")
			require.NoError(t, err)
			defer clientConn.Close()

			conn := NewServerPacketConn(serverConn)
			read := func() error {
				buffer := buf.NewPacket()
				defer buffer.Release()
				return conn.ReadBuffer(buffer)
			}
			if readWaiter {
				waiter, created := CreateReadWaiter(conn)
				if !created {
					t.Skip("no read waiter for this platform")
				}
				waiter.InitializeReadWaiter(N.ReadWaitOptions{})
				read = func() error {
					buffer, err := waiter.WaitReadBuffer()
					if err == nil {
						buffer.Release()
					}
					return err
				}
			}

			const packets = 200
			require.NoError(t, serverConn.SetReadDeadline(time.Now().Add(10*time.Second)))
			done := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(3)
			go func() {
				defer wg.Done()
				defer close(done)
				for range packets {
					if err := read(); err != nil {
						t.Error(err)
						return
					}
				}
			}()
			go func() {
				defer wg.Done()
				for {
					select {
					case <-done:
						return
					default:
					}
					_, _ = clientConn.WriteTo([]byte("ping"), serverConn.LocalAddr())
					time.Sleep(time.Millisecond)
				}
			}()
			go func() {
				defer wg.Done()
				for {
					select {
					case <-done:
						return
					default:
					}
					_ = conn.RemoteAddr()
					_ = conn.WriteBuffer(buf.As([]byte("pong")))
				}
			}()
			wg.Wait()
		})
	}
}
