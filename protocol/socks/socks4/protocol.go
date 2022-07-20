package socks4

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/netip"
	"os"

	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/rw"
)

const (
	Version byte = 4

	CommandConnect byte = 1
	CommandBind    byte = 2

	ReplyCodeGranted                     byte = 90
	ReplyCodeRejectedOrFailed            byte = 91
	ReplyCodeCannotConnectToIdentd       byte = 92
	ReplyCodeIdentdReportDifferentUserID byte = 93
)

type Request struct {
	Command     byte
	Destination M.Socksaddr
	Username    string
}

func ReadRequest(reader io.Reader) (request Request, err error) {
	version, err := rw.ReadByte(reader)
	if err != nil {
		return
	}
	if version != 4 {
		err = E.New("excepted socks version 4, got ", version)
		return
	}
	return ReadRequest0(reader)
}

func ReadRequest0(reader io.Reader) (request Request, err error) {
	request.Command, err = rw.ReadByte(reader)
	if err != nil {
		return
	}
	err = binary.Read(reader, binary.BigEndian, &request.Destination.Port)
	if err != nil {
		return
	}
	var dstIP [4]byte
	_, err = io.ReadFull(reader, dstIP[:])
	if err != nil {
		return
	}
	var readHostName bool
	if dstIP[0] == 0 && dstIP[1] == 0 && dstIP[2] == 0 {
		readHostName = true
	} else {
		request.Destination.Addr = netip.AddrFrom4(dstIP)
	}
	request.Username, err = readString(reader)
	if readHostName {
		request.Destination.Fqdn, err = readString(reader)
	}
	return
}

func WriteRequest(writer io.Writer, request Request) error {
	if request.Command != CommandConnect && request.Command != CommandBind {
		return os.ErrInvalid
	}
	_, err := writer.Write([]byte{Version, request.Command})
	if err != nil {
		return err
	}
	err = binary.Write(writer, binary.BigEndian, request.Destination.Port)
	if err != nil {
		return err
	}
	if request.Destination.IsIPv4() {
		dstIP := request.Destination.Addr.As4()
		_, err = writer.Write(dstIP[:])
		if err != nil {
			return err
		}
	} else {
		err = rw.WriteZeroN(writer, 4)
		if err != nil {
			return err
		}
		_, err = writer.Write([]byte(request.Destination.AddrString()))
		if err != nil {
			return err
		}
		err = rw.WriteZero(writer)
		if err != nil {
			return err
		}
	}
	if request.Username != "" {
		_, err = writer.Write([]byte(request.Username))
		if err != nil {
			return err
		}
	}
	return rw.WriteZero(writer)
}

type Response struct {
	ReplyCode   byte
	Destination M.Socksaddr
}

func ReadResponse(reader io.Reader) (response Response, err error) {
	version, err := rw.ReadByte(reader)
	if err != nil {
		return
	}
	if version != 4 {
		err = E.New("excepted socks version 4, got ", version)
		return
	}
	response.ReplyCode, err = rw.ReadByte(reader)
	if err != nil {
		return
	}
	err = binary.Read(reader, binary.BigEndian, &response.Destination.Port)
	if err != nil {
		return
	}
	var dstIP [4]byte
	_, err = io.ReadFull(reader, dstIP[:])
	if err != nil {
		return
	}
	response.Destination.Addr = netip.AddrFrom4(dstIP)
	return
}

func WriteResponse(writer io.Writer, response Response) error {
	_, err := writer.Write([]byte{Version, response.ReplyCode})
	if err != nil {
		return err
	}
	err = binary.Write(writer, binary.BigEndian, response.Destination.Port)
	if err != nil {
		return err
	}
	dstIP := response.Destination.Addr.As4()
	return rw.WriteBytes(writer, dstIP[:])
}

func readString(reader io.Reader) (string, error) {
	buffer := bytes.Buffer{}
	for {
		b, err := rw.ReadByte(reader)
		if err != nil {
			return "", err
		}
		if b == 0 {
			break
		}
		buffer.WriteByte(b)
	}
	return buffer.String(), nil
}
