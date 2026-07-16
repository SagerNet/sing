package control

import (
	"net"
	"net/netip"
	"sync"

	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/x/list"
)

var _ InterfaceFinder = (*DefaultInterfaceFinder)(nil)

type DefaultInterfaceFinder struct {
	access         sync.RWMutex
	interfaces     []Interface
	callbackAccess sync.Mutex
	callbacks      list.List[InterfaceUpdateCallback]
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
		var iif Interface
		iif, err = InterfaceFromNet(netIf)
		if err != nil {
			return err
		}
		interfaces = append(interfaces, iif)
	}
	f.UpdateInterfaces(interfaces)
	return nil
}

func (f *DefaultInterfaceFinder) UpdateInterfaces(interfaces []Interface) {
	f.access.Lock()
	f.interfaces = interfaces
	f.access.Unlock()
	f.callbackAccess.Lock()
	callbacks := f.callbacks.Array()
	f.callbackAccess.Unlock()
	for _, callback := range callbacks {
		callback(interfaces)
	}
}

func (f *DefaultInterfaceFinder) Interfaces() []Interface {
	f.access.RLock()
	defer f.access.RUnlock()
	return f.interfaces
}

func (f *DefaultInterfaceFinder) RegisterInterfaceUpdateCallback(callback InterfaceUpdateCallback) *list.Element[InterfaceUpdateCallback] {
	f.callbackAccess.Lock()
	defer f.callbackAccess.Unlock()
	return f.callbacks.PushBack(callback)
}

func (f *DefaultInterfaceFinder) UnregisterInterfaceUpdateCallback(element *list.Element[InterfaceUpdateCallback]) {
	f.callbackAccess.Lock()
	defer f.callbackAccess.Unlock()
	f.callbacks.Remove(element)
}

func (f *DefaultInterfaceFinder) ByName(name string) (*Interface, error) {
	f.access.RLock()
	for _, netInterface := range f.interfaces {
		if netInterface.Name == name {
			f.access.RUnlock()
			return &netInterface, nil
		}
	}
	f.access.RUnlock()
	_, err := net.InterfaceByName(name)
	if err == nil {
		err = f.Update()
		if err != nil {
			return nil, err
		}
		return f.ByName(name)
	}
	return nil, &net.OpError{Op: "route", Net: "ip+net", Source: nil, Addr: &net.IPAddr{IP: nil}, Err: E.New("no such network interface")}
}

func (f *DefaultInterfaceFinder) ByIndex(index int) (*Interface, error) {
	f.access.RLock()
	for _, netInterface := range f.interfaces {
		if netInterface.Index == index {
			f.access.RUnlock()
			return &netInterface, nil
		}
	}
	f.access.RUnlock()
	_, err := net.InterfaceByIndex(index)
	if err == nil {
		err = f.Update()
		if err != nil {
			return nil, err
		}
		return f.ByIndex(index)
	}
	return nil, &net.OpError{Op: "route", Net: "ip+net", Source: nil, Addr: &net.IPAddr{IP: nil}, Err: E.New("no such network interface")}
}

func (f *DefaultInterfaceFinder) ByAddr(addr netip.Addr) (*Interface, error) {
	f.access.RLock()
	defer f.access.RUnlock()
	for _, netInterface := range f.interfaces {
		if netInterface.Flags&net.FlagRunning == 0 {
			continue
		}
		for _, prefix := range netInterface.Addresses {
			if prefix.Addr() == addr {
				return &netInterface, nil
			}
		}
	}
	for _, netInterface := range f.interfaces {
		if netInterface.Flags&net.FlagRunning == 0 {
			continue
		}
		for _, prefix := range netInterface.Addresses {
			if prefix.Contains(addr) {
				return &netInterface, nil
			}
		}
	}
	return nil, &net.OpError{Op: "route", Net: "ip+net", Source: nil, Addr: &net.IPAddr{IP: addr.AsSlice()}, Err: E.New("no such network interface")}
}
