package metadata

import (
	"encoding/binary"
	"io"
	"net/netip"

	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/rw"
)

type SerializerOption func(*Serializer)

func AddressFamilyByte(b byte, f Family) SerializerOption {
	return func(s *Serializer) {
		s.familyMap[b] = f
		s.familyByteMap[f] = b
	}
}

func PortThenAddress() SerializerOption {
	return func(s *Serializer) {
		s.portFirst = true
	}
}

type Serializer struct {
	familyMap     map[byte]Family
	familyByteMap map[Family]byte
	portFirst     bool
}

func NewSerializer(options ...SerializerOption) *Serializer {
	s := &Serializer{
		familyMap:     make(map[byte]Family),
		familyByteMap: make(map[Family]byte),
	}
	for _, option := range options {
		option(s)
	}
	return s
}

func (s *Serializer) WriteAddress(writer io.Writer, addr Socksaddr) error {
	err := rw.WriteByte(writer, s.familyByteMap[addr.Family()])
	if err != nil {
		return err
	}
	if addr.Addr.IsValid() {
		err = rw.WriteBytes(writer, addr.Addr.AsSlice())
	} else {
		err = WriteString(writer, "fqdn", addr.Fqdn)
	}
	return err
}

func (s *Serializer) AddressLen(addr Socksaddr) int {
	switch addr.Family() {
	case AddressFamilyIPv4:
		return 5
	case AddressFamilyIPv6:
		return 17
	default:
		return 2 + len(addr.Fqdn)
	}
}

func (s *Serializer) WritePort(writer io.Writer, port uint16) error {
	return binary.Write(writer, binary.BigEndian, port)
}

func (s *Serializer) WriteAddrPort(writer io.Writer, destination Socksaddr) error {
	var err error
	if !s.portFirst {
		err = s.WriteAddress(writer, destination)
	} else {
		err = s.WritePort(writer, destination.Port)
	}
	if err != nil {
		return err
	}
	if s.portFirst {
		err = s.WriteAddress(writer, destination)
	} else {
		err = s.WritePort(writer, destination.Port)
	}
	return err
}

func (s *Serializer) AddrPortLen(destination Socksaddr) int {
	return s.AddressLen(destination) + 2
}

func (s *Serializer) ReadAddress(reader io.Reader) (Socksaddr, error) {
	af, err := rw.ReadByte(reader)
	if err != nil {
		return Socksaddr{}, err
	}
	family := s.familyMap[af]
	switch family {
	case AddressFamilyFqdn:
		fqdn, err := ReadString(reader)
		if err != nil {
			return Socksaddr{}, E.Cause(err, "read fqdn")
		}
		return Socksaddr{
			Fqdn: fqdn,
		}, nil
	default:
		switch family {
		case AddressFamilyIPv4:
			var addr [4]byte
			err = common.Error(reader.Read(addr[:]))
			if err != nil {
				return Socksaddr{}, E.Cause(err, "read ipv4 address")
			}
			return Socksaddr{Addr: netip.AddrFrom4(addr)}, nil
		case AddressFamilyIPv6:
			var addr [16]byte
			err = common.Error(reader.Read(addr[:]))
			if err != nil {
				return Socksaddr{}, E.Cause(err, "read ipv6 address")
			}
			return Socksaddr{Addr: netip.AddrFrom16(addr)}, nil
		default:
			return Socksaddr{}, E.New("unknown address family: ", af)
		}
	}
}

func (s *Serializer) ReadPort(reader io.Reader) (uint16, error) {
	port, err := rw.ReadBytes(reader, 2)
	if err != nil {
		return 0, E.Cause(err, "read port")
	}
	return binary.BigEndian.Uint16(port), nil
}

func (s *Serializer) ReadAddrPort(reader io.Reader) (destination Socksaddr, err error) {
	var addr Socksaddr
	var port uint16
	if !s.portFirst {
		addr, err = s.ReadAddress(reader)
	} else {
		port, err = s.ReadPort(reader)
	}
	if err != nil {
		return
	}
	if s.portFirst {
		addr, err = s.ReadAddress(reader)
	} else {
		port, err = s.ReadPort(reader)
	}
	if err != nil {
		return
	}
	addr.Port = port
	return addr, nil
}

func ReadString(reader io.Reader) (string, error) {
	strLen, err := rw.ReadByte(reader)
	if err != nil {
		return common.EmptyString, err
	}
	return rw.ReadString(reader, int(strLen))
}

func WriteString(writer io.Writer, op string, str string) error {
	strLen := len(str)
	if strLen > 255 {
		return &StringTooLongException{op, strLen}
	}
	err := rw.WriteByte(writer, byte(strLen))
	if err != nil {
		return err
	}
	return rw.WriteString(writer, str)
}
