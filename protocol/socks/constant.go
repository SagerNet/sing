package socks

import (
	"strconv"

	"github.com/sagernet/sing/common/socksaddr"
)

const (
	Version4 byte = 0x04
	Version5 byte = 0x05
)

const (
	AuthTypeNotRequired       byte = 0x00
	AuthTypeGSSAPI            byte = 0x01
	AuthTypeUsernamePassword  byte = 0x02
	AuthTypeNoAcceptedMethods byte = 0xFF
)

const (
	UsernamePasswordVersion1      byte = 0x01
	UsernamePasswordStatusSuccess byte = 0x00
	UsernamePasswordStatusFailure byte = 0x01
)

const (
	CommandConnect      byte = 0x01
	CommandBind         byte = 0x02
	CommandUDPAssociate byte = 0x03
)

type ReplyCode byte

const (
	ReplyCodeSuccess ReplyCode = iota
	ReplyCodeFailure
	ReplyCodeNotAllowed
	ReplyCodeNetworkUnreachable
	ReplyCodeHostUnreachable
	ReplyCodeConnectionRefused
	ReplyCodeTTLExpired
	ReplyCodeUnsupported
	ReplyCodeAddressTypeUnsupported
)

func (code ReplyCode) String() string {
	switch code {
	case ReplyCodeSuccess:
		return "succeeded"
	case ReplyCodeFailure:
		return "general SOCKS server failure"
	case ReplyCodeNotAllowed:
		return "connection not allowed by ruleset"
	case ReplyCodeNetworkUnreachable:
		return "network unreachable"
	case ReplyCodeHostUnreachable:
		return "host unreachable"
	case ReplyCodeConnectionRefused:
		return "connection refused"
	case ReplyCodeTTLExpired:
		return "TTL expired"
	case ReplyCodeUnsupported:
		return "command not supported"
	case ReplyCodeAddressTypeUnsupported:
		return "address type not supported"
	default:
		return "unassigned code: " + strconv.Itoa(int(code))
	}
}

var AddressSerializer = socksaddr.NewSerializer(
	socksaddr.AddressFamilyByte(0x01, socksaddr.AddressFamilyIPv4),
	socksaddr.AddressFamilyByte(0x04, socksaddr.AddressFamilyIPv6),
	socksaddr.AddressFamilyByte(0x03, socksaddr.AddressFamilyFqdn),
)
