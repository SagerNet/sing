//go:build !linux && !windows

package control

func ReuseAddr() Func {
	return nil
}
