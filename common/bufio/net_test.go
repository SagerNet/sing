package bufio

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	M "github.com/metacubex/sing/common/metadata"
	"github.com/metacubex/sing/common/task"

	"github.com/stretchr/testify/assert"
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
		return serverErr
	})
	group.Append0(func(ctx context.Context) error {
		var clientErr error
		clientConn, clientErr = net.Dial("tcp", listener.Addr().String())
		return clientErr
	})
	err = group.Run(context.Background())
	require.NoError(t, err)
	listener.Close()
	t.Cleanup(func() {
		serverConn.Close()
		clientConn.Close()
	})
	return serverConn, clientConn
}

func UDPPipe(t *testing.T) (net.PacketConn, net.PacketConn, M.Socksaddr) {
	serverConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	clientConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	return serverConn, clientConn, M.SocksaddrFromNet(clientConn.LocalAddr())
}

func UDPPipe6(t *testing.T) (net.PacketConn, net.PacketConn, M.Socksaddr) {
	serverConn, err := net.ListenPacket("udp", "[::]:0")
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
			t.Error("timeout")
		}
	}()
	return cancel
}

type hashPair struct {
	sendHash map[int][]byte
	recvHash map[int][]byte
}

func newLargeDataPair() (chan hashPair, chan hashPair, func(t *testing.T) error) {
	pingCh := make(chan hashPair)
	pongCh := make(chan hashPair)
	test := func(t *testing.T) error {
		defer close(pingCh)
		defer close(pongCh)
		pingOpen := false
		pongOpen := false
		var serverPair hashPair
		var clientPair hashPair

		for {
			if pingOpen && pongOpen {
				break
			}

			select {
			case serverPair, pingOpen = <-pingCh:
				assert.True(t, pingOpen)
			case clientPair, pongOpen = <-pongCh:
				assert.True(t, pongOpen)
			case <-time.After(10 * time.Second):
				return errors.New("timeout")
			}
		}

		assert.Equal(t, serverPair.recvHash, clientPair.sendHash)
		assert.Equal(t, serverPair.sendHash, clientPair.recvHash)

		return nil
	}

	return pingCh, pongCh, test
}

func TCPTest(t *testing.T, inputConn net.Conn, outputConn net.Conn) error {
	times := 100
	chunkSize := int64(64 * 1024)

	pingCh, pongCh, test := newLargeDataPair()
	writeRandData := func(conn net.Conn) (map[int][]byte, error) {
		buf := make([]byte, chunkSize)
		hashMap := map[int][]byte{}
		for i := 0; i < times; i++ {
			if _, err := rand.Read(buf[1:]); err != nil {
				return nil, err
			}
			buf[0] = byte(i)

			hash := md5.Sum(buf)
			hashMap[i] = hash[:]

			if _, err := conn.Write(buf); err != nil {
				return nil, err
			}
		}

		return hashMap, nil
	}
	go func() {
		hashMap := map[int][]byte{}
		buf := make([]byte, chunkSize)

		for i := 0; i < times; i++ {
			_, err := io.ReadFull(outputConn, buf)
			if err != nil {
				t.Log(err.Error())
				return
			}

			hash := md5.Sum(buf)
			hashMap[int(buf[0])] = hash[:]
		}

		sendHash, err := writeRandData(outputConn)
		if err != nil {
			t.Log(err.Error())
			return
		}

		pingCh <- hashPair{
			sendHash: sendHash,
			recvHash: hashMap,
		}
	}()

	go func() {
		sendHash, err := writeRandData(inputConn)
		if err != nil {
			t.Log(err.Error())
			return
		}

		hashMap := map[int][]byte{}
		buf := make([]byte, chunkSize)

		for i := 0; i < times; i++ {
			_, err = io.ReadFull(inputConn, buf)
			if err != nil {
				t.Log(err.Error())
				return
			}

			hash := md5.Sum(buf)
			hashMap[int(buf[0])] = hash[:]
		}

		pongCh <- hashPair{
			sendHash: sendHash,
			recvHash: hashMap,
		}
	}()
	return test(t)
}

func UDPTest(t *testing.T, inputConn net.PacketConn, outputConn net.PacketConn, outputAddr M.Socksaddr) error {
	rAddr := outputAddr.UDPAddr()
	times := 50
	chunkSize := 9000
	pingCh, pongCh, test := newLargeDataPair()
	writeRandData := func(pc net.PacketConn, addr net.Addr) (map[int][]byte, error) {
		hashMap := map[int][]byte{}
		mux := sync.Mutex{}
		for i := 0; i < times; i++ {
			buf := make([]byte, chunkSize)
			if _, err := rand.Read(buf[1:]); err != nil {
				t.Log(err.Error())
				continue
			}
			buf[0] = byte(i)

			hash := md5.Sum(buf)
			mux.Lock()
			hashMap[i] = hash[:]
			mux.Unlock()

			if _, err := pc.WriteTo(buf, addr); err != nil {
				t.Log(err.Error())
			}

			time.Sleep(10 * time.Millisecond)
		}

		return hashMap, nil
	}
	go func() {
		var (
			lAddr net.Addr
			err   error
		)
		hashMap := map[int][]byte{}
		buf := make([]byte, 64*1024)

		for i := 0; i < times; i++ {
			_, lAddr, err = outputConn.ReadFrom(buf)
			if err != nil {
				t.Log(err.Error())
				return
			}
			hash := md5.Sum(buf[:chunkSize])
			hashMap[int(buf[0])] = hash[:]
		}
		sendHash, err := writeRandData(outputConn, lAddr)
		if err != nil {
			t.Log(err.Error())
			return
		}

		pingCh <- hashPair{
			sendHash: sendHash,
			recvHash: hashMap,
		}
	}()

	go func() {
		sendHash, err := writeRandData(inputConn, rAddr)
		if err != nil {
			t.Log(err.Error())
			return
		}

		hashMap := map[int][]byte{}
		buf := make([]byte, 64*1024)

		for i := 0; i < times; i++ {
			_, _, err := inputConn.ReadFrom(buf)
			if err != nil {
				t.Log(err.Error())
				return
			}

			hash := md5.Sum(buf[:chunkSize])
			hashMap[int(buf[0])] = hash[:]
		}

		pongCh <- hashPair{
			sendHash: sendHash,
			recvHash: hashMap,
		}
	}()

	return test(t)
}
