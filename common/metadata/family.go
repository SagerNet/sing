package metadata

type Family byte

const (
	AddressFamilyIPv4 Family = iota
	AddressFamilyIPv6
	AddressFamilyFqdn
)

func (af Family) IsIPv4() bool {
	return af == AddressFamilyIPv4
}

func (af Family) IsIPv6() bool {
	return af == AddressFamilyIPv6
}

func (af Family) IsIP() bool {
	return af != AddressFamilyFqdn
}

func (af Family) IsFqdn() bool {
	return af == AddressFamilyFqdn
}

type FamilyParser func(byte) byte
