package metadata

import "net/netip"

func IsDomainName(domain string) bool {
	l := len(domain)
	if l == 0 || l > 254 {
		return false
	}
	if _, err := netip.ParseAddr(unwrapIPv6Address(domain)); err == nil {
		return false
	}
	labelLength := 0
	for i := 0; i < l; i++ {
		c := domain[i]
		if c == '.' {
			if labelLength == 0 {
				return false
			}
			labelLength = 0
			continue
		}
		if c == 0 {
			return false
		}
		labelLength++
		if labelLength > 63 {
			return false
		}
	}
	return labelLength > 0
}
