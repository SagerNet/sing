//go:build !(darwin || dragonfly || freebsd || netbsd || openbsd)

package control

func IsUDPEOF(fd uintptr) bool {
	return false
}
