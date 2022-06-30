//go:build !linux

package control

func BindToInterface(interfaceName string) Func {
	return nil
}
