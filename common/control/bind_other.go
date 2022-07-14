//go:build !linux && !windows

package control

func NewBindManager() BindManager {
	return nil
}

func BindToInterface(manager BindManager, interfaceName string) Func {
	return nil
}

func BindToInterfaceFunc(manager BindManager, interfaceNameFunc func() string) Func {
	return nil
}

func BindToInterfaceIndexFunc(interfaceIndexFunc func() int) Func {
	return nil
}
