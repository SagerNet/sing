package bufio

import (
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"github.com/stretchr/testify/require"
)

func TestCreatePacketVectorisedReadWaiterDeprecated(t *testing.T) {
	t.Parallel()
	reader := &testPacketBatchReader{}
	batchReader, created := CreatePacketBatchReadWaiter(reader)
	require.True(t, created)
	require.NotNil(t, batchReader)
	vectorisedReader, created := CreatePacketVectorisedReadWaiter(reader)
	require.True(t, created)
	require.Same(t, batchReader, vectorisedReader)
}

func TestCopyPacketUsesBatchPath(t *testing.T) {
	t.Parallel()
	destinationA := M.SocksaddrFrom(netip.MustParseAddr("127.0.0.1"), 1000)
	destinationB := M.SocksaddrFrom(netip.MustParseAddr("127.0.0.1"), 1001)
	reader := &testPacketBatchReader{
		batches: []testPacketBatch{{
			payloads:     [][]byte{[]byte("a"), []byte("bc")},
			destinations: []M.Socksaddr{destinationA, destinationB},
		}},
	}
	writer := &testPacketBatchWriter{}
	var readBytes, readPackets, writeBytes, writePackets atomic.Int64
	n, err := CopyPacketWithCounters(writer, reader, reader, []N.CountFunc{
		func(n int64) { readBytes.Add(n) },
		func(n int64) { readPackets.Add(1) },
	}, []N.CountFunc{
		func(n int64) { writeBytes.Add(n) },
		func(n int64) { writePackets.Add(1) },
	})
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, int64(3), n)
	require.True(t, writer.usedBatch)
	require.Equal(t, [][]byte{[]byte("a"), []byte("bc")}, writer.payloads)
	require.Equal(t, []M.Socksaddr{destinationA, destinationB}, writer.destinations)
	require.Equal(t, int64(3), readBytes.Load())
	require.Equal(t, int64(2), readPackets.Load())
	require.Equal(t, int64(3), writeBytes.Load())
	require.Equal(t, int64(2), writePackets.Load())
}

func TestCopyPacketUsesConnectedBatchWriter(t *testing.T) {
	t.Parallel()
	destinationA := M.SocksaddrFrom(netip.MustParseAddr("127.0.0.1"), 1000)
	destinationB := M.SocksaddrFrom(netip.MustParseAddr("127.0.0.1"), 1001)
	reader := &testPacketBatchReader{
		batches: []testPacketBatch{{
			payloads:     [][]byte{[]byte("a"), []byte("bc")},
			destinations: []M.Socksaddr{destinationA, destinationB},
		}},
	}
	writer := &testConnectedPacketBatchWriter{}
	var readBytes, readPackets, writeBytes, writePackets atomic.Int64
	n, err := CopyPacketWithCounters(writer, reader, reader, []N.CountFunc{
		func(n int64) { readBytes.Add(n) },
		func(n int64) { readPackets.Add(1) },
	}, []N.CountFunc{
		func(n int64) { writeBytes.Add(n) },
		func(n int64) { writePackets.Add(1) },
	})
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, int64(3), n)
	require.True(t, writer.usedConnectedBatch)
	require.Equal(t, [][]byte{[]byte("a"), []byte("bc")}, writer.payloads)
	require.Equal(t, int64(3), readBytes.Load())
	require.Equal(t, int64(2), readPackets.Load())
	require.Equal(t, int64(3), writeBytes.Load())
	require.Equal(t, int64(2), writePackets.Load())
}

func TestCopyPacketUsesConnectedBatchReader(t *testing.T) {
	t.Parallel()
	destination := M.SocksaddrFrom(netip.MustParseAddr("127.0.0.1"), 1000)
	reader := &testConnectedPacketBatchReader{
		destination: destination,
		batches: [][]byte{
			[]byte("a"),
			[]byte("bc"),
		},
	}
	writer := &testPacketBatchWriter{}
	var readBytes, readPackets, writeBytes, writePackets atomic.Int64
	n, err := CopyPacketWithCounters(writer, reader, reader, []N.CountFunc{
		func(n int64) { readBytes.Add(n) },
		func(n int64) { readPackets.Add(1) },
	}, []N.CountFunc{
		func(n int64) { writeBytes.Add(n) },
		func(n int64) { writePackets.Add(1) },
	})
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, int64(3), n)
	require.True(t, writer.usedBatch)
	require.Equal(t, [][]byte{[]byte("a"), []byte("bc")}, writer.payloads)
	require.Equal(t, []M.Socksaddr{destination, destination}, writer.destinations)
	require.Equal(t, int64(3), readBytes.Load())
	require.Equal(t, int64(2), readPackets.Load())
	require.Equal(t, int64(3), writeBytes.Load())
	require.Equal(t, int64(2), writePackets.Load())
}

func TestCopyPacketCachedWriteErrorAfterSuccess(t *testing.T) {
	t.Parallel()
	destination := M.SocksaddrFrom(netip.MustParseAddr("127.0.0.1"), 1000)
	writeErr := errors.New("cached write failed")
	reader := &testCachedPacketReader{
		packets: []*N.PacketBuffer{
			testPacketBuffer("a", destination),
			testPacketBuffer("bc", destination),
		},
		readErr: io.EOF,
	}
	writer := &testFailAfterPacketWriter{
		failAt: 2,
		err:    writeErr,
	}
	n, err := CopyPacket(writer, reader)
	require.ErrorIs(t, err, writeErr)
	require.Equal(t, int64(1), n)
}

func TestNewPacketBatchWriterFallback(t *testing.T) {
	t.Parallel()
	destinationA := M.SocksaddrFrom(netip.MustParseAddr("127.0.0.1"), 1000)
	destinationB := M.SocksaddrFrom(netip.MustParseAddr("127.0.0.1"), 1001)
	writer := &testPacketWriter{}
	_, created := CreatePacketBatchWriter(writer)
	require.False(t, created)
	batchWriter := NewPacketBatchWriter(writer)
	require.NoError(t, batchWriter.WritePacketBatch(testBuffers("a", "bc"), []M.Socksaddr{destinationA, destinationB}))
	require.Equal(t, [][]byte{[]byte("a"), []byte("bc")}, writer.payloads)
	require.Equal(t, []M.Socksaddr{destinationA, destinationB}, writer.destinations)
}

func TestNATPacketBatchWriter(t *testing.T) {
	t.Parallel()
	origin := M.SocksaddrFrom(netip.MustParseAddr("10.0.0.1"), 1000)
	destination := M.SocksaddrFrom(netip.MustParseAddr("20.0.0.1"), 2000)
	other := M.SocksaddrFrom(netip.MustParseAddr("30.0.0.1"), 3000)
	conn := &testNetPacketConn{}
	natConn := NewNATPacketConn(conn, origin, destination)
	writer, created := CreatePacketBatchWriter(natConn)
	require.True(t, created)
	require.NoError(t, writer.WritePacketBatch(testBuffers("a", "bc"), []M.Socksaddr{
		M.SocksaddrFrom(destination.Addr, 9999),
		other,
	}))
	require.Equal(t, [][]byte{[]byte("a"), []byte("bc")}, conn.payloads)
	require.Equal(t, []M.Socksaddr{
		M.SocksaddrFrom(origin.Addr, 9999),
		other,
	}, conn.destinations)
}

func TestRemoteAddrDoesNotCreateConnectedPacketBatch(t *testing.T) {
	t.Parallel()
	conn := &testRemoteAddrPacketConn{}
	_, readCreated := CreateConnectedPacketBatchReadWaiter(conn)
	require.False(t, readCreated)
	_, writeCreated := CreateConnectedPacketBatchWriter(conn)
	require.False(t, writeCreated)
}

func TestUnconnectedUnbindPacketConnDoesNotCreateConnectedPacketBatch(t *testing.T) {
	t.Parallel()
	inputConn, outputConn, outputAddr := UDPPipe(t)
	defer inputConn.Close()
	defer outputConn.Close()
	packetConn := NewUnbindPacketConnWithAddr(inputConn.(*net.UDPConn), outputAddr)
	_, readCreated := CreateConnectedPacketBatchReadWaiter(packetConn)
	require.False(t, readCreated)
	_, writeCreated := CreateConnectedPacketBatchWriter(packetConn)
	require.False(t, writeCreated)
}

func TestPacketBatchUDP(t *testing.T) {
	t.Parallel()
	for _, batchSize := range []int{1, 2, DefaultPacketReadBatchSize} {
		t.Run(strconv.Itoa(batchSize), func(t *testing.T) {
			inputConn, outputConn, outputAddr := UDPPipe(t)
			defer inputConn.Close()
			defer outputConn.Close()
			require.NoError(t, inputConn.SetDeadline(time.Now().Add(time.Second)))
			require.NoError(t, outputConn.SetDeadline(time.Now().Add(time.Second)))
			packetInputConn := NewPacketConn(inputConn)
			reader, readCreated := CreatePacketBatchReadWaiter(packetInputConn)
			writer, writeCreated := CreatePacketBatchWriter(packetInputConn)
			if !readCreated && !writeCreated {
				t.Skip("packet batch syscall backend is not available on this platform")
			}
			if writeCreated {
				require.NoError(t, writer.WritePacketBatch(testBuffers("x", "yz"), []M.Socksaddr{outputAddr, outputAddr}))
				output := make([]byte, 2)
				n, _, err := outputConn.ReadFrom(output)
				require.NoError(t, err)
				require.Equal(t, []byte("x"), output[:n])
				n, _, err = outputConn.ReadFrom(output)
				require.NoError(t, err)
				require.Equal(t, []byte("yz"), output[:n])
			}
			if readCreated {
				reader.InitializeReadWaiter(N.ReadWaitOptions{BatchSize: batchSize})
				_, err := outputConn.WriteTo([]byte("a"), inputConn.LocalAddr())
				require.NoError(t, err)
				_, err = outputConn.WriteTo([]byte("bc"), inputConn.LocalAddr())
				require.NoError(t, err)
				var payloads [][]byte
				var destinations []M.Socksaddr
				for len(payloads) < 2 {
					buffers, destinationsN, err := reader.WaitReadPackets()
					require.NoError(t, err)
					require.NotEmpty(t, buffers)
					require.Len(t, destinationsN, len(buffers))
					for index, buffer := range buffers {
						payloads = append(payloads, append([]byte(nil), buffer.Bytes()...))
						destinations = append(destinations, destinationsN[index])
						buffer.Release()
					}
				}
				require.Equal(t, [][]byte{[]byte("a"), []byte("bc")}, payloads)
				require.Equal(t, []M.Socksaddr{outputAddr, outputAddr}, destinations)
			}
		})
	}
}

func TestConnectedPacketBatchUDP(t *testing.T) {
	t.Parallel()
	for _, batchSize := range []int{1, 2, DefaultPacketReadBatchSize} {
		t.Run(strconv.Itoa(batchSize), func(t *testing.T) {
			serverConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
			require.NoError(t, err)
			defer serverConn.Close()
			clientConn, err := net.DialUDP("udp", nil, serverConn.LocalAddr().(*net.UDPAddr))
			require.NoError(t, err)
			defer clientConn.Close()
			packetConn := NewUnbindPacketConn(clientConn)
			_, ordinaryReadCreated := CreatePacketBatchReadWaiter(packetConn)
			require.False(t, ordinaryReadCreated)
			_, ordinaryWriteCreated := CreatePacketBatchWriter(packetConn)
			require.False(t, ordinaryWriteCreated)
			reader, readCreated := CreateConnectedPacketBatchReadWaiter(packetConn)
			writer, writeCreated := CreateConnectedPacketBatchWriter(packetConn)
			if !readCreated || !writeCreated {
				t.Skip("connected packet batch syscall backend is not available on this platform")
			}
			require.NoError(t, serverConn.SetDeadline(time.Now().Add(time.Second)))
			require.NoError(t, clientConn.SetDeadline(time.Now().Add(time.Second)))
			reader.InitializeReadWaiter(N.ReadWaitOptions{BatchSize: batchSize})
			require.NoError(t, writer.WriteConnectedPacketBatch(testBuffers("x", "yz")))
			output := make([]byte, 2)
			n, addr, err := serverConn.ReadFromUDP(output)
			require.NoError(t, err)
			require.Equal(t, clientConn.LocalAddr().String(), addr.String())
			require.Equal(t, []byte("x"), output[:n])
			n, addr, err = serverConn.ReadFromUDP(output)
			require.NoError(t, err)
			require.Equal(t, clientConn.LocalAddr().String(), addr.String())
			require.Equal(t, []byte("yz"), output[:n])
			_, err = serverConn.WriteToUDP([]byte("a"), clientConn.LocalAddr().(*net.UDPAddr))
			require.NoError(t, err)
			_, err = serverConn.WriteToUDP([]byte("bc"), clientConn.LocalAddr().(*net.UDPAddr))
			require.NoError(t, err)
			var payloads [][]byte
			for len(payloads) < 2 {
				buffers, destination, err := reader.WaitReadConnectedPackets()
				require.NoError(t, err)
				require.Equal(t, M.SocksaddrFromNet(clientConn.RemoteAddr()).Unwrap(), destination)
				for _, buffer := range buffers {
					payloads = append(payloads, append([]byte(nil), buffer.Bytes()...))
					buffer.Release()
				}
			}
			require.Equal(t, [][]byte{[]byte("a"), []byte("bc")}, payloads)
		})
	}
}

type testPacketBatch struct {
	payloads     [][]byte
	destinations []M.Socksaddr
}

type testPacketBatchReader struct {
	batches []testPacketBatch
	index   int
}

func (r *testPacketBatchReader) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	return M.Socksaddr{}, io.ErrUnexpectedEOF
}

func (r *testPacketBatchReader) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	return false
}

func (r *testPacketBatchReader) WaitReadPackets() ([]*buf.Buffer, []M.Socksaddr, error) {
	if r.index >= len(r.batches) {
		return nil, nil, io.EOF
	}
	batch := r.batches[r.index]
	r.index++
	return testBuffersBytes(batch.payloads...), append([]M.Socksaddr(nil), batch.destinations...), nil
}

type testPacketBatchWriter struct {
	usedBatch    bool
	payloads     [][]byte
	destinations []M.Socksaddr
}

func (w *testPacketBatchWriter) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	return io.ErrUnexpectedEOF
}

func (w *testPacketBatchWriter) WritePacketBatch(buffers []*buf.Buffer, destinations []M.Socksaddr) error {
	w.usedBatch = true
	for index, buffer := range buffers {
		w.payloads = append(w.payloads, append([]byte(nil), buffer.Bytes()...))
		w.destinations = append(w.destinations, destinations[index])
	}
	buf.ReleaseMulti(buffers)
	return nil
}

type testConnectedPacketBatchWriter struct {
	usedConnectedBatch bool
	payloads           [][]byte
}

func (w *testConnectedPacketBatchWriter) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	return io.ErrUnexpectedEOF
}

func (w *testConnectedPacketBatchWriter) WriteConnectedPacketBatch(buffers []*buf.Buffer) error {
	w.usedConnectedBatch = true
	for _, buffer := range buffers {
		w.payloads = append(w.payloads, append([]byte(nil), buffer.Bytes()...))
	}
	buf.ReleaseMulti(buffers)
	return nil
}

type testConnectedPacketBatchReader struct {
	destination M.Socksaddr
	batches     [][]byte
	index       int
}

func (r *testConnectedPacketBatchReader) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	return M.Socksaddr{}, io.ErrUnexpectedEOF
}

func (r *testConnectedPacketBatchReader) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	return false
}

func (r *testConnectedPacketBatchReader) WaitReadConnectedPackets() ([]*buf.Buffer, M.Socksaddr, error) {
	if r.index >= len(r.batches) {
		return nil, M.Socksaddr{}, io.EOF
	}
	buffers := testBuffersBytes(r.batches[r.index:]...)
	r.index = len(r.batches)
	return buffers, r.destination, nil
}

type testCachedPacketReader struct {
	packets []*N.PacketBuffer
	readErr error
}

func (r *testCachedPacketReader) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	return M.Socksaddr{}, r.readErr
}

func (r *testCachedPacketReader) ReadCachedPacket() *N.PacketBuffer {
	if len(r.packets) == 0 {
		return nil
	}
	packet := r.packets[0]
	r.packets = r.packets[1:]
	return packet
}

type testFailAfterPacketWriter struct {
	count  int
	failAt int
	err    error
}

func (w *testFailAfterPacketWriter) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	w.count++
	if w.count == w.failAt {
		return w.err
	}
	buffer.Release()
	return nil
}

type testPacketWriter struct {
	payloads     [][]byte
	destinations []M.Socksaddr
}

func (w *testPacketWriter) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	w.payloads = append(w.payloads, append([]byte(nil), buffer.Bytes()...))
	w.destinations = append(w.destinations, destination)
	buffer.Release()
	return nil
}

type testNetPacketConn struct {
	testPacketBatchWriter
}

func (c *testNetPacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	return M.Socksaddr{}, io.ErrUnexpectedEOF
}

func (c *testNetPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	return 0, nil, io.ErrUnexpectedEOF
}

func (c *testNetPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	return len(p), nil
}

func (c *testNetPacketConn) Close() error {
	return nil
}

func (c *testNetPacketConn) LocalAddr() net.Addr {
	return M.SocksaddrFrom(netip.MustParseAddr("127.0.0.1"), 0).UDPAddr()
}

func (c *testNetPacketConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *testNetPacketConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *testNetPacketConn) SetWriteDeadline(t time.Time) error {
	return nil
}

type testRemoteAddrPacketConn struct {
	testPacketWriter
}

func (c *testRemoteAddrPacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	return M.Socksaddr{}, io.ErrUnexpectedEOF
}

func (c *testRemoteAddrPacketConn) RemoteAddr() net.Addr {
	return M.SocksaddrFrom(netip.MustParseAddr("127.0.0.1"), 1000)
}

func testPacketBuffer(data string, destination M.Socksaddr) *N.PacketBuffer {
	packet := N.NewPacketBuffer()
	packet.Buffer = buf.As([]byte(data)).ToOwned()
	packet.Destination = destination
	return packet
}

func testBuffers(values ...string) []*buf.Buffer {
	payloads := make([][]byte, len(values))
	for index, value := range values {
		payloads[index] = []byte(value)
	}
	return testBuffersBytes(payloads...)
}

func testBuffersBytes(values ...[]byte) []*buf.Buffer {
	buffers := make([]*buf.Buffer, len(values))
	for index, value := range values {
		buffer := buf.NewSize(len(value))
		common.Must1(buffer.Write(value))
		buffers[index] = buffer
	}
	return buffers
}
