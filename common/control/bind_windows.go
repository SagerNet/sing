package control

import (
	"encoding/binary"
	"net"
	"net/netip"
	"syscall"
	"unsafe"

	E "github.com/sagernet/sing/common/exceptions"
)

const (
	IP_UNICAST_IF   = 31
	IPV6_UNICAST_IF = 31
)

func NewBindManager() BindManager {
	return &simpleBindManager{
		interfaceIndexByName: make(map[string]int),
	}
}

func bind4(handle syscall.Handle, ifaceIdx int) error {
	var bytes [4]byte
	binary.BigEndian.PutUint32(bytes[:], uint32(ifaceIdx))
	idx := *(*uint32)(unsafe.Pointer(&bytes[0]))
	return syscall.SetsockoptInt(handle, syscall.IPPROTO_IP, IP_UNICAST_IF, int(idx))
}

func bind6(handle syscall.Handle, ifaceIdx int) error {
	return syscall.SetsockoptInt(handle, syscall.IPPROTO_IPV6, IPV6_UNICAST_IF, ifaceIdx)
}

func bindInterfaceIndex(network string, address string, conn syscall.RawConn, interfaceIndex int) error {
	ipStr, _, err := net.SplitHostPort(address)
	if err == nil {
		if ip, err := netip.ParseAddr(ipStr); err == nil && !ip.IsGlobalUnicast() {
			return err
		}
	}
	var innerErr error
	err = conn.Control(func(fd uintptr) {
		handle := syscall.Handle(fd)
		// handle ip empty, e.g. net.Listen("udp", ":0")
		if ipStr == "" {
			innerErr = bind4(handle, interfaceIndex)
			if innerErr != nil {
				return
			}
			// try bind ipv6, if failed, ignore. it's a workaround for windows disable interface ipv6
			bind6(handle, interfaceIndex)
			return
		}

		switch network {
		case "tcp4", "udp4", "ip4":
			innerErr = bind4(handle, interfaceIndex)
		case "tcp6", "udp6":
			innerErr = bind6(handle, interfaceIndex)
		}
	})
	return E.Errors(innerErr, err)
}

func BindToInterface(manager BindManager, interfaceName string) Func {
	return func(network, address string, conn syscall.RawConn) error {
		index, err := manager.IndexByName(interfaceName)
		if err != nil {
			return err
		}
		return bindInterfaceIndex(network, address, conn, index)
	}
}

func BindToInterfaceFunc(manager BindManager, interfaceNameFunc func() string) Func {
	return func(network, address string, conn syscall.RawConn) error {
		interfaceName := interfaceNameFunc()
		if interfaceName == "" {
			return nil
		}
		index, err := manager.IndexByName(interfaceName)
		if err != nil {
			return err
		}
		return bindInterfaceIndex(network, address, conn, index)
	}
}

func BindToInterfaceIndexFunc(interfaceIndexFunc func() int) Func {
	return func(network, address string, conn syscall.RawConn) error {
		index := interfaceIndexFunc()
		if index == -1 {
			return nil
		}
		return bindInterfaceIndex(network, address, conn, index)
	}
}
