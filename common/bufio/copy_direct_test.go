package bufio

import (
	"net"
	"testing"

	"github.com/sagernet/sing/common/buf"
	N "github.com/sagernet/sing/common/network"

	"github.com/stretchr/testify/require"
)

func TestCopyWaitTCP(t *testing.T) {
	t.Parallel()
	inputConn, outputConn := TCPPipe(t)
	readWaiter, created := CreateReadWaiter(outputConn)
	require.True(t, created)
	require.NotNil(t, readWaiter)
	readWaiter.InitializeReadWaiter(N.ReadWaitOptions{})
	require.NoError(t, TCPTest(t, inputConn, &readWaitWrapper{
		Conn:       outputConn,
		readWaiter: readWaiter,
	}))
}

type readWaitWrapper struct {
	net.Conn
	readWaiter N.ReadWaiter
	buffer     *buf.Buffer
}

func (r *readWaitWrapper) Read(p []byte) (n int, err error) {
	if r.buffer != nil {
		if r.buffer.Len() > 0 {
			return r.buffer.Read(p)
		}
		if r.buffer.IsEmpty() {
			r.buffer.Release()
			r.buffer = nil
		}
	}
	buffer, err := r.readWaiter.WaitReadBuffer()
	if err != nil {
		return
	}
	r.buffer = buffer
	return r.buffer.Read(p)
}

func TestCopyWaitUDP(t *testing.T) {
	t.Parallel()
	inputConn, outputConn, outputAddr := UDPPipe(t)
	readWaiter, created := CreatePacketReadWaiter(NewPacketConn(outputConn))
	require.True(t, created)
	require.NotNil(t, readWaiter)
	readWaiter.InitializeReadWaiter(N.ReadWaitOptions{})
	require.NoError(t, UDPTest(t, inputConn, &packetReadWaitWrapper{
		PacketConn: outputConn,
		readWaiter: readWaiter,
	}, outputAddr))
}

type packetReadWaitWrapper struct {
	net.PacketConn
	readWaiter N.PacketReadWaiter
}

func (r *packetReadWaitWrapper) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	buffer, destination, err := r.readWaiter.WaitReadPacket()
	if err != nil {
		return
	}
	n = copy(p, buffer.Bytes())
	buffer.Release()
	addr = destination.UDPAddr()
	return
}

func TestCopyVectorisedWaitTCP(t *testing.T) {
	t.Parallel()
	inputConn, outputConn := TCPPipe(t)
	readWaiter, created := CreateVectorisedReadWaiter(outputConn)
	require.True(t, created)
	require.NotNil(t, readWaiter)
	readWaiter.InitializeReadWaiter(N.ReadWaitOptions{BatchSize: DefaultBatchSize})
	require.NoError(t, TCPTest(t, inputConn, &vectorisedReadWaitWrapper{
		Conn:       outputConn,
		readWaiter: readWaiter,
	}))
}

type vectorisedReadWaitWrapper struct {
	net.Conn
	readWaiter N.VectorisedReadWaiter
	buffers    []*buf.Buffer
}

func (r *vectorisedReadWaitWrapper) Read(p []byte) (n int, err error) {
	for len(r.buffers) > 0 {
		if r.buffers[0].Len() > 0 {
			return r.buffers[0].Read(p)
		} else {
			r.buffers[0].Release()
			r.buffers = r.buffers[1:]
		}
	}
	buffers, err := r.readWaiter.WaitReadBuffers()
	if err != nil {
		return
	}
	r.buffers = buffers
	return r.buffers[0].Read(p)
}
