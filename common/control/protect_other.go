//go:build !android

package control

func ProtectPath(protectPath string) Func {
	return nil
}
