//go:build !windows

package control

func DisableUDPNetReset() Func {
	return nil
}
