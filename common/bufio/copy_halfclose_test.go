package bufio

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

// isTimeoutErr reports whether err chains to a net.Error whose Timeout returns
// true (the error a blocked Read returns after SetDeadline elapses). It traverses
// Unwrap chains, so it still matches after task.Group wraps the error with a
// cause message.
func isTimeoutErr(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// withConnDoneTimeout temporarily shortens the teardown deadline for the duration
// of a test. The override is stored atomically, so it is race-free even if other
// CopyConn calls run concurrently. t.Cleanup restores the default on exit,
// including when the test fails.
func withConnDoneTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	override := d
	connDoneTimeoutOverride.Store(&override)
	t.Cleanup(func() { connDoneTimeoutOverride.Store(nil) })
}

// closeWrite fails the test if the half-close fails: a failed CloseWrite would
// mean the test never set up the half-closed state it intends to exercise.
func closeWrite(t *testing.T, c *net.TCPConn) {
	t.Helper()
	if err := c.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite: %v", err)
	}
}

// TestCopyConnHalfCloseDeadline verifies the duplex leak fix: when one copy
// direction reaches EOF, the surviving direction's blocked Read is woken by the
// teardown deadline instead of parking forever. TCP pairs are used (not
// net.Pipe) because the test needs a real half-close via CloseWrite -- the
// duplex teardown calls CloseWrite, which half-closes write only and leaves the
// surviving Read parked, which is exactly the condition the deadline fixes.
func TestCopyConnHalfCloseDeadline(t *testing.T) {
	withConnDoneTimeout(t, 200*time.Millisecond)

	source, client := tcpPair(t)
	destination, server := tcpPair(t)
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- CopyConn(ctx, source, destination) }()

	// Drive one byte through upload (client -> source -> destination -> server)
	// and drain it on the server side. server.Read succeeding confirms upload
	// has entered its loop and forwarded the byte.
	if _, err := client.Write([]byte{0}); err != nil {
		t.Fatalf("client write: %v", err)
	}
	one := make([]byte, 1)
	if _, err := server.Read(one); err != nil {
		t.Fatalf("server read handshake byte: %v", err)
	}

	// Client half-closes its write side -> upload reaches EOF -> duplex
	// teardown CloseWrite(destination) half-closes write, leaving download
	// parked in Read on destination. The server never writes again, so only the
	// teardown deadline can wake download.
	closeWrite(t, client.(*net.TCPConn))

	select {
	case err := <-done:
		// The surviving direction was woken by the deadline, so CopyConn
		// returns a timeout error rather than blocking until ctx expires.
		if err == nil {
			t.Fatal("CopyConn returned nil; expected a timeout from the woken surviving direction")
		}
		if !isTimeoutErr(err) {
			t.Errorf("CopyConn error is not a timeout: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("CopyConn did not return within 1s of half-close; teardown deadline not armed")
	}
}

// TestCopyConnBothDirectionsClose verifies the deadline does not fire spuriously
// on a natural close: when both peers half-close, both copy directions reach EOF
// and CopyConn returns nil promptly. The default 30s deadline never elapses
// because no direction stalls.
func TestCopyConnBothDirectionsClose(t *testing.T) {
	source, client := tcpPair(t)
	destination, server := tcpPair(t)
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- CopyConn(ctx, source, destination) }()

	// Handshake bytes both ways so both goroutines enter their loops.
	if _, err := client.Write([]byte{1}); err != nil {
		t.Fatalf("client write: %v", err)
	}
	if _, err := server.Write([]byte{2}); err != nil {
		t.Fatalf("server write: %v", err)
	}
	one := make([]byte, 1)
	if _, err := server.Read(one); err != nil {
		t.Fatalf("server read: %v", err)
	}
	if _, err := client.Read(one); err != nil {
		t.Fatalf("client read: %v", err)
	}

	// Both sides half-close -> both directions reach EOF naturally -> CopyConn
	// returns nil well before the default teardown deadline elapses.
	closeWrite(t, client.(*net.TCPConn))
	closeWrite(t, server.(*net.TCPConn))

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("CopyConn returned non-nil error on natural close: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("CopyConn did not return within 1s of natural close")
	}
}

// nonDuplexConn wraps a net.Conn to hide CloseWrite, forcing CopyConn onto the
// non-duplex teardown path (common.Close, full close). It does not implement
// WithUpstream or a NetConn() method, so common.Cast[N.WriteCloser] does not
// unwrap to the underlying TCPConn.
type nonDuplexConn struct{ net.Conn }

// TestCopyConnNonDuplexCloseWakesPeer verifies the non-duplex path self-heals
// without a deadline: a full Close on teardown wakes the surviving direction's
// blocked Read immediately, so CopyConn returns promptly. This is why the patch
// arms a deadline only in the duplex branches.
func TestCopyConnNonDuplexCloseWakesPeer(t *testing.T) {
	source, client := tcpPair(t)
	destination, server := tcpPair(t)
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- CopyConn(ctx, nonDuplexConn{source}, nonDuplexConn{destination})
	}()

	if _, err := client.Write([]byte{0}); err != nil {
		t.Fatalf("client write: %v", err)
	}
	one := make([]byte, 1)
	if _, err := server.Read(one); err != nil {
		t.Fatalf("server read handshake byte: %v", err)
	}

	// Client half-closes -> upload (non-duplex) reaches EOF -> teardown
	// common.Close(destination) fully closes destination, waking download's
	// parked Read. No deadline is involved.
	closeWrite(t, client.(*net.TCPConn))

	select {
	case <-done:
		// CopyConn returned: the surviving direction was woken by Close.
	case <-time.After(1 * time.Second):
		t.Fatal("CopyConn did not return within 1s; non-duplex Close did not wake surviving direction")
	}
}

// TestCopyConnConcurrentOverride runs several relays concurrently while the
// teardown-deadline override is set, under the race detector. It confirms the
// atomic override is read safely by concurrent CopyConn calls and that the
// shortened deadline does not interfere across independent relays.
func TestCopyConnConcurrentOverride(t *testing.T) {
	withConnDoneTimeout(t, 200*time.Millisecond)

	const n = 4
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		source, client := tcpPair(t)
		destination, server := tcpPair(t)
		defer server.Close()
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		go func() {
			defer wg.Done()
			done := make(chan error, 1)
			go func() { done <- CopyConn(ctx, source, destination) }()

			if _, err := client.Write([]byte{0}); err != nil {
				t.Errorf("client write: %v", err)
				return
			}
			one := make([]byte, 1)
			if _, err := server.Read(one); err != nil {
				t.Errorf("server read: %v", err)
				return
			}
			closeWrite(t, client.(*net.TCPConn))

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Errorf("CopyConn did not return within 2s")
			}
		}()
	}
	wg.Wait()
}

// tcpPair returns a connected loopback TCP conn pair (a, b). The listener is
// always closed before return so the accept goroutine exits without leaking.
func tcpPair(t *testing.T) (net.Conn, net.Conn) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			close(accepted)
			return
		}
		accepted <- c
	}()
	b, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		ln.Close()
		t.Fatalf("dial: %v", err)
	}
	a := <-accepted
	ln.Close()
	if a == nil {
		b.Close()
		t.Fatalf("accept failed")
	}
	return a, b
}
