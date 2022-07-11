//go:build !linux

package control

func ProtectPath(protectPath string) Func {
	return nil
}
