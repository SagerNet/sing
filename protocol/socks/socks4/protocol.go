package socks4

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/netip"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
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
	if dstIP[0] == 0 && dstIP[1] == 0 && dstIP[2] == 0 && dstIP[3] != 0 {
		readHostName = true
	} else {
		request.Destination.Addr = netip.AddrFrom4(dstIP)
	}
	request.Username, err = readString(reader)
	if readHostName {
		request.Destination.Fqdn, err = readString(reader)
		request.Destination = M.ParseSocksaddrHostPort(request.Destination.Fqdn, request.Destination.Port)
	}
	return
}

func WriteRequest(writer io.Writer, request Request) error {
	var requestLen int
	requestLen += 1 // version
	requestLen += 1 // command
	requestLen += 2 // port
	requestLen += 4 // ip
	requestLen += 1 // NUL
	if !request.Destination.IsIPv4() {
		requestLen += len(request.Destination.AddrString()) + 1
	}
	if request.Username != "" {
		requestLen += len(request.Username)
	}

	buffer := buf.NewSize(requestLen)
	defer buffer.Release()

	common.Must(
		buffer.WriteByte(Version),
		buffer.WriteByte(request.Command),
		binary.Write(buffer, binary.BigEndian, request.Destination.Port),
	)
	if request.Destination.IsIPv4() {
		common.Must1(buffer.Write(request.Destination.Addr.AsSlice()))
	} else {
		// 0.0.0.X
		common.Must(buffer.WriteZeroN(3))
		common.Must(buffer.WriteByte(1))
	}
	if request.Username != "" {
		common.Must1(buffer.WriteString(request.Username))
	}
	common.Must(buffer.WriteZero())
	if !request.Destination.IsIPv4() {
		common.Must1(buffer.WriteString(request.Destination.AddrString()))
		common.Must(buffer.WriteZero())
	}
	return rw.WriteBytes(writer, buffer.Bytes())
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
	if version != 0 {
		err = E.New("excepted socks4 response version 0, got ", version)
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
	buffer := buf.NewSize(8)
	defer buffer.Release()
	common.Must(
		buffer.WriteByte(0),
		buffer.WriteByte(response.ReplyCode),
		binary.Write(buffer, binary.BigEndian, response.Destination.Port),
		common.Error(buffer.Write(response.Destination.Addr.AsSlice())),
	)
	return rw.WriteBytes(writer, buffer.Bytes())
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
