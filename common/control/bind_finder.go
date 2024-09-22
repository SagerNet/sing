package control

import (
	"net"
	"net/netip"
)

type InterfaceFinder interface {
	Update() error
	Interfaces() []Interface
	InterfaceIndexByName(name string) (int, error)
	InterfaceNameByIndex(index int) (string, error)
	InterfaceByAddr(addr netip.Addr) (*Interface, error)
}

type Interface struct {
	Index        int
	MTU          int
	Name         string
	Addresses    []netip.Prefix
	HardwareAddr net.HardwareAddr
}
