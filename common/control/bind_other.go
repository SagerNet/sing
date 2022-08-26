//go:build !(linux || windows || darwin)

package control

func NewBindManager() BindManager {
	return nil
}

func BindToInterface(manager BindManager, interfaceName string) Func {
	return nil
}

func BindToInterfaceFunc(manager BindManager, interfaceNameFunc func(network, address string) string) Func {
	return nil
}

func BindToInterfaceIndexFunc(interfaceIndexFunc func(network, address string) int) Func {
	return nil
}
