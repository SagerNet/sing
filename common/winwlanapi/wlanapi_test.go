//go:build windows

package winwlanapi

import (
	"testing"
)

func TestOpenHandle(t *testing.T) {
	handle, err := OpenHandle()
	if err != nil {
		t.Skipf("WLAN service not available: %v", err)
	}
	defer CloseHandle(handle)

	if handle == 0 {
		t.Error("expected non-zero handle")
	}
}

func TestEnumInterfaces(t *testing.T) {
	handle, err := OpenHandle()
	if err != nil {
		t.Skipf("WLAN service not available: %v", err)
	}
	defer CloseHandle(handle)

	interfaces, err := EnumInterfaces(handle)
	if err != nil {
		t.Fatalf("EnumInterfaces failed: %v", err)
	}

	t.Logf("Found %d WLAN interface(s)", len(interfaces))
	for i, iface := range interfaces {
		description := windowsUTF16ToString(iface.InterfaceDescription[:])
		t.Logf("Interface %d: %s (state=%d)", i, description, iface.InterfaceState)
	}
}

func TestQueryCurrentConnection(t *testing.T) {
	handle, err := OpenHandle()
	if err != nil {
		t.Skipf("WLAN service not available: %v", err)
	}
	defer CloseHandle(handle)

	interfaces, err := EnumInterfaces(handle)
	if err != nil {
		t.Fatalf("EnumInterfaces failed: %v", err)
	}

	if len(interfaces) == 0 {
		t.Skip("no WLAN interfaces available")
	}

	for _, iface := range interfaces {
		if iface.InterfaceState != InterfaceStateConnected {
			continue
		}

		guid := iface.InterfaceGUID
		attrs, err := QueryCurrentConnection(handle, &guid)
		if err != nil {
			t.Errorf("QueryCurrentConnection failed: %v", err)
			continue
		}

		ssidLen := attrs.AssociationAttributes.SSID.Length
		if ssidLen > 0 && ssidLen <= Dot11SSIDMaxLength {
			ssid := string(attrs.AssociationAttributes.SSID.SSID[:ssidLen])
			bssid := attrs.AssociationAttributes.BSSID
			t.Logf("Connected to SSID: %q, BSSID: %02X:%02X:%02X:%02X:%02X:%02X",
				ssid, bssid[0], bssid[1], bssid[2], bssid[3], bssid[4], bssid[5])
		}
		return
	}

	t.Log("no connected WLAN interface found")
}

func TestCloseHandle(t *testing.T) {
	handle, err := OpenHandle()
	if err != nil {
		t.Skipf("WLAN service not available: %v", err)
	}

	err = CloseHandle(handle)
	if err != nil {
		t.Errorf("CloseHandle failed: %v", err)
	}

	// closing again should fail
	err = CloseHandle(handle)
	if err == nil {
		t.Error("expected error when closing already closed handle")
	}
}

func windowsUTF16ToString(s []uint16) string {
	for i, c := range s {
		if c == 0 {
			return string(utf16ToRunes(s[:i]))
		}
	}
	return string(utf16ToRunes(s))
}

func utf16ToRunes(s []uint16) []rune {
	runes := make([]rune, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= 0xD800 && s[i] <= 0xDBFF && i+1 < len(s) && s[i+1] >= 0xDC00 && s[i+1] <= 0xDFFF {
			runes = append(runes, rune((int(s[i])-0xD800)<<10+(int(s[i+1])-0xDC00)+0x10000))
			i++
		} else {
			runes = append(runes, rune(s[i]))
		}
	}
	return runes
}
