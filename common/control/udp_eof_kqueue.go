//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package control

import "golang.org/x/sys/unix"

func IsUDPEOF(fd uintptr) bool {
	kqueue, err := unix.Kqueue()
	if err != nil {
		return false
	}
	defer unix.Close(kqueue)
	changes := make([]unix.Kevent_t, 1)
	unix.SetKevent(&changes[0], int(fd), unix.EVFILT_READ, unix.EV_ADD)
	events := make([]unix.Kevent_t, 1)
	n, err := unix.Kevent(kqueue, changes, events, &unix.Timespec{})
	return err == nil && n == 1 && events[0].Flags&unix.EV_EOF != 0
}
