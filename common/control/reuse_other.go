//go:build !linux

package control

func ReuseAddr() Func {
	return nil
}
