package control

import (
	"net/netip"
)

type InterfaceFinder interface {
	InterfaceIndexByName(name string) (int, error)
	InterfaceNameByIndex(index int) (string, error)
	InterfaceByAddr(addr netip.Addr) (*Interface, error)
}

type Interface struct {
	Index     int
	MTU       int
	Name      string
	Addresses []netip.Prefix
}
