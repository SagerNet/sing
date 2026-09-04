//go:build windows

package winiphlpapi

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"os"
	"slices"
	"syscall"
	"time"
	"unsafe"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/control"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"golang.org/x/sys/windows"
)

func LoadEStats() error {
	err := modiphlpapi.Load()
	if err != nil {
		return err
	}
	err = procGetTcpTable.Find()
	if err != nil {
		return err
	}
	err = procGetTcp6Table.Find()
	if err != nil {
		return err
	}
	err = procGetPerTcp6ConnectionEStats.Find()
	if err != nil {
		return err
	}
	err = procGetPerTcp6ConnectionEStats.Find()
	if err != nil {
		return err
	}
	err = procSetPerTcpConnectionEStats.Find()
	if err != nil {
		return err
	}
	err = procSetPerTcp6ConnectionEStats.Find()
	if err != nil {
		return err
	}
	return nil
}

func LoadExtendedTable() error {
	err := modiphlpapi.Load()
	if err != nil {
		return err
	}
	err = procGetExtendedTcpTable.Find()
	if err != nil {
		return err
	}
	err = procGetExtendedUdpTable.Find()
	if err != nil {
		return err
	}
	err = procGetOwnerModuleFromTcpEntry.Find()
	if err != nil {
		return err
	}
	err = procGetOwnerModuleFromTcp6Entry.Find()
	if err != nil {
		return err
	}
	err = procGetOwnerModuleFromUdpEntry.Find()
	if err != nil {
		return err
	}
	err = procGetOwnerModuleFromUdp6Entry.Find()
	if err != nil {
		return err
	}
	// GetOwnerModuleFrom*Entry resolves service tags through advapi32 without loading it and fails with ERROR_MOD_NOT_FOUND until the process has.
	return procI_QueryTagInformation.Find()
}

type SocketOwner struct {
	Pid         uint32
	ServiceName string
}

func FindPid(network string, source netip.AddrPort) (uint32, error) {
	owner, err := FindSocketOwner(network, source)
	if err != nil {
		return 0, err
	}
	return owner.Pid, nil
}

func FindSocketOwner(network string, source netip.AddrPort) (SocketOwner, error) {
	switch N.NetworkName(network) {
	case N.NetworkTCP:
		if source.Addr().Is4() {
			tcpTable, err := GetExtendedTcpTableOwnerModule()
			if err != nil {
				return SocketOwner{}, err
			}
			index := slices.IndexFunc(tcpTable, func(row MibTcpRowOwnerModule) bool {
				return source == netip.AddrPortFrom(DwordToAddr(row.DwLocalAddr), DwordToPort(row.DwLocalPort))
			})
			if index != -1 {
				row := &tcpTable[index]
				return socketOwner(row.DwOwningPid, row.OwningModuleInfo[0], func() (*TcpipOwnerModuleBasicInfo, error) {
					return GetOwnerModuleFromTcpEntry(row)
				}), nil
			}
		} else {
			tcpTable, err := GetExtendedTcp6TableOwnerModule()
			if err != nil {
				return SocketOwner{}, err
			}
			index := slices.IndexFunc(tcpTable, func(row MibTcp6RowOwnerModule) bool {
				return source == netip.AddrPortFrom(netip.AddrFrom16(row.UcLocalAddr), DwordToPort(row.DwLocalPort))
			})
			if index != -1 {
				row := &tcpTable[index]
				return socketOwner(row.DwOwningPid, row.OwningModuleInfo[0], func() (*TcpipOwnerModuleBasicInfo, error) {
					return GetOwnerModuleFromTcp6Entry(row)
				}), nil
			}
		}
	case N.NetworkUDP:
		if source.Addr().Is4() {
			udpTable, err := GetExtendedUdpTableOwnerModule()
			if err != nil {
				return SocketOwner{}, err
			}
			index := slices.IndexFunc(udpTable, func(row MibUdpRowOwnerModule) bool {
				return source == netip.AddrPortFrom(DwordToAddr(row.DwLocalAddr), DwordToPort(row.DwLocalPort)) ||
					DwordToAddr(row.DwLocalAddr) == netip.IPv4Unspecified() && source.Port() == DwordToPort(row.DwLocalPort)
			})
			if index != -1 {
				row := &udpTable[index]
				return socketOwner(row.DwOwningPid, row.OwningModuleInfo[0], func() (*TcpipOwnerModuleBasicInfo, error) {
					return GetOwnerModuleFromUdpEntry(row)
				}), nil
			}
		} else {
			udpTable, err := GetExtendedUdp6TableOwnerModule()
			if err != nil {
				return SocketOwner{}, err
			}
			index := slices.IndexFunc(udpTable, func(row MibUdp6RowOwnerModule) bool {
				return source == netip.AddrPortFrom(netip.AddrFrom16(row.UcLocalAddr), DwordToPort(row.DwLocalPort)) ||
					netip.AddrFrom16(row.UcLocalAddr) == netip.IPv6Unspecified() && source.Port() == DwordToPort(row.DwLocalPort)
			})
			if index != -1 {
				row := &udpTable[index]
				return socketOwner(row.DwOwningPid, row.OwningModuleInfo[0], func() (*TcpipOwnerModuleBasicInfo, error) {
					return GetOwnerModuleFromUdp6Entry(row)
				}), nil
			}
		}
	}
	return SocketOwner{}, E.New("process not found for ", source)
}

func socketOwner(pid uint32, serviceTag uint64, queryModule func() (*TcpipOwnerModuleBasicInfo, error)) SocketOwner {
	owner := SocketOwner{Pid: pid}
	if serviceTag == 0 {
		return owner
	}
	moduleInfo, err := queryModule()
	if err != nil {
		return owner
	}
	owner.ServiceName = moduleInfo.ModuleName
	return owner
}

func WriteAndWaitAck(ctx context.Context, conn net.Conn, payload []byte) error {
	syscallConn, isSyscallConn := common.Cast[syscall.Conn](conn)
	if !isSyscallConn {
		return writeAndWaitAckEStats(ctx, conn, payload)
	}
	rawConn, err := syscallConn.SyscallConn()
	if err != nil {
		return writeAndWaitAckEStats(ctx, conn, payload)
	}
	tcpInfo, err := control.Raw0(rawConn, GetTcpInfo)
	if err != nil {
		if E.IsMulti(err, windows.WSAEOPNOTSUPP, windows.WSAEINVAL) {
			return writeAndWaitAckEStats(ctx, conn, payload)
		}
		return os.NewSyscallError("WSAIoctl", err)
	}
	bytesOutBefore := tcpInfo.BytesOut
	_, err = conn.Write(payload)
	if err != nil {
		return err
	}
	return control.Raw(rawConn, func(fd uintptr) error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			tcpInfo, err = GetTcpInfo(fd)
			if err != nil {
				return os.NewSyscallError("WSAIoctl", err)
			}
			if tcpInfo.BytesOut >= bytesOutBefore+uint64(len(payload)) && tcpInfo.BytesInFlight == 0 {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
}

func writeAndWaitAckEStats(ctx context.Context, conn net.Conn, payload []byte) error {
	source := M.AddrPortFromNet(conn.LocalAddr())
	destination := M.AddrPortFromNet(conn.RemoteAddr())
	if source.Addr().Is4() {
		tcpTable, err := GetTcpTable()
		if err != nil {
			return err
		}
		rowIndex := slices.IndexFunc(tcpTable, func(row MibTcpRow) bool {
			return source == netip.AddrPortFrom(DwordToAddr(row.DwLocalAddr), DwordToPort(row.DwLocalPort)) &&
				destination == netip.AddrPortFrom(DwordToAddr(row.DwRemoteAddr), DwordToPort(row.DwRemotePort))
		})
		if rowIndex == -1 {
			rowIndex = slices.IndexFunc(tcpTable, func(row MibTcpRow) bool {
				return source == netip.AddrPortFrom(DwordToAddr(row.DwLocalAddr), DwordToPort(row.DwLocalPort)) ||
					destination == netip.AddrPortFrom(DwordToAddr(row.DwRemoteAddr), DwordToPort(row.DwRemotePort))
			})
		}
		if rowIndex == -1 {
			return E.New("row not found for: ", source)
		}
		tcpRow := &tcpTable[rowIndex]
		err = SetPerTcpConnectionEStatsSendBuffer(tcpRow, &TcpEstatsSendBuffRwV0{
			EnableCollection: true,
		})
		if err != nil {
			return os.NewSyscallError("SetPerTcpConnectionEStatsSendBufferV0", err)
		}
		defer SetPerTcpConnectionEStatsSendBuffer(tcpRow, &TcpEstatsSendBuffRwV0{
			EnableCollection: false,
		})
		_, err = conn.Write(payload)
		if err != nil {
			return err
		}
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			eStstsSendBuffer, err := GetPerTcpConnectionEStatsSendBuffer(tcpRow)
			if err != nil {
				return err
			}
			if eStstsSendBuffer.CurRetxQueue == 0 {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
		}
	} else {
		tcpTable, err := GetTcp6Table()
		if err != nil {
			return err
		}
		rowIndex := slices.IndexFunc(tcpTable, func(row MibTcp6Row) bool {
			return source == netip.AddrPortFrom(netip.AddrFrom16(row.LocalAddr), DwordToPort(row.LocalPort)) &&
				destination == netip.AddrPortFrom(netip.AddrFrom16(row.RemoteAddr), DwordToPort(row.RemotePort))
		})
		if rowIndex == -1 {
			rowIndex = slices.IndexFunc(tcpTable, func(row MibTcp6Row) bool {
				return source == netip.AddrPortFrom(netip.AddrFrom16(row.LocalAddr), DwordToPort(row.LocalPort)) ||
					destination == netip.AddrPortFrom(netip.AddrFrom16(row.RemoteAddr), DwordToPort(row.RemotePort))
			})
		}
		if rowIndex == -1 {
			return E.New("row not found for: ", source)
		}
		tcpRow := &tcpTable[rowIndex]
		err = SetPerTcp6ConnectionEStatsSendBuffer(tcpRow, &TcpEstatsSendBuffRwV0{
			EnableCollection: true,
		})
		if err != nil {
			return os.NewSyscallError("SetPerTcpConnectionEStatsSendBufferV0", err)
		}
		defer SetPerTcp6ConnectionEStatsSendBuffer(tcpRow, &TcpEstatsSendBuffRwV0{
			EnableCollection: false,
		})
		_, err = conn.Write(payload)
		if err != nil {
			return err
		}
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			eStstsSendBuffer, err := GetPerTcp6ConnectionEStatsSendBuffer(tcpRow)
			if err != nil {
				return err
			}
			if eStstsSendBuffer.CurRetxQueue == 0 {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func DwordToAddr(addr uint32) netip.Addr {
	return netip.AddrFrom4(*(*[4]byte)(unsafe.Pointer(&addr)))
}

func DwordToPort(dword uint32) uint16 {
	return binary.BigEndian.Uint16((*[4]byte)(unsafe.Pointer(&dword))[:])
}
