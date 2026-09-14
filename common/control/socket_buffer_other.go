//go:build !(unix || windows)

package control

func setSocketBuffer(fd uintptr, size int) {
}
