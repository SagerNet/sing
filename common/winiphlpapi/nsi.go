//go:build windows

package winiphlpapi

import (
	"encoding/binary"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NsiGetParameter is undocumented; the TCP connection table index and the key and static
// parameter layouts below come from Wine (include/wine/nsi.h). The connection table is the
// one GetExtendedTcpTable reads for TCP_TABLE_*_CONNECTIONS, keyed by
// {SOCKADDR_INET local; SOCKADDR_INET remote}, and its static parameter carries the same
// owning pid, create timestamp and owning module info as MIB_TCPROW_OWNER_MODULE.
// The UDP endpoint table rejects parameter reads with ERROR_NOT_SUPPORTED.

const (
	nsiStoreActive        = 1
	nsiParameterStatic    = 2
	nsiTCPConnectionTable = 4
	npiModuleIDTypeGUID   = 1
	sockaddrInetSize      = 28
)

type npiModuleID struct {
	Length uint16
	_      uint16
	Type   uint32
	GUID   windows.GUID
}

var npiTCPModuleID = npiModuleID{
	Length: uint16(unsafe.Sizeof(npiModuleID{})),
	Type:   npiModuleIDTypeGUID,
	GUID: windows.GUID{
		Data1: 0xeb004a03,
		Data2: 0x9b1a,
		Data3: 0x11d4,
		Data4: [8]byte{0x91, 0x23, 0x00, 0x50, 0x04, 0x77, 0x59, 0xbc},
	},
}

type nsiTCPConnectionStatic struct {
	_                [3]uint32
	OwningPid        uint32
	CreateTimestamp  int64
	OwningModuleInfo uint64
}

func nsiGetTCPConnection(source netip.AddrPort, destination netip.AddrPort) (*nsiTCPConnectionStatic, error) {
	var key [2 * sockaddrInetSize]byte
	writeSockaddrInet(key[:sockaddrInetSize], source)
	writeSockaddrInet(key[sockaddrInetSize:], destination)
	var static nsiTCPConnectionStatic
	err := nsiGetParameter(nsiStoreActive, &npiTCPModuleID, nsiTCPConnectionTable, &key[0], uint32(len(key)), nsiParameterStatic, unsafe.Pointer(&static), uint32(unsafe.Sizeof(static)), 0)
	if err != nil {
		return nil, err
	}
	return &static, nil
}

func writeSockaddrInet(buffer []byte, addrPort netip.AddrPort) {
	binary.BigEndian.PutUint16(buffer[2:], addrPort.Port())
	if addrPort.Addr().Is4() {
		binary.NativeEndian.PutUint16(buffer, windows.AF_INET)
		address := addrPort.Addr().As4()
		copy(buffer[4:], address[:])
	} else {
		binary.NativeEndian.PutUint16(buffer, windows.AF_INET6)
		address := addrPort.Addr().As16()
		copy(buffer[8:], address[:])
	}
}
