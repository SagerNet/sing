package udpnat

import (
	"context"
	"errors"
	"io"
	"net"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/pipe"
)

type lifecycleWriter struct {
	closed atomic.Int32
	err    error
}

type replyAddressRecorder struct {
	N.PacketConn
	address M.Socksaddr
}

func (w *replyAddressRecorder) WritePacket(b *buf.Buffer, address M.Socksaddr) error {
	w.address = address
	b.Release()
	return nil
}

func (*lifecycleWriter) WritePacket(b *buf.Buffer, _ M.Socksaddr) error { b.Release(); return nil }
func (w *lifecycleWriter) Close() error                                 { w.closed.Add(1); return w.err }

func lifecycleBuffer() *buf.Buffer { b := buf.NewSize(32); b.WriteString("data"); return b }

func lifecycleConn() *conn {
	ctx, cancel := context.WithCancelCause(context.Background())
	return &conn{ctx: ctx, cancel: cancel, data: make(chan packet, 64), sourceReady: true, readDeadline: pipe.MakeDeadline()}
}

func TestCloseUnblocksFullQueueAndReleasesPackets(t *testing.T) {
	c := lifecycleConn()
	defer c.Close()
	packets := make([]*buf.Buffer, 65)
	for i := 0; i < 64; i++ {
		packets[i] = lifecycleBuffer()
		c.enqueue(packet{data: packets[i]})
	}
	packets[64] = lifecycleBuffer()
	done := make(chan struct{})
	go func() { c.enqueue(packet{data: packets[64]}); close(done) }()
	deadline := time.Now().Add(3 * time.Second)
	for c.enqueueAccess.TryLock() {
		c.enqueueAccess.Unlock()
		if time.Now().After(deadline) {
			t.Fatal("producer did not enter full queue")
		}
		runtime.Gosched()
	}
	closed := make(chan error, 1)
	go func() { closed <- c.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("close blocked behind producer")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("enqueue not canceled")
	}
	for i, b := range packets {
		if b.Len() != 0 {
			t.Fatalf("packet %d retained after close", i)
		}
	}
	if len(c.data) != 0 {
		t.Fatal("queue not drained")
	}
}

func TestClosePreservesConsumerOwnedPacket(t *testing.T) {
	c := lifecycleConn()
	b := lifecycleBuffer()
	c.enqueue(packet{data: b, destination: M.ParseSocksaddr("127.0.0.1:53")})
	c.InitializeReadWaiter(N.ReadWaitOptions{})
	owned, destination, err := c.WaitReadPacket()
	if err != nil || string(owned.Bytes()) != "data" || destination.Port != 53 {
		t.Fatalf("wait result: %v %v", destination, err)
	}
	defer owned.Release()
	c.Close()
	if string(owned.Bytes()) != "data" {
		t.Fatal("close released consumer-owned buffer")
	}
}

func TestWaitReadErrorReleasesDequeuedPacket(t *testing.T) {
	c := lifecycleConn()
	defer c.Close()
	b := lifecycleBuffer()
	c.enqueue(packet{data: b})
	c.InitializeReadWaiter(N.ReadWaitOptions{MTU: 1, FrontHeadroom: 1})
	got, _, err := c.WaitReadPacket()
	if !errors.Is(err, io.ErrShortBuffer) || got != nil || b.Len() != 0 {
		t.Fatalf("failed copied packet retained: got=%v inputLen=%d err=%v", got, b.Len(), err)
	}
}

func TestReadPacketShortBufferReleasesDequeuedPacket(t *testing.T) {
	c := lifecycleConn()
	defer c.Close()
	input, output := lifecycleBuffer(), buf.NewSize(1)
	defer output.Release()
	c.enqueue(packet{data: input, destination: M.ParseSocksaddr("127.0.0.1:53")})
	destination, err := c.ReadPacket(output)
	if !errors.Is(err, io.ErrShortBuffer) || input.Len() != 0 || string(output.Bytes()) != "d" || destination.Port != 53 {
		t.Fatalf("short packet: destination=%v input=%d output=%q err=%v", destination, input.Len(), output.Bytes(), err)
	}
}

func TestInitializerSameKeyReentryPreservesQueueCapacity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan struct{})
	service := New[int](60, lifecycleHandler{start: func(_ context.Context, c N.PacketConn, _ M.Metadata) error {
		defer close(finished)
		b := buf.NewSize(32)
		defer b.Release()
		for i := 0; i < 65; i++ {
			b.Reset()
			if _, err := c.ReadPacket(b); err != nil {
				return err
			}
			want := "data"
			if i == 64 {
				want = "outer"
			}
			if string(b.Bytes()) != want {
				t.Errorf("packet %d = %q want %q", i, b.Bytes(), want)
			}
		}
		return nil
	}})
	returned := make(chan struct{})
	go func() {
		outer := buf.NewSize(32)
		outer.WriteString("outer")
		service.NewPacket(ctx, 1, outer, M.Metadata{}, func(N.PacketConn) N.PacketWriter {
			for i := 0; i < 64; i++ {
				service.NewPacket(ctx, 1, lifecycleBuffer(), M.Metadata{}, func(N.PacketConn) N.PacketWriter { t.Error("reentry initialized twice"); return nil })
			}
			return new(lifecycleWriter)
		})
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("initializer reentry blocked")
	}
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("queued packets did not drain")
	}
}

type lifecycleHandler struct {
	start func(context.Context, N.PacketConn, M.Metadata) error
}

func (h lifecycleHandler) NewPacketConnection(ctx context.Context, c N.PacketConn, m M.Metadata) error {
	return h.start(ctx, c, m)
}
func (lifecycleHandler) NewError(context.Context, error) {}

func TestCloseDuringSourcePreparation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready, release := make(chan N.PacketConn, 1), make(chan struct{})
	var started atomic.Int32
	service := New[int](60, lifecycleHandler{start: func(context.Context, N.PacketConn, M.Metadata) error { started.Add(1); return nil }})
	failure := errors.New("source close failed")
	source := &lifecycleWriter{err: failure}
	b := lifecycleBuffer()
	done := make(chan struct{})
	go func() {
		service.NewPacket(ctx, 1, b, M.Metadata{}, func(c N.PacketConn) N.PacketWriter { ready <- c; <-release; return source })
		close(done)
	}()
	c := <-ready
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("preparation did not finish")
	}
	if b.Len() != 0 || source.closed.Load() != 1 || started.Load() != 0 {
		t.Fatalf("input=%d sourceCloses=%d callbacks=%d", b.Len(), source.closed.Load(), started.Load())
	}
	if err := c.Close(); err != failure {
		t.Fatalf("published source close error lost: %v", err)
	}
}

func TestCanceledInputIsReleased(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := New[int](60, lifecycleHandler{start: func(context.Context, N.PacketConn, M.Metadata) error {
		t.Error("canceled callback started")
		return nil
	}})
	b := lifecycleBuffer()
	service.NewPacket(ctx, 1, b, M.Metadata{}, func(N.PacketConn) N.PacketWriter { t.Error("canceled source initialized"); return new(lifecycleWriter) })
	if b.Len() != 0 {
		t.Fatal("canceled packet retained")
	}
}

func TestConcurrentSourceUpdatesAndReads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opened := make(chan N.PacketConn, 1)
	handlerDone := make(chan struct{})
	service := New[int](60, lifecycleHandler{start: func(ctx context.Context, c N.PacketConn, _ M.Metadata) error {
		defer close(handlerDone)
		opened <- c
		b := buf.NewSize(32)
		defer b.Release()
		for {
			b.Reset()
			if _, err := c.ReadPacket(b); err != nil {
				return nil
			}
		}
	}})
	send := func(port uint16) {
		service.NewPacket(ctx, 1, lifecycleBuffer(), M.Metadata{Source: M.Socksaddr{Addr: M.ParseSocksaddr("127.0.0.1:1").Addr, Port: port}}, func(N.PacketConn) N.PacketWriter { return new(lifecycleWriter) })
	}
	send(10000)
	c := <-opened
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for i := 0; i < 1000; i++ {
			if c.LocalAddr().Network() == "" {
				t.Error("source lost")
			}
		}
	}()
	for i := 0; i < 1000; i++ {
		send(uint16(10000 + i))
	}
	<-readerDone
	if got := c.LocalAddr().(M.Socksaddr).Port; got != 10999 {
		t.Fatalf("current source port=%d, want 10999", got)
	}
	replies := new(replyAddressRecorder)
	if err := (&DirectBackWriter{Source: replies, Nat: c}).WritePacket(lifecycleBuffer(), M.Socksaddr{}); err != nil {
		t.Fatal(err)
	}
	if replies.address.Port != 10999 {
		t.Fatalf("reply destination port=%d, want 10999", replies.address.Port)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("reader not unblocked")
	}
	if !errors.Is(context.Cause(c.(*conn).ctx), net.ErrClosed) {
		t.Fatal("native close cause changed")
	}
}
