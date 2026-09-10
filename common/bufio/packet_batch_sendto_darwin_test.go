package bufio

import (
	"net/netip"
	"os"
	"syscall"
	"testing"

	M "github.com/sagernet/sing/common/metadata"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestPacketBatchSendtoResume(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name        string
		errors      []syscall.Errno
		attempts    []string
		sent        []string
		pollErr     error
		wantErr     error
		wantWakeups int
	}{
		{
			name:        "interrupted_and_blocked",
			errors:      []syscall.Errno{0, syscall.EINTR, syscall.EAGAIN, 0, 0},
			attempts:    []string{"a", "b", "b", "b", "c"},
			sent:        []string{"a", "b", "c"},
			wantWakeups: 2,
		},
		{
			name:        "partial_error",
			errors:      []syscall.Errno{0, syscall.EMSGSIZE},
			attempts:    []string{"a", "b"},
			sent:        []string{"a"},
			wantErr:     syscall.EMSGSIZE,
			wantWakeups: 1,
		},
		{
			name:        "partial_deadline",
			errors:      []syscall.Errno{0, syscall.EAGAIN},
			attempts:    []string{"a", "b"},
			sent:        []string{"a"},
			pollErr:     os.ErrDeadlineExceeded,
			wantErr:     os.ErrDeadlineExceeded,
			wantWakeups: 1,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			rawConn := &testSendtoRawConn{pollErr: testCase.pollErr}
			writer := &syscallPacketBatchWriter{rawConn: rawConn, localAddr: netip.MustParseAddrPort("127.0.0.1:1000")}
			buffers := testBuffers("a", "b", "c")
			destination := M.ParseSocksaddr("127.0.0.1:2000")
			var attempts, sent []string
			err := writer.writePacketBatch(buffers, []M.Socksaddr{destination, destination, destination}, func(_ int, data []byte, name *unix.RawSockaddrAny, nameLen uint32) syscall.Errno {
				require.Less(t, len(attempts), len(testCase.errors), "unexpected send or retry")
				require.EqualValues(t, unix.SizeofSockaddrInet4, nameLen)
				require.Equal(t, destination, M.SocksaddrFromRawSockaddrAny(name))
				errno := testCase.errors[len(attempts)]
				attempts = append(attempts, string(data))
				if errno == 0 {
					sent = append(sent, string(data))
				}
				return errno
			})
			require.ErrorIs(t, err, testCase.wantErr)
			require.Equal(t, testCase.attempts, attempts)
			require.Equal(t, testCase.sent, sent)
			require.Equal(t, testCase.wantWakeups, rawConn.wakeups)
			for _, buffer := range buffers {
				require.Zero(t, buffer.Cap())
			}
		})
	}
}

type testSendtoRawConn struct {
	pollErr error
	wakeups int
}

func (c *testSendtoRawConn) Control(func(uintptr)) error {
	panic("unexpected control")
}

func (c *testSendtoRawConn) Read(func(uintptr) bool) error {
	panic("unexpected read")
}

func (c *testSendtoRawConn) Write(write func(uintptr) bool) error {
	for range 4 {
		c.wakeups++
		if write(0) {
			return nil
		}
		if c.pollErr != nil {
			return c.pollErr
		}
	}
	return syscall.EIO
}
