//go:build windows

package winiphlpapi

import (
	"net"
	"net/netip"
	"testing"

	M "github.com/sagernet/sing/common/metadata"

	"github.com/stretchr/testify/require"
)

func dialLoopback(t *testing.T, network string, address string) (netip.AddrPort, netip.AddrPort) {
	listener, err := net.Listen(network, address)
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			accepted, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			t.Cleanup(func() { accepted.Close() })
		}
	}()
	conn, err := net.Dial(network, listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return M.AddrPortFromNet(conn.LocalAddr()), M.AddrPortFromNet(conn.RemoteAddr())
}

func TestNSITCPConnectionMatchesTable(t *testing.T) {
	require.NoError(t, LoadExtendedTable())
	require.NoError(t, procNsiGetParameter.Find())
	dialLoopback(t, "tcp4", "127.0.0.1:0")
	dialLoopback(t, "tcp6", "[::1]:0")
	var checked int
	table4, err := GetExtendedTcpTableOwnerModule()
	require.NoError(t, err)
	for _, row := range table4 {
		local := netip.AddrPortFrom(DwordToAddr(row.DwLocalAddr), DwordToPort(row.DwLocalPort))
		remote := netip.AddrPortFrom(DwordToAddr(row.DwRemoteAddr), DwordToPort(row.DwRemotePort))
		connection, lookupErr := nsiGetTCPConnection(local, remote)
		if lookupErr != nil {
			continue
		}
		require.Equal(t, row.DwOwningPid, connection.OwningPid, "%v -> %v", local, remote)
		require.Equal(t, row.LiCreateTimestamp, connection.CreateTimestamp, "%v -> %v", local, remote)
		require.Equal(t, row.OwningModuleInfo[0], connection.OwningModuleInfo, "%v -> %v", local, remote)
		checked++
	}
	table6, err := GetExtendedTcp6TableOwnerModule()
	require.NoError(t, err)
	for _, row := range table6 {
		if row.DwLocalScopeId != 0 || row.DwRemoteScopeId != 0 {
			continue
		}
		local := netip.AddrPortFrom(netip.AddrFrom16(row.UcLocalAddr), DwordToPort(row.DwLocalPort))
		remote := netip.AddrPortFrom(netip.AddrFrom16(row.UcRemoteAddr), DwordToPort(row.DwRemotePort))
		connection, lookupErr := nsiGetTCPConnection(local, remote)
		if lookupErr != nil {
			continue
		}
		require.Equal(t, row.DwOwningPid, connection.OwningPid, "%v -> %v", local, remote)
		require.Equal(t, row.LiCreateTimestamp, connection.CreateTimestamp, "%v -> %v", local, remote)
		require.Equal(t, row.OwningModuleInfo[0], connection.OwningModuleInfo, "%v -> %v", local, remote)
		checked++
	}
	require.GreaterOrEqual(t, checked, 4)
}
