//go:build darwin || linux || windows

package bufio

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testPacketPipe struct {
	inChan    chan *N.PacketBuffer
	outChan   chan *N.PacketBuffer
	localAddr M.Socksaddr
	closed    atomic.Bool
	closeOnce sync.Once
	done      chan struct{}
}

func newTestPacketPipe(localAddr M.Socksaddr) *testPacketPipe {
	return &testPacketPipe{
		inChan:    make(chan *N.PacketBuffer, 256),
		outChan:   make(chan *N.PacketBuffer, 256),
		localAddr: localAddr,
		done:      make(chan struct{}),
	}
}

func (p *testPacketPipe) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	select {
	case packet, ok := <-p.inChan:
		if !ok {
			return M.Socksaddr{}, io.EOF
		}
		_, err = buffer.ReadOnceFrom(packet.Buffer)
		destination = packet.Destination
		packet.Buffer.Release()
		N.PutPacketBuffer(packet)
		return destination, err
	case <-p.done:
		return M.Socksaddr{}, net.ErrClosed
	}
}

func (p *testPacketPipe) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	if p.closed.Load() {
		buffer.Release()
		return net.ErrClosed
	}
	packet := N.NewPacketBuffer()
	newBuf := buf.NewSize(buffer.Len())
	newBuf.Write(buffer.Bytes())
	packet.Buffer = newBuf
	packet.Destination = destination
	buffer.Release()
	select {
	case p.outChan <- packet:
		return nil
	case <-p.done:
		packet.Buffer.Release()
		N.PutPacketBuffer(packet)
		return net.ErrClosed
	}
}

func (p *testPacketPipe) Close() error {
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		close(p.done)
	})
	return nil
}

func (p *testPacketPipe) LocalAddr() net.Addr {
	return p.localAddr.UDPAddr()
}

func (p *testPacketPipe) SetDeadline(t time.Time) error {
	return nil
}

func (p *testPacketPipe) SetReadDeadline(t time.Time) error {
	return nil
}

func (p *testPacketPipe) SetWriteDeadline(t time.Time) error {
	return nil
}

func (p *testPacketPipe) send(data []byte, destination M.Socksaddr) {
	packet := N.NewPacketBuffer()
	newBuf := buf.NewSize(len(data))
	newBuf.Write(data)
	packet.Buffer = newBuf
	packet.Destination = destination
	p.inChan <- packet
}

func (p *testPacketPipe) receive() (*N.PacketBuffer, bool) {
	select {
	case packet, ok := <-p.outChan:
		return packet, ok
	case <-p.done:
		return nil, false
	}
}

type fdPacketConn struct {
	N.NetPacketConn
	fd         int
	targetAddr M.Socksaddr
}

func newFDPacketConn(t *testing.T, conn net.PacketConn, targetAddr M.Socksaddr) *fdPacketConn {
	syscallConn, ok := conn.(syscall.Conn)
	require.True(t, ok, "connection must implement syscall.Conn")
	rawConn, err := syscallConn.SyscallConn()
	require.NoError(t, err)
	var fd int
	err = rawConn.Control(func(f uintptr) { fd = int(f) })
	require.NoError(t, err)
	return &fdPacketConn{
		NetPacketConn: NewPacketConn(conn),
		fd:            fd,
		targetAddr:    targetAddr,
	}
}

func (c *fdPacketConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	_, err = c.NetPacketConn.ReadPacket(buffer)
	if err != nil {
		return M.Socksaddr{}, err
	}
	return c.targetAddr, nil
}

func (c *fdPacketConn) FD() int {
	return c.fd
}

type channelPacketConn struct {
	N.NetPacketConn
	packetChan   chan *N.PacketBuffer
	done         chan struct{}
	closeOnce    sync.Once
	targetAddr   M.Socksaddr
	deadlineLock sync.Mutex
	deadline     time.Time
	deadlineChan chan struct{}
}

func newChannelPacketConn(conn net.PacketConn, targetAddr M.Socksaddr) *channelPacketConn {
	c := &channelPacketConn{
		NetPacketConn: NewPacketConn(conn),
		packetChan:    make(chan *N.PacketBuffer, 256),
		done:          make(chan struct{}),
		targetAddr:    targetAddr,
		deadlineChan:  make(chan struct{}),
	}
	go c.readLoop()
	return c
}

func (c *channelPacketConn) readLoop() {
	for {
		select {
		case <-c.done:
			return
		default:
		}
		buffer := buf.NewPacket()
		_, err := c.NetPacketConn.ReadPacket(buffer)
		if err != nil {
			buffer.Release()
			close(c.packetChan)
			return
		}
		packet := N.NewPacketBuffer()
		packet.Buffer = buffer
		packet.Destination = c.targetAddr
		select {
		case c.packetChan <- packet:
		case <-c.done:
			buffer.Release()
			N.PutPacketBuffer(packet)
			return
		}
	}
}

func (c *channelPacketConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	c.deadlineLock.Lock()
	deadline := c.deadline
	deadlineChan := c.deadlineChan
	c.deadlineLock.Unlock()

	var timer <-chan time.Time
	if !deadline.IsZero() {
		d := time.Until(deadline)
		if d <= 0 {
			return M.Socksaddr{}, os.ErrDeadlineExceeded
		}
		t := time.NewTimer(d)
		defer t.Stop()
		timer = t.C
	}

	select {
	case packet, ok := <-c.packetChan:
		if !ok {
			return M.Socksaddr{}, net.ErrClosed
		}
		_, err = buffer.ReadOnceFrom(packet.Buffer)
		destination = packet.Destination
		packet.Buffer.Release()
		N.PutPacketBuffer(packet)
		return
	case <-c.done:
		return M.Socksaddr{}, net.ErrClosed
	case <-deadlineChan:
		return M.Socksaddr{}, os.ErrDeadlineExceeded
	case <-timer:
		return M.Socksaddr{}, os.ErrDeadlineExceeded
	}
}

func (c *channelPacketConn) SetReadDeadline(t time.Time) error {
	c.deadlineLock.Lock()
	c.deadline = t
	if c.deadlineChan != nil {
		close(c.deadlineChan)
	}
	c.deadlineChan = make(chan struct{})
	c.deadlineLock.Unlock()
	return nil
}

func (c *channelPacketConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
	})
	return c.NetPacketConn.Close()
}

type batchHashPair struct {
	sendHash map[int][]byte
	recvHash map[int][]byte
}

func TestBatchCopy_Pipe_DataIntegrity(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 10001)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 10002)

	pipeA := newTestPacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)
	defer pipeA.Close()
	defer pipeB.Close()

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	go func() {
		copier.Copy(context.Background(), pipeA, pipeB, nil)
	}()

	time.Sleep(50 * time.Millisecond)

	const times = 50
	const chunkSize = 9000

	sendHash := make(map[int][]byte)
	recvHash := make(map[int][]byte)

	recvDone := make(chan struct{})
	go func() {
		defer close(recvDone)
		for i := 0; i < times; i++ {
			packet, ok := pipeB.receive()
			if !ok {
				t.Logf("recv channel closed at %d", i)
				return
			}
			hash := md5.Sum(packet.Buffer.Bytes())
			recvHash[int(packet.Buffer.Byte(0))] = hash[:]
			packet.Buffer.Release()
			N.PutPacketBuffer(packet)
		}
	}()

	for i := 0; i < times; i++ {
		data := make([]byte, chunkSize)
		rand.Read(data[1:])
		data[0] = byte(i)
		hash := md5.Sum(data)
		sendHash[i] = hash[:]
		pipeA.send(data, addr2)
		time.Sleep(5 * time.Millisecond)
	}

	select {
	case <-recvDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for receive")
	}

	assert.Equal(t, sendHash, recvHash, "data mismatch")
}

func TestBatchCopy_Pipe_Bidirectional(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 10001)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 10002)

	pipeA := newTestPacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)
	defer pipeA.Close()
	defer pipeB.Close()

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	go func() {
		copier.Copy(context.Background(), pipeA, pipeB, nil)
	}()

	time.Sleep(50 * time.Millisecond)

	const times = 50
	const chunkSize = 9000

	pingCh := make(chan batchHashPair, 1)
	pongCh := make(chan batchHashPair, 1)

	go func() {
		sendHash := make(map[int][]byte)
		recvHash := make(map[int][]byte)

		for i := 0; i < times; i++ {
			data := make([]byte, chunkSize)
			rand.Read(data[1:])
			data[0] = byte(i)
			hash := md5.Sum(data)
			sendHash[i] = hash[:]
			pipeA.send(data, addr2)
			time.Sleep(5 * time.Millisecond)
		}

		for i := 0; i < times; i++ {
			packet, ok := pipeA.receive()
			if !ok {
				return
			}
			hash := md5.Sum(packet.Buffer.Bytes())
			recvHash[int(packet.Buffer.Byte(0))] = hash[:]
			packet.Buffer.Release()
			N.PutPacketBuffer(packet)
		}

		pingCh <- batchHashPair{sendHash: sendHash, recvHash: recvHash}
	}()

	go func() {
		sendHash := make(map[int][]byte)
		recvHash := make(map[int][]byte)

		for i := 0; i < times; i++ {
			packet, ok := pipeB.receive()
			if !ok {
				return
			}
			hash := md5.Sum(packet.Buffer.Bytes())
			recvHash[int(packet.Buffer.Byte(0))] = hash[:]
			packet.Buffer.Release()
			N.PutPacketBuffer(packet)
		}

		for i := 0; i < times; i++ {
			data := make([]byte, chunkSize)
			rand.Read(data[1:])
			data[0] = byte(i)
			hash := md5.Sum(data)
			sendHash[i] = hash[:]
			pipeB.send(data, addr1)
			time.Sleep(5 * time.Millisecond)
		}

		pongCh <- batchHashPair{sendHash: sendHash, recvHash: recvHash}
	}()

	var aPair, bPair batchHashPair
	select {
	case aPair = <-pingCh:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for A")
	}
	select {
	case bPair = <-pongCh:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for B")
	}

	assert.Equal(t, aPair.sendHash, bPair.recvHash, "A->B mismatch")
	assert.Equal(t, bPair.sendHash, aPair.recvHash, "B->A mismatch")
}

func TestBatchCopy_FDPoller_DataIntegrity(t *testing.T) {
	t.Parallel()

	clientConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer clientConn.Close()

	proxyAConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	proxyBConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	serverConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer serverConn.Close()

	serverAddr := M.SocksaddrFromNet(serverConn.LocalAddr())
	clientAddr := M.SocksaddrFromNet(clientConn.LocalAddr())
	proxyAAddr := M.SocksaddrFromNet(proxyAConn.LocalAddr())
	proxyBAddr := M.SocksaddrFromNet(proxyBConn.LocalAddr())

	proxyA := newFDPacketConn(t, proxyAConn, serverAddr)
	proxyB := newFDPacketConn(t, proxyBConn, clientAddr)

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	go func() {
		copier.Copy(context.Background(), proxyA, proxyB, nil)
	}()

	time.Sleep(50 * time.Millisecond)

	const times = 50
	const chunkSize = 9000

	pingCh := make(chan batchHashPair, 1)
	pongCh := make(chan batchHashPair, 1)
	errCh := make(chan error, 2)

	go func() {
		sendHash := make(map[int][]byte)
		recvHash := make(map[int][]byte)
		recvBuf := make([]byte, 65536)

		for i := 0; i < times; i++ {
			data := make([]byte, chunkSize)
			rand.Read(data[1:])
			data[0] = byte(i)
			hash := md5.Sum(data)
			sendHash[i] = hash[:]
			_, err := clientConn.WriteTo(data, proxyAAddr.UDPAddr())
			if err != nil {
				errCh <- err
				return
			}
			time.Sleep(5 * time.Millisecond)
		}

		for i := 0; i < times; i++ {
			n, _, err := clientConn.ReadFrom(recvBuf)
			if err != nil {
				errCh <- err
				return
			}
			hash := md5.Sum(recvBuf[:n])
			recvHash[int(recvBuf[0])] = hash[:]
		}

		pingCh <- batchHashPair{sendHash: sendHash, recvHash: recvHash}
	}()

	go func() {
		sendHash := make(map[int][]byte)
		recvHash := make(map[int][]byte)
		recvBuf := make([]byte, 65536)

		for i := 0; i < times; i++ {
			n, _, err := serverConn.ReadFrom(recvBuf)
			if err != nil {
				errCh <- err
				return
			}
			hash := md5.Sum(recvBuf[:n])
			recvHash[int(recvBuf[0])] = hash[:]
		}

		for i := 0; i < times; i++ {
			data := make([]byte, chunkSize)
			rand.Read(data[1:])
			data[0] = byte(i)
			hash := md5.Sum(data)
			sendHash[i] = hash[:]
			_, err := serverConn.WriteTo(data, proxyBAddr.UDPAddr())
			if err != nil {
				errCh <- err
				return
			}
			time.Sleep(5 * time.Millisecond)
		}

		pongCh <- batchHashPair{sendHash: sendHash, recvHash: recvHash}
	}()

	var clientPair, serverPair batchHashPair
	for i := 0; i < 2; i++ {
		select {
		case clientPair = <-pingCh:
		case serverPair = <-pongCh:
		case err := <-errCh:
			t.Fatal(err)
		case <-time.After(15 * time.Second):
			t.Fatal("timeout")
		}
	}

	assert.Equal(t, clientPair.sendHash, serverPair.recvHash, "client->server mismatch")
	assert.Equal(t, serverPair.sendHash, clientPair.recvHash, "server->client mismatch")
}

func TestBatchCopy_LegacyChannel_DataIntegrity(t *testing.T) {
	t.Parallel()

	clientConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer clientConn.Close()

	proxyAConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	proxyBConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	serverConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer serverConn.Close()

	serverAddr := M.SocksaddrFromNet(serverConn.LocalAddr())
	clientAddr := M.SocksaddrFromNet(clientConn.LocalAddr())
	proxyAAddr := M.SocksaddrFromNet(proxyAConn.LocalAddr())
	proxyBAddr := M.SocksaddrFromNet(proxyBConn.LocalAddr())

	proxyA := newChannelPacketConn(proxyAConn, serverAddr)
	proxyB := newChannelPacketConn(proxyBConn, clientAddr)

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	go func() {
		copier.Copy(context.Background(), proxyA, proxyB, nil)
	}()

	time.Sleep(50 * time.Millisecond)

	const times = 50
	const chunkSize = 9000

	pingCh := make(chan batchHashPair, 1)
	pongCh := make(chan batchHashPair, 1)
	errCh := make(chan error, 2)

	go func() {
		sendHash := make(map[int][]byte)
		recvHash := make(map[int][]byte)
		recvBuf := make([]byte, 65536)

		for i := 0; i < times; i++ {
			data := make([]byte, chunkSize)
			rand.Read(data[1:])
			data[0] = byte(i)
			hash := md5.Sum(data)
			sendHash[i] = hash[:]
			_, err := clientConn.WriteTo(data, proxyAAddr.UDPAddr())
			if err != nil {
				errCh <- err
				return
			}
			time.Sleep(5 * time.Millisecond)
		}

		for i := 0; i < times; i++ {
			n, _, err := clientConn.ReadFrom(recvBuf)
			if err != nil {
				errCh <- err
				return
			}
			hash := md5.Sum(recvBuf[:n])
			recvHash[int(recvBuf[0])] = hash[:]
		}

		pingCh <- batchHashPair{sendHash: sendHash, recvHash: recvHash}
	}()

	go func() {
		sendHash := make(map[int][]byte)
		recvHash := make(map[int][]byte)
		recvBuf := make([]byte, 65536)

		for i := 0; i < times; i++ {
			n, _, err := serverConn.ReadFrom(recvBuf)
			if err != nil {
				errCh <- err
				return
			}
			hash := md5.Sum(recvBuf[:n])
			recvHash[int(recvBuf[0])] = hash[:]
		}

		for i := 0; i < times; i++ {
			data := make([]byte, chunkSize)
			rand.Read(data[1:])
			data[0] = byte(i)
			hash := md5.Sum(data)
			sendHash[i] = hash[:]
			_, err := serverConn.WriteTo(data, proxyBAddr.UDPAddr())
			if err != nil {
				errCh <- err
				return
			}
			time.Sleep(5 * time.Millisecond)
		}

		pongCh <- batchHashPair{sendHash: sendHash, recvHash: recvHash}
	}()

	var clientPair, serverPair batchHashPair
	for i := 0; i < 2; i++ {
		select {
		case clientPair = <-pingCh:
		case serverPair = <-pongCh:
		case err := <-errCh:
			t.Fatal(err)
		case <-time.After(15 * time.Second):
			t.Fatal("timeout")
		}
	}

	assert.Equal(t, clientPair.sendHash, serverPair.recvHash, "client->server mismatch")
	assert.Equal(t, serverPair.sendHash, clientPair.recvHash, "server->client mismatch")
}

func TestBatchCopy_MixedMode_DataIntegrity(t *testing.T) {
	t.Parallel()

	clientConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer clientConn.Close()

	proxyAConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	proxyBConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	serverConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer serverConn.Close()

	serverAddr := M.SocksaddrFromNet(serverConn.LocalAddr())
	clientAddr := M.SocksaddrFromNet(clientConn.LocalAddr())
	proxyAAddr := M.SocksaddrFromNet(proxyAConn.LocalAddr())
	proxyBAddr := M.SocksaddrFromNet(proxyBConn.LocalAddr())

	proxyA := newFDPacketConn(t, proxyAConn, serverAddr)
	proxyB := newChannelPacketConn(proxyBConn, clientAddr)

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	go func() {
		copier.Copy(context.Background(), proxyA, proxyB, nil)
	}()

	time.Sleep(50 * time.Millisecond)

	const times = 50
	const chunkSize = 9000

	pingCh := make(chan batchHashPair, 1)
	pongCh := make(chan batchHashPair, 1)
	errCh := make(chan error, 2)

	go func() {
		sendHash := make(map[int][]byte)
		recvHash := make(map[int][]byte)
		recvBuf := make([]byte, 65536)

		for i := 0; i < times; i++ {
			data := make([]byte, chunkSize)
			rand.Read(data[1:])
			data[0] = byte(i)
			hash := md5.Sum(data)
			sendHash[i] = hash[:]
			_, err := clientConn.WriteTo(data, proxyAAddr.UDPAddr())
			if err != nil {
				errCh <- err
				return
			}
			time.Sleep(5 * time.Millisecond)
		}

		for i := 0; i < times; i++ {
			n, _, err := clientConn.ReadFrom(recvBuf)
			if err != nil {
				errCh <- err
				return
			}
			hash := md5.Sum(recvBuf[:n])
			recvHash[int(recvBuf[0])] = hash[:]
		}

		pingCh <- batchHashPair{sendHash: sendHash, recvHash: recvHash}
	}()

	go func() {
		sendHash := make(map[int][]byte)
		recvHash := make(map[int][]byte)
		recvBuf := make([]byte, 65536)

		for i := 0; i < times; i++ {
			n, _, err := serverConn.ReadFrom(recvBuf)
			if err != nil {
				errCh <- err
				return
			}
			hash := md5.Sum(recvBuf[:n])
			recvHash[int(recvBuf[0])] = hash[:]
		}

		for i := 0; i < times; i++ {
			data := make([]byte, chunkSize)
			rand.Read(data[1:])
			data[0] = byte(i)
			hash := md5.Sum(data)
			sendHash[i] = hash[:]
			_, err := serverConn.WriteTo(data, proxyBAddr.UDPAddr())
			if err != nil {
				errCh <- err
				return
			}
			time.Sleep(5 * time.Millisecond)
		}

		pongCh <- batchHashPair{sendHash: sendHash, recvHash: recvHash}
	}()

	var clientPair, serverPair batchHashPair
	for i := 0; i < 2; i++ {
		select {
		case clientPair = <-pingCh:
		case serverPair = <-pongCh:
		case err := <-errCh:
			t.Fatal(err)
		case <-time.After(15 * time.Second):
			t.Fatal("timeout")
		}
	}

	assert.Equal(t, clientPair.sendHash, serverPair.recvHash, "client->server mismatch")
	assert.Equal(t, serverPair.sendHash, clientPair.recvHash, "server->client mismatch")
}

func TestBatchCopy_MultipleConnections_DataIntegrity(t *testing.T) {
	t.Parallel()

	const numConnections = 5

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	var wg sync.WaitGroup
	errCh := make(chan error, numConnections)

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			addr1 := M.ParseSocksaddrHostPort("127.0.0.1", uint16(20000+idx*2))
			addr2 := M.ParseSocksaddrHostPort("127.0.0.1", uint16(20001+idx*2))

			pipeA := newTestPacketPipe(addr1)
			pipeB := newTestPacketPipe(addr2)
			defer pipeA.Close()
			defer pipeB.Close()

			go func() {
				copier.Copy(context.Background(), pipeA, pipeB, nil)
			}()

			time.Sleep(50 * time.Millisecond)

			const times = 20
			const chunkSize = 1000

			sendHash := make(map[int][]byte)
			recvHash := make(map[int][]byte)

			recvDone := make(chan struct{})
			go func() {
				defer close(recvDone)
				for i := 0; i < times; i++ {
					packet, ok := pipeB.receive()
					if !ok {
						return
					}
					hash := md5.Sum(packet.Buffer.Bytes())
					recvHash[int(packet.Buffer.Byte(0))] = hash[:]
					packet.Buffer.Release()
					N.PutPacketBuffer(packet)
				}
			}()

			for i := 0; i < times; i++ {
				data := make([]byte, chunkSize)
				rand.Read(data[1:])
				data[0] = byte(i)
				hash := md5.Sum(data)
				sendHash[i] = hash[:]
				pipeA.send(data, addr2)
				time.Sleep(5 * time.Millisecond)
			}

			select {
			case <-recvDone:
			case <-time.After(10 * time.Second):
				errCh <- errors.New("timeout")
				return
			}

			for k, v := range sendHash {
				if rv, ok := recvHash[k]; !ok || string(v) != string(rv) {
					errCh <- errors.New("data mismatch")
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
}

func TestBatchCopy_TimeoutAndResume_DataIntegrity(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 30001)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 30002)

	pipeA := newTestPacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)
	defer pipeA.Close()
	defer pipeB.Close()

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	go func() {
		copier.Copy(context.Background(), pipeA, pipeB, nil)
	}()

	time.Sleep(50 * time.Millisecond)

	sendAndVerify := func(batchID int, count int) {
		sendHash := make(map[int][]byte)
		recvHash := make(map[int][]byte)

		recvDone := make(chan struct{})
		go func() {
			defer close(recvDone)
			for i := 0; i < count; i++ {
				packet, ok := pipeB.receive()
				if !ok {
					return
				}
				hash := md5.Sum(packet.Buffer.Bytes())
				recvHash[int(packet.Buffer.Byte(1))] = hash[:]
				packet.Buffer.Release()
				N.PutPacketBuffer(packet)
			}
		}()

		for i := 0; i < count; i++ {
			data := make([]byte, 1000)
			rand.Read(data[2:])
			data[0] = byte(batchID)
			data[1] = byte(i)
			hash := md5.Sum(data)
			sendHash[i] = hash[:]
			pipeA.send(data, addr2)
			time.Sleep(5 * time.Millisecond)
		}

		select {
		case <-recvDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("batch %d timeout", batchID)
		}

		assert.Equal(t, sendHash, recvHash, "batch %d mismatch", batchID)
	}

	sendAndVerify(1, 10)

	time.Sleep(350 * time.Millisecond)

	sendAndVerify(2, 10)
}

func TestBatchCopy_CloseWhileTransferring(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 40001)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 40002)

	pipeA := newTestPacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)

	copier := NewPacketReactor(context.Background())

	copyDone := make(chan struct{})
	go func() {
		copier.Copy(context.Background(), pipeA, pipeB, nil)
		close(copyDone)
	}()

	time.Sleep(50 * time.Millisecond)

	stopSend := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopSend:
				return
			default:
				data := make([]byte, 1000)
				rand.Read(data)
				pipeA.send(data, addr2)
				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)

	pipeA.Close()
	pipeB.Close()
	copier.Close()
	close(stopSend)

	select {
	case <-copyDone:
	case <-time.After(5 * time.Second):
		t.Fatal("copier did not close - possible deadlock")
	}
}

func TestBatchCopy_HighThroughput(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 50001)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 50002)

	pipeA := newTestPacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)
	defer pipeA.Close()
	defer pipeB.Close()

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	go func() {
		copier.Copy(context.Background(), pipeA, pipeB, nil)
	}()

	time.Sleep(50 * time.Millisecond)

	const times = 500
	const chunkSize = 8000

	sendHash := make(map[int][]byte)
	recvHash := make(map[int][]byte)
	var mu sync.Mutex

	recvDone := make(chan struct{})
	go func() {
		defer close(recvDone)
		for i := 0; i < times; i++ {
			packet, ok := pipeB.receive()
			if !ok {
				t.Logf("recv channel closed at %d", i)
				return
			}
			hash := md5.Sum(packet.Buffer.Bytes())
			idx := int(packet.Buffer.Byte(0))<<8 | int(packet.Buffer.Byte(1))
			mu.Lock()
			recvHash[idx] = hash[:]
			mu.Unlock()
			packet.Buffer.Release()
			N.PutPacketBuffer(packet)
		}
	}()

	for i := 0; i < times; i++ {
		data := make([]byte, chunkSize)
		rand.Read(data[2:])
		data[0] = byte(i >> 8)
		data[1] = byte(i & 0xff)
		hash := md5.Sum(data)
		sendHash[i] = hash[:]
		pipeA.send(data, addr2)
		time.Sleep(1 * time.Millisecond)
	}

	select {
	case <-recvDone:
	case <-time.After(30 * time.Second):
		t.Fatal("high throughput test timeout")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, len(sendHash), len(recvHash), "packet count mismatch")
	for k, v := range sendHash {
		assert.Equal(t, v, recvHash[k], "packet %d mismatch", k)
	}
}

func TestBatchCopy_LegacyFallback_DataIntegrity(t *testing.T) {
	t.Parallel()

	clientConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer clientConn.Close()

	proxyAConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	proxyBConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	serverConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer serverConn.Close()

	serverAddr := M.SocksaddrFromNet(serverConn.LocalAddr())
	clientAddr := M.SocksaddrFromNet(clientConn.LocalAddr())
	proxyAAddr := M.SocksaddrFromNet(proxyAConn.LocalAddr())
	proxyBAddr := M.SocksaddrFromNet(proxyBConn.LocalAddr())

	proxyA := &legacyPacketConn{NetPacketConn: NewPacketConn(proxyAConn), targetAddr: serverAddr}
	proxyB := &legacyPacketConn{NetPacketConn: NewPacketConn(proxyBConn), targetAddr: clientAddr}

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	go func() {
		copier.Copy(context.Background(), proxyA, proxyB, nil)
	}()

	time.Sleep(50 * time.Millisecond)

	const times = 50
	const chunkSize = 9000

	pingCh := make(chan batchHashPair, 1)
	pongCh := make(chan batchHashPair, 1)
	errCh := make(chan error, 2)

	go func() {
		sendHash := make(map[int][]byte)
		recvHash := make(map[int][]byte)
		recvBuf := make([]byte, 65536)

		for i := 0; i < times; i++ {
			data := make([]byte, chunkSize)
			rand.Read(data[1:])
			data[0] = byte(i)
			hash := md5.Sum(data)
			sendHash[i] = hash[:]
			_, err := clientConn.WriteTo(data, proxyAAddr.UDPAddr())
			if err != nil {
				errCh <- err
				return
			}
			time.Sleep(5 * time.Millisecond)
		}

		clientConn.SetReadDeadline(time.Now().Add(10 * time.Second))
		for i := 0; i < times; i++ {
			n, _, err := clientConn.ReadFrom(recvBuf)
			if err != nil {
				if os.IsTimeout(err) {
					t.Logf("client read timeout after %d packets", i)
				}
				errCh <- err
				return
			}
			hash := md5.Sum(recvBuf[:n])
			recvHash[int(recvBuf[0])] = hash[:]
		}

		pingCh <- batchHashPair{sendHash: sendHash, recvHash: recvHash}
	}()

	go func() {
		sendHash := make(map[int][]byte)
		recvHash := make(map[int][]byte)
		recvBuf := make([]byte, 65536)

		serverConn.SetReadDeadline(time.Now().Add(10 * time.Second))
		for i := 0; i < times; i++ {
			n, _, err := serverConn.ReadFrom(recvBuf)
			if err != nil {
				if os.IsTimeout(err) {
					t.Logf("server read timeout after %d packets", i)
				}
				errCh <- err
				return
			}
			hash := md5.Sum(recvBuf[:n])
			recvHash[int(recvBuf[0])] = hash[:]
		}

		for i := 0; i < times; i++ {
			data := make([]byte, chunkSize)
			rand.Read(data[1:])
			data[0] = byte(i)
			hash := md5.Sum(data)
			sendHash[i] = hash[:]
			_, err := serverConn.WriteTo(data, proxyBAddr.UDPAddr())
			if err != nil {
				errCh <- err
				return
			}
			time.Sleep(5 * time.Millisecond)
		}

		pongCh <- batchHashPair{sendHash: sendHash, recvHash: recvHash}
	}()

	var clientPair, serverPair batchHashPair
	for i := 0; i < 2; i++ {
		select {
		case clientPair = <-pingCh:
		case serverPair = <-pongCh:
		case err := <-errCh:
			t.Fatal(err)
		case <-time.After(20 * time.Second):
			t.Fatal("timeout")
		}
	}

	assert.Equal(t, clientPair.sendHash, serverPair.recvHash, "client->server mismatch")
	assert.Equal(t, serverPair.sendHash, clientPair.recvHash, "server->client mismatch")
}

func TestBatchCopy_ReactorClose(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 60001)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 60002)

	pipeA := newTestPacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)
	defer pipeA.Close()
	defer pipeB.Close()

	copier := NewPacketReactor(context.Background())

	copyDone := make(chan struct{})
	go func() {
		copier.Copy(context.Background(), pipeA, pipeB, nil)
		close(copyDone)
	}()

	time.Sleep(50 * time.Millisecond)

	go func() {
		for {
			select {
			case <-copyDone:
				return
			default:
				data := make([]byte, 100)
				rand.Read(data)
				pipeA.send(data, addr2)
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)

	pipeA.Close()
	pipeB.Close()
	copier.Close()

	select {
	case <-copyDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Copy did not return after reactor close")
	}
}

func TestBatchCopy_SmallPackets(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 60011)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 60012)

	pipeA := newTestPacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)
	defer pipeA.Close()
	defer pipeB.Close()

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	go func() {
		copier.Copy(context.Background(), pipeA, pipeB, nil)
	}()

	time.Sleep(50 * time.Millisecond)

	const totalPackets = 20
	receivedCount := 0

	recvDone := make(chan struct{})
	go func() {
		defer close(recvDone)
		for i := 0; i < totalPackets; i++ {
			packet, ok := pipeB.receive()
			if !ok {
				return
			}
			receivedCount++
			packet.Buffer.Release()
			N.PutPacketBuffer(packet)
		}
	}()

	for i := 0; i < totalPackets; i++ {
		size := (i % 10) + 1
		data := make([]byte, size)
		rand.Read(data)
		pipeA.send(data, addr2)
		time.Sleep(5 * time.Millisecond)
	}

	select {
	case <-recvDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for packets")
	}

	assert.Equal(t, totalPackets, receivedCount)
}

func TestBatchCopy_VaryingPacketSizes(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 60041)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 60042)

	pipeA := newTestPacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)
	defer pipeA.Close()
	defer pipeB.Close()

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	go func() {
		copier.Copy(context.Background(), pipeA, pipeB, nil)
	}()

	time.Sleep(50 * time.Millisecond)

	sizes := []int{10, 100, 500, 1000, 2000, 4000, 8000}
	const times = 3

	totalPackets := len(sizes) * times
	sendHash := make(map[int][]byte)
	recvHash := make(map[int][]byte)

	recvDone := make(chan struct{})
	go func() {
		defer close(recvDone)
		for i := 0; i < totalPackets; i++ {
			packet, ok := pipeB.receive()
			if !ok {
				return
			}
			idx := int(packet.Buffer.Byte(0))<<8 | int(packet.Buffer.Byte(1))
			hash := md5.Sum(packet.Buffer.Bytes())
			recvHash[idx] = hash[:]
			packet.Buffer.Release()
			N.PutPacketBuffer(packet)
		}
	}()

	packetIdx := 0
	for _, size := range sizes {
		for j := 0; j < times; j++ {
			data := make([]byte, size)
			rand.Read(data[2:])
			data[0] = byte(packetIdx >> 8)
			data[1] = byte(packetIdx & 0xff)
			hash := md5.Sum(data)
			sendHash[packetIdx] = hash[:]
			pipeA.send(data, addr2)
			packetIdx++
			time.Sleep(5 * time.Millisecond)
		}
	}

	select {
	case <-recvDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout")
	}

	assert.Equal(t, len(sendHash), len(recvHash))
	for k, v := range sendHash {
		assert.Equal(t, v, recvHash[k], "packet %d mismatch", k)
	}
}

func TestBatchCopy_OnCloseCallback(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 60021)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 60022)

	pipeA := newTestPacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	callbackCalled := make(chan error, 1)
	onClose := func(err error) {
		select {
		case callbackCalled <- err:
		default:
		}
	}

	go func() {
		copier.Copy(context.Background(), pipeA, pipeB, onClose)
	}()

	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 5; i++ {
		data := make([]byte, 100)
		rand.Read(data)
		pipeA.send(data, addr2)
	}

	time.Sleep(50 * time.Millisecond)

	pipeA.Close()
	pipeB.Close()

	select {
	case <-callbackCalled:
	case <-time.After(5 * time.Second):
		t.Fatal("onClose callback was not called")
	}
}

func TestBatchCopy_SourceClose(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 60031)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 60032)

	pipeA := newTestPacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	var capturedErr error
	var errMu sync.Mutex
	callbackCalled := make(chan struct{})
	onClose := func(err error) {
		errMu.Lock()
		capturedErr = err
		errMu.Unlock()
		close(callbackCalled)
	}

	copyDone := make(chan struct{})
	go func() {
		copier.Copy(context.Background(), pipeA, pipeB, onClose)
		close(copyDone)
	}()

	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 5; i++ {
		data := make([]byte, 100)
		rand.Read(data)
		pipeA.send(data, addr2)
	}

	time.Sleep(50 * time.Millisecond)

	pipeA.Close()
	close(pipeA.inChan)

	select {
	case <-callbackCalled:
	case <-time.After(5 * time.Second):
		pipeB.Close()
		t.Fatal("onClose callback was not called after source close")
	}

	select {
	case <-copyDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Copy did not return after source close")
	}

	pipeB.Close()

	errMu.Lock()
	err := capturedErr
	errMu.Unlock()

	require.NotNil(t, err)
}

type legacyPacketConn struct {
	N.NetPacketConn
	targetAddr M.Socksaddr
}

func (c *legacyPacketConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	_, err = c.NetPacketConn.ReadPacket(buffer)
	if err != nil {
		return M.Socksaddr{}, err
	}
	return c.targetAddr, nil
}

// pushablePacketPipe implements PacketPushable for testing with deadline support
type pushablePacketPipe struct {
	inChan       chan *N.PacketBuffer
	outChan      chan *N.PacketBuffer
	localAddr    M.Socksaddr
	closed       atomic.Bool
	closeOnce    sync.Once
	done         chan struct{}
	onDataReady  func()
	mutex        sync.Mutex
	deadlineLock sync.Mutex
	deadline     time.Time
	deadlineChan chan struct{}
}

func newPushablePacketPipe(localAddr M.Socksaddr) *pushablePacketPipe {
	return &pushablePacketPipe{
		inChan:       make(chan *N.PacketBuffer, 256),
		outChan:      make(chan *N.PacketBuffer, 256),
		localAddr:    localAddr,
		done:         make(chan struct{}),
		deadlineChan: make(chan struct{}),
	}
}

func (p *pushablePacketPipe) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	p.deadlineLock.Lock()
	deadline := p.deadline
	deadlineChan := p.deadlineChan
	p.deadlineLock.Unlock()

	var timer <-chan time.Time
	if !deadline.IsZero() {
		d := time.Until(deadline)
		if d <= 0 {
			return M.Socksaddr{}, os.ErrDeadlineExceeded
		}
		t := time.NewTimer(d)
		defer t.Stop()
		timer = t.C
	}

	select {
	case packet, ok := <-p.inChan:
		if !ok {
			return M.Socksaddr{}, io.EOF
		}
		_, err = buffer.ReadOnceFrom(packet.Buffer)
		destination = packet.Destination
		packet.Buffer.Release()
		N.PutPacketBuffer(packet)
		return destination, err
	case <-p.done:
		return M.Socksaddr{}, net.ErrClosed
	case <-deadlineChan:
		return M.Socksaddr{}, os.ErrDeadlineExceeded
	case <-timer:
		return M.Socksaddr{}, os.ErrDeadlineExceeded
	}
}

func (p *pushablePacketPipe) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	if p.closed.Load() {
		buffer.Release()
		return net.ErrClosed
	}
	packet := N.NewPacketBuffer()
	newBuf := buf.NewSize(buffer.Len())
	newBuf.Write(buffer.Bytes())
	packet.Buffer = newBuf
	packet.Destination = destination
	buffer.Release()
	select {
	case p.outChan <- packet:
		return nil
	case <-p.done:
		packet.Buffer.Release()
		N.PutPacketBuffer(packet)
		return net.ErrClosed
	}
}

func (p *pushablePacketPipe) Close() error {
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		close(p.done)
	})
	return nil
}

func (p *pushablePacketPipe) LocalAddr() net.Addr {
	return p.localAddr.UDPAddr()
}

func (p *pushablePacketPipe) SetDeadline(t time.Time) error {
	return p.SetReadDeadline(t)
}

func (p *pushablePacketPipe) SetReadDeadline(t time.Time) error {
	p.deadlineLock.Lock()
	p.deadline = t
	if p.deadlineChan != nil {
		close(p.deadlineChan)
	}
	p.deadlineChan = make(chan struct{})
	p.deadlineLock.Unlock()
	return nil
}

func (p *pushablePacketPipe) SetWriteDeadline(t time.Time) error {
	return nil
}

func (p *pushablePacketPipe) SetOnDataReady(callback func()) {
	p.mutex.Lock()
	p.onDataReady = callback
	p.mutex.Unlock()
}

func (p *pushablePacketPipe) HasPendingData() bool {
	return len(p.inChan) > 0
}

func (p *pushablePacketPipe) triggerDataReady() {
	p.mutex.Lock()
	callback := p.onDataReady
	p.mutex.Unlock()
	if callback != nil {
		callback()
	}
}

func (p *pushablePacketPipe) send(data []byte, destination M.Socksaddr) {
	packet := N.NewPacketBuffer()
	newBuf := buf.NewSize(len(data))
	newBuf.Write(data)
	packet.Buffer = newBuf
	packet.Destination = destination
	p.inChan <- packet
}

func (p *pushablePacketPipe) sendWithNotify(data []byte, destination M.Socksaddr) {
	p.send(data, destination)
	p.triggerDataReady()
}

func (p *pushablePacketPipe) receive() (*N.PacketBuffer, bool) {
	select {
	case packet, ok := <-p.outChan:
		return packet, ok
	case <-p.done:
		return nil, false
	}
}

// failingPacketWriter fails after N writes
type failingPacketWriter struct {
	N.PacketConn
	failAfter  int
	writeCount atomic.Int32
}

func (w *failingPacketWriter) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	if w.writeCount.Add(1) > int32(w.failAfter) {
		buffer.Release()
		return errors.New("simulated packet write error")
	}
	return w.PacketConn.WritePacket(buffer, destination)
}

// errorPacketReader returns error on ReadPacket
type errorPacketReader struct {
	N.PacketConn
	readError error
}

func (r *errorPacketReader) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	return M.Socksaddr{}, r.readError
}

func TestPacketReactor_Pushable_Basic(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 61001)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 61002)

	pipeA := newPushablePacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)
	defer pipeA.Close()
	defer pipeB.Close()

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	copyDone := make(chan struct{})
	go func() {
		copier.Copy(context.Background(), pipeA, pipeB, nil)
		close(copyDone)
	}()

	time.Sleep(50 * time.Millisecond)

	const times = 20
	const chunkSize = 1000

	sendHash := make(map[int][]byte)
	recvHash := make(map[int][]byte)

	recvDone := make(chan struct{})
	go func() {
		defer close(recvDone)
		for i := 0; i < times; i++ {
			packet, ok := pipeB.receive()
			if !ok {
				return
			}
			hash := md5.Sum(packet.Buffer.Bytes())
			recvHash[int(packet.Buffer.Byte(0))] = hash[:]
			packet.Buffer.Release()
			N.PutPacketBuffer(packet)
		}
	}()

	for i := 0; i < times; i++ {
		data := make([]byte, chunkSize)
		rand.Read(data[1:])
		data[0] = byte(i)
		hash := md5.Sum(data)
		sendHash[i] = hash[:]
		pipeA.sendWithNotify(data, addr2)
		time.Sleep(5 * time.Millisecond)
	}

	select {
	case <-recvDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for receive")
	}

	assert.Equal(t, sendHash, recvHash, "data mismatch")

	pipeA.Close()
	pipeB.Close()

	select {
	case <-copyDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for copy to complete")
	}
}

func TestPacketReactor_Pushable_HasPendingData(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 61011)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 61012)

	pipeA := newPushablePacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)
	defer pipeA.Close()
	defer pipeB.Close()

	// Pre-load data before starting the reactor
	const preloadCount = 5
	preloadHashes := make([][]byte, preloadCount)
	for i := 0; i < preloadCount; i++ {
		data := make([]byte, 100)
		rand.Read(data[1:])
		data[0] = byte(i)
		hash := md5.Sum(data)
		preloadHashes[i] = hash[:]
		pipeA.send(data, addr2)
	}

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	copyDone := make(chan struct{})
	go func() {
		copier.Copy(context.Background(), pipeA, pipeB, nil)
		close(copyDone)
	}()

	// Wait for the reactor to detect pending data and process it
	receivedHashes := make([][]byte, 0, preloadCount)
	timeout := time.After(5 * time.Second)

	for i := 0; i < preloadCount; i++ {
		select {
		case packet := <-pipeB.outChan:
			hash := md5.Sum(packet.Buffer.Bytes())
			receivedHashes = append(receivedHashes, hash[:])
			packet.Buffer.Release()
			N.PutPacketBuffer(packet)
		case <-timeout:
			t.Fatalf("timeout: only received %d/%d packets", len(receivedHashes), preloadCount)
		}
	}

	// Verify all preloaded packets were received
	assert.Equal(t, len(preloadHashes), len(receivedHashes), "should receive all preloaded packets")

	pipeA.Close()
	pipeB.Close()

	select {
	case <-copyDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for copy to complete")
	}
}

func TestPacketReactor_Pushable_TimeoutResume(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 61021)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 61022)

	pipeA := newPushablePacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)
	defer pipeA.Close()
	defer pipeB.Close()

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	copyDone := make(chan struct{})
	go func() {
		copier.Copy(context.Background(), pipeA, pipeB, nil)
		close(copyDone)
	}()

	time.Sleep(50 * time.Millisecond)

	// Send first batch
	const batchSize = 5
	for i := 0; i < batchSize; i++ {
		data := make([]byte, 100)
		rand.Read(data)
		pipeA.sendWithNotify(data, addr2)
	}

	// Receive first batch
	for i := 0; i < batchSize; i++ {
		select {
		case packet := <-pipeB.outChan:
			packet.Buffer.Release()
			N.PutPacketBuffer(packet)
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout receiving first batch packet %d", i)
		}
	}

	// Wait longer than the batch timeout (250ms) to trigger return to idle
	time.Sleep(400 * time.Millisecond)

	// Send second batch - should trigger data ready callback
	for i := 0; i < batchSize; i++ {
		data := make([]byte, 100)
		rand.Read(data)
		pipeA.sendWithNotify(data, addr2)
	}

	// Receive second batch
	for i := 0; i < batchSize; i++ {
		select {
		case packet := <-pipeB.outChan:
			packet.Buffer.Release()
			N.PutPacketBuffer(packet)
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout receiving second batch packet %d", i)
		}
	}

	pipeA.Close()
	pipeB.Close()

	select {
	case <-copyDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for copy to complete")
	}
}

func TestPacketReactor_WriteError(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 61031)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 61032)

	pipeA := newTestPacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)
	defer pipeA.Close()
	defer pipeB.Close()

	// Wrap destination with failing writer that fails after 3 packets
	failingDest := &failingPacketWriter{PacketConn: pipeB, failAfter: 3}

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	var capturedErr error
	var errMu sync.Mutex
	closeDone := make(chan struct{})

	go func() {
		copier.Copy(context.Background(), pipeA, failingDest, func(err error) {
			errMu.Lock()
			capturedErr = err
			errMu.Unlock()
			close(closeDone)
		})
	}()

	time.Sleep(50 * time.Millisecond)

	// Send packets until failure
	for i := 0; i < 10; i++ {
		data := make([]byte, 100)
		rand.Read(data)
		pipeA.send(data, addr2)
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for close callback
	select {
	case <-closeDone:
		errMu.Lock()
		err := capturedErr
		errMu.Unlock()
		assert.NotNil(t, err, "expected error to be propagated")
		assert.Contains(t, err.Error(), "simulated packet write error")
	case <-time.After(5 * time.Second):
		pipeA.Close()
		pipeB.Close()
		t.Fatal("timeout waiting for close callback")
	}
}

func TestPacketReactor_ReadError(t *testing.T) {
	t.Parallel()

	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 61042)

	pipeA := newTestPacketPipe(M.ParseSocksaddrHostPort("127.0.0.1", 61041))
	pipeB := newTestPacketPipe(addr2)
	defer pipeA.Close()
	defer pipeB.Close()

	// Wrap source with error-returning reader
	readErr := errors.New("simulated packet read error")
	errorSrc := &errorPacketReader{PacketConn: pipeA, readError: readErr}

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	var capturedErr error
	var errMu sync.Mutex
	closeDone := make(chan struct{})

	go func() {
		copier.Copy(context.Background(), errorSrc, pipeB, func(err error) {
			errMu.Lock()
			capturedErr = err
			errMu.Unlock()
			close(closeDone)
		})
	}()

	select {
	case <-closeDone:
		errMu.Lock()
		err := capturedErr
		errMu.Unlock()
		assert.NotNil(t, err, "expected error to be propagated")
		assert.Contains(t, err.Error(), "simulated packet read error")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for close callback")
	}
}

func TestPacketReactor_StateMachine_ConcurrentWakeup(t *testing.T) {
	t.Parallel()

	clientConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer clientConn.Close()

	proxyAConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	proxyBConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	serverConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer serverConn.Close()

	serverAddr := M.SocksaddrFromNet(serverConn.LocalAddr())
	clientAddr := M.SocksaddrFromNet(clientConn.LocalAddr())
	proxyAAddr := M.SocksaddrFromNet(proxyAConn.LocalAddr())

	proxyA := newFDPacketConn(t, proxyAConn, serverAddr)
	proxyB := newFDPacketConn(t, proxyBConn, clientAddr)

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	copyDone := make(chan struct{})
	go func() {
		copier.Copy(context.Background(), proxyA, proxyB, nil)
		close(copyDone)
	}()

	time.Sleep(50 * time.Millisecond)

	// Send many packets rapidly to stress the state machine
	const numPackets = 100
	var wg sync.WaitGroup

	// Multiple goroutines sending packets concurrently
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < numPackets/5; i++ {
				data := make([]byte, 100)
				rand.Read(data)
				clientConn.WriteTo(data, proxyAAddr.UDPAddr())
			}
		}(g)
	}

	wg.Wait()

	// Give time for packets to be processed
	time.Sleep(500 * time.Millisecond)

	// Cleanup
	proxyAConn.Close()
	proxyBConn.Close()

	select {
	case <-copyDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for copy to complete")
	}
}

func TestPacketReactor_StateMachine_CloseWhileActive(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 61051)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 61052)

	pipeA := newTestPacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)

	copier := NewPacketReactor(context.Background())

	copyDone := make(chan struct{})
	go func() {
		copier.Copy(context.Background(), pipeA, pipeB, nil)
		close(copyDone)
	}()

	time.Sleep(50 * time.Millisecond)

	// Start sending packets continuously
	stopSend := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopSend:
				return
			default:
				data := make([]byte, 100)
				rand.Read(data)
				pipeA.send(data, addr2)
				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	// Give time for active processing
	time.Sleep(100 * time.Millisecond)

	// Close while actively processing
	pipeA.Close()
	pipeB.Close()
	copier.Close()
	close(stopSend)

	// Verify no deadlock
	select {
	case <-copyDone:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock detected: Copy did not return after close")
	}
}

func TestPacketReactor_Counters(t *testing.T) {
	t.Parallel()

	addr1 := M.ParseSocksaddrHostPort("127.0.0.1", 61061)
	addr2 := M.ParseSocksaddrHostPort("127.0.0.1", 61062)

	pipeA := newTestPacketPipe(addr1)
	pipeB := newTestPacketPipe(addr2)
	defer pipeA.Close()
	defer pipeB.Close()

	copier := NewPacketReactor(context.Background())
	defer copier.Close()

	copyDone := make(chan struct{})
	go func() {
		copier.Copy(context.Background(), pipeA, pipeB, nil)
		close(copyDone)
	}()

	time.Sleep(50 * time.Millisecond)

	// Send packets with known sizes
	const numPackets = 10
	packetSizes := []int{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000}
	totalBytes := 0
	for _, size := range packetSizes {
		totalBytes += size
	}

	recvDone := make(chan struct{})
	receivedBytes := 0
	go func() {
		defer close(recvDone)
		for i := 0; i < numPackets; i++ {
			packet, ok := pipeB.receive()
			if !ok {
				return
			}
			receivedBytes += packet.Buffer.Len()
			packet.Buffer.Release()
			N.PutPacketBuffer(packet)
		}
	}()

	for i := 0; i < numPackets; i++ {
		data := make([]byte, packetSizes[i])
		rand.Read(data)
		pipeA.send(data, addr2)
		time.Sleep(5 * time.Millisecond)
	}

	select {
	case <-recvDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for receive")
	}

	assert.Equal(t, totalBytes, receivedBytes, "total bytes received should match sent")

	pipeA.Close()
	pipeB.Close()

	select {
	case <-copyDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for copy to complete")
	}
}
