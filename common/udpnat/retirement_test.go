package udpnat

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	B "github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type retirementHandler struct {
	opened  chan N.PacketConn
	retire  chan struct{}
	starts  atomic.Int32
	workers sync.WaitGroup
	waiter  bool
}

func (h *retirementHandler) NewPacketConnection(ctx context.Context, conn N.PacketConn, _ M.Metadata) error {
	h.workers.Add(1)
	defer h.workers.Done()
	index := h.starts.Add(1)
	if h.waiter {
		conn.(N.PacketReadWaiter).InitializeReadWaiter(N.ReadWaitOptions{})
	}
	if _, err := readRetirementPacket(conn, h.waiter); err != nil {
		return err
	}
	h.opened <- conn
	if index == 1 {
		select {
		case <-h.retire:
		case <-ctx.Done():
		}
	} else {
		<-ctx.Done()
	}
	return nil
}

func (*retirementHandler) NewError(context.Context, error) {}

type retirementWriter struct{}

func (retirementWriter) WritePacket(packet *B.Buffer, _ M.Socksaddr) error {
	packet.Release()
	return nil
}

func TestRetiredSessionKeepsReplacement(t *testing.T) {
	for _, waiter := range []bool{false, true} {
		t.Run(map[bool]string{false: "ReadPacket", true: "WaitReadPacket"}[waiter], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) { checkRetirementReplacement(t, waiter) })
		})
	}
}

func readRetirementPacket(conn N.PacketConn, waiter bool) (string, error) {
	if waiter {
		packet, _, err := conn.(N.PacketReadWaiter).WaitReadPacket()
		if packet == nil {
			return "", err
		}
		defer packet.Release()
		return string(packet.Bytes()), err
	}
	packet := B.NewSize(64)
	defer packet.Release()
	_, err := conn.ReadPacket(packet)
	return string(packet.Bytes()), err
}

func checkRetirementReplacement(t *testing.T, waiter bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	handler := &retirementHandler{opened: make(chan N.PacketConn, 4), retire: make(chan struct{}), waiter: waiter}
	t.Cleanup(func() { cancel(); handler.workers.Wait() })
	native := New[uint64](60, handler)
	metadata := M.Metadata{Source: M.ParseSocksaddr("127.0.0.1:40000"), Destination: M.ParseSocksaddr("127.0.0.1:40001")}
	send := func(payload string) {
		native.NewPacket(ctx, 42, B.As([]byte(payload)).ToOwned(), metadata, func(N.PacketConn) N.PacketWriter { return retirementWriter{} })
	}
	await := func() N.PacketConn {
		t.Helper()
		select {
		case conn := <-handler.opened:
			return conn
		case <-time.After(3 * time.Second):
			t.Fatal("native association did not start")
			return nil
		}
	}
	send("first")
	old := await()
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	send("replacement")
	replacement := await()
	if replacement == old {
		t.Fatal("closed native object was reused")
	}
	close(handler.retire)
	// Wait for the old service goroutine to finish retirement, not just for
	// the handler to return. The replacement handler is blocked on ctx.Done.
	synctest.Wait()
	requireLiveReplacement(t, native, replacement)
	if err := replacement.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	send("still-routed-to-replacement")
	if packet, err := readRetirementPacket(replacement, waiter); err != nil || packet != "still-routed-to-replacement" || handler.starts.Load() != 2 {
		t.Fatalf("replacement lost after retirement: packet=%q starts=%d err=%v", packet, handler.starts.Load(), err)
	}
}

func requireLiveReplacement(t *testing.T, service *Service[uint64], replacement N.PacketConn) {
	t.Helper()
	current, ok := service.nat.Load(42)
	if !ok || current != replacement {
		t.Fatal("old callback removed the replacement")
	}
	if err := current.ctx.Err(); err != nil {
		t.Fatalf("old callback closed the replacement: %v", err)
	}
}

type retirementSession struct {
	conn    N.PacketConn
	onClose func(error)
}

type retirementHandlerEx chan retirementSession

func (h retirementHandlerEx) NewPacketConnectionEx(_ context.Context, c N.PacketConn, _, _ M.Socksaddr, onClose func(error)) {
	h <- retirementSession{c, onClose}
}

func TestExtendedHandlerRetirementKeepsReplacement(t *testing.T) {
	synctest.Test(t, checkExtendedHandlerRetirement)
}

func checkExtendedHandlerRetirement(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := make(retirementHandlerEx, 2)
	await := func() retirementSession {
		t.Helper()
		select {
		case session := <-h:
			return session
		case <-time.After(3 * time.Second):
			t.Fatal("native association did not start")
			return retirementSession{}
		}
	}
	service := NewEx[uint64](60, h)
	metadata := M.Metadata{Source: M.ParseSocksaddr("127.0.0.1:40000"), Destination: M.ParseSocksaddr("127.0.0.1:40001")}
	send := func(payload string) {
		service.NewPacketEx(ctx, 42, B.As([]byte(payload)).ToOwned(), metadata.Source, metadata.Destination,
			func(N.PacketConn) N.PacketWriter { return retirementWriter{} })
	}
	send("first")
	oldSession := await()
	old := oldSession.conn
	if packet, err := readRetirementPacket(old, false); err != nil || packet != "first" {
		t.Fatalf("old packet=%q err=%v", packet, err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	send("replacement")
	replacementSession := await()
	replacement := replacementSession.conn
	defer replacementSession.onClose(nil)
	if packet, err := readRetirementPacket(replacement, false); err != nil || packet != "replacement" {
		t.Fatalf("replacement packet=%q err=%v", packet, err)
	}
	oldSession.onClose(nil)
	requireLiveReplacement(t, service, replacement)
	if err := replacement.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	send("still-routed")
	if packet, err := readRetirementPacket(replacement, false); err != nil || packet != "still-routed" {
		t.Fatalf("replacement packet=%q err=%v", packet, err)
	}
}
