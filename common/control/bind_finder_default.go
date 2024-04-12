package control

import (
	"net"
	"net/netip"
	_ "unsafe"

	"github.com/sagernet/sing/common"
	M "github.com/sagernet/sing/common/metadata"
)

type DefaultInterfaceFinder struct {
	interfaces []Interface
}

func NewDefaultInterfaceFinder() *DefaultInterfaceFinder {
	return &DefaultInterfaceFinder{}
}

func (f *DefaultInterfaceFinder) Update() error {
	netIfs, err := net.Interfaces()
	if err != nil {
		return err
	}
	interfaces := make([]Interface, 0, len(netIfs))
	for _, netIf := range netIfs {
		ifAddrs, err := netIf.Addrs()
		if err != nil {
			return err
		}
		interfaces = append(interfaces, Interface{
			Index:     netIf.Index,
			MTU:       netIf.MTU,
			Name:      netIf.Name,
			Addresses: common.Map(ifAddrs, M.PrefixFromNet),
		})
	}
	f.interfaces = interfaces
	return nil
}

func (f *DefaultInterfaceFinder) UpdateInterfaces(interfaces []Interface) {
	f.interfaces = interfaces
}

func (f *DefaultInterfaceFinder) InterfaceIndexByName(name string) (int, error) {
	for _, netInterface := range f.interfaces {
		if netInterface.Name == name {
			return netInterface.Index, nil
		}
	}
	netInterface, err := net.InterfaceByName(name)
	if err != nil {
		return 0, err
	}
	f.Update()
	return netInterface.Index, nil
}

func (f *DefaultInterfaceFinder) InterfaceNameByIndex(index int) (string, error) {
	for _, netInterface := range f.interfaces {
		if netInterface.Index == index {
			return netInterface.Name, nil
		}
	}
	netInterface, err := net.InterfaceByIndex(index)
	if err != nil {
		return "", err
	}
	f.Update()
	return netInterface.Name, nil
}

//go:linkname errNoSuchInterface net.errNoSuchInterface
var errNoSuchInterface error

func (f *DefaultInterfaceFinder) InterfaceByAddr(addr netip.Addr) (*Interface, error) {
	for _, netInterface := range f.interfaces {
		for _, prefix := range netInterface.Addresses {
			if prefix.Contains(addr) {
				return &netInterface, nil
			}
		}
	}
	err := f.Update()
	if err != nil {
		return nil, err
	}
	for _, netInterface := range f.interfaces {
		for _, prefix := range netInterface.Addresses {
			if prefix.Contains(addr) {
				return &netInterface, nil
			}
		}
	}
	return nil, &net.OpError{Op: "route", Net: "ip+net", Source: nil, Addr: &net.IPAddr{IP: addr.AsSlice()}, Err: errNoSuchInterface}
}
