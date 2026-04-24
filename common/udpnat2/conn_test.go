package udpnat

import (
	"testing"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/pipe"
)

func TestNatConnPacketBatchReadWaiter(t *testing.T) {
	conn := &natConn{
		writer:       testPacketWriter{},
		packetChan:   make(chan *N.PacketBuffer, 3),
		doneChan:     make(chan struct{}),
		readDeadline: pipe.MakeDeadline(),
	}
	conn.InitializeReadWaiter(N.ReadWaitOptions{BatchSize: 2})
	conn.packetChan <- testPacketBuffer("a", M.ParseSocksaddr("1.1.1.1:53"))
	conn.packetChan <- testPacketBuffer("bb", M.ParseSocksaddr("2.2.2.2:53"))
	conn.packetChan <- testPacketBuffer("ccc", M.ParseSocksaddr("3.3.3.3:53"))

	readWaiter, created := conn.CreatePacketBatchReadWaiter()
	if !created {
		t.Fatal("CreatePacketBatchReadWaiter returned false")
	}
	buffers, destinations, err := readWaiter.WaitReadPackets()
	if err != nil {
		t.Fatal(err)
	}
	defer buf.ReleaseMulti(buffers)
	if len(buffers) != 2 {
		t.Fatalf("batch size mismatch: %d", len(buffers))
	}
	if string(buffers[0].Bytes()) != "a" || string(buffers[1].Bytes()) != "bb" {
		t.Fatalf("unexpected buffers: %q %q", buffers[0].Bytes(), buffers[1].Bytes())
	}
	if destinations[0] != M.ParseSocksaddr("1.1.1.1:53") || destinations[1] != M.ParseSocksaddr("2.2.2.2:53") {
		t.Fatalf("unexpected destinations: %v", destinations)
	}
}

func TestNatConnPacketBatchWriterCreator(t *testing.T) {
	writer := &testPacketBatchWriter{}
	conn := &natConn{writer: writer}
	batchWriter, created := conn.CreatePacketBatchWriter()
	if !created {
		t.Fatal("CreatePacketBatchWriter returned false")
	}
	err := batchWriter.WritePacketBatch([]*buf.Buffer{
		buf.As([]byte("a")).ToOwned(),
		buf.As([]byte("bb")).ToOwned(),
	}, []M.Socksaddr{
		M.ParseSocksaddr("1.1.1.1:53"),
		M.ParseSocksaddr("2.2.2.2:53"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if writer.count != 2 {
		t.Fatalf("unexpected write count: %d", writer.count)
	}
}

func testPacketBuffer(data string, destination M.Socksaddr) *N.PacketBuffer {
	packet := N.NewPacketBuffer()
	packet.Buffer = buf.As([]byte(data)).ToOwned()
	packet.Destination = destination
	return packet
}

type testPacketWriter struct{}

func (testPacketWriter) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	buffer.Release()
	return nil
}

type testPacketBatchWriter struct {
	count int
}

func (w *testPacketBatchWriter) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	buffer.Release()
	w.count++
	return nil
}

func (w *testPacketBatchWriter) WritePacketBatch(buffers []*buf.Buffer, destinations []M.Socksaddr) error {
	defer buf.ReleaseMulti(buffers)
	w.count += len(buffers)
	return nil
}
