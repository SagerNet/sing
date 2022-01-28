package shadowsocks_test

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"strings"
	"sync"
	"testing"

	vb "github.com/v2fly/v2ray-core/v5/common/buf"
	vn "github.com/v2fly/v2ray-core/v5/common/net"
	vp "github.com/v2fly/v2ray-core/v5/common/protocol"
	vs "github.com/v2fly/v2ray-core/v5/proxy/shadowsocks"
	"sing/common"
	"sing/common/buf"
	"sing/common/crypto"
	"sing/common/socksaddr"
	"sing/protocol/shadowsocks"
)

func TestShadowsocksTCP(t *testing.T) {
	for index := 1; index <= int(vs.CipherType_XCHACHA20); index++ {
		if index == 0 {
			continue
		}
		cipherType := vs.CipherType(index)
		cipher := strings.ReplaceAll(strings.ToLower(cipherType.String()), "_", "-")
		t.Log("Test", cipher, "server")
		testShadowsocksServerTCPWithCipher(t, cipherType, cipher)
		t.Log("Test", cipher, "client")
		testShadowsocksClientTCPWithCipher(t, cipherType, cipher)
	}
}

func testShadowsocksServerTCPWithCipher(t *testing.T, cipherType vs.CipherType, cipherName string) {
	password := "fuck me till the daylight"
	cipher, err := shadowsocks.CreateCipher(cipherName)
	if err != nil {
		t.Log("Skip unsupported method: ", cipherName)
		return
	}
	key := shadowsocks.Key([]byte(password), cipher.KeySize())
	address := socksaddr.AddrFromFqdn("internal.sagernet.org")
	data := crypto.RandomBytes(1024)

	protoAccount := &vs.Account{
		Password:   password,
		CipherType: cipherType,
	}
	memoryAccount, err := protoAccount.AsAccount()
	common.Must(err)
	memoryUser := &vp.MemoryUser{
		Account: memoryAccount,
	}
	account := memoryAccount.(*vs.MemoryAccount)

	client, server := net.Pipe()
	defer common.Close(client, server)

	wg := new(sync.WaitGroup)
	wg.Add(1)

	go func() {
		defer wg.Done()

		req := &vp.RequestHeader{
			Version: vs.Version,
			Command: vp.RequestCommandTCP,
			Address: vn.DomainAddress(address.Fqdn()),
			Port:    443,
			User:    memoryUser,
		}
		writeIv := crypto.RandomBytes(int(account.Cipher.IVSize()))
		writer, err := vs.WriteTCPRequest(req, client, writeIv, nil)
		if err != nil {
			t.Error(err)
			return
		}
		reader, err := vs.ReadTCPResponse(memoryUser, client, nil)
		if err != nil {
			t.Error(err)
			return
		}
		conn := vb.NewConnection(vb.ConnectionOutputMulti(reader), vb.ConnectionInputMulti(writer))
		buffer := vb.New()
		defer buffer.Release()
		buffer.Write(data)
		_, err = conn.Write(buffer.Bytes())
		if err != nil {
			t.Error(err)
			return
		}
		clientRead := make([]byte, 1024)
		_, err = io.ReadFull(conn, clientRead)
		if err != nil {
			t.Error(err)
			return
		}
		if bytes.Compare(clientRead, data) > 0 {
			t.Error("bad response data")
			return
		}
		client.Close()
	}()

	var readIv []byte
	if cipher.IVSize() > 0 {
		readIv = make([]byte, cipher.IVSize())
		_, err = io.ReadFull(server, readIv)
		if err != nil {
			t.Fatal(err)
		}
	}
	reader, err := cipher.NewDecryptionReader(key, readIv, server)
	if err != nil {
		t.Fatal(err)
	}

	addr, port, err := shadowsocks.AddressSerializer.ReadAddressAndPort(reader)
	if err != nil {
		t.Fatal(err)
	}
	if addr != address {
		t.Fatal("bad address")
	}
	if port != 443 {
		t.Fatal("bad port")
	}

	var writeIv []byte
	if cipher.IVSize() > 0 {
		writeIv = crypto.RandomBytes(cipher.IVSize())
		_, err = server.Write(writeIv)
		if err != nil {
			t.Fatal(err)
		}
	}

	serverRead := make([]byte, 1024)
	_, err = io.ReadFull(reader, serverRead)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Compare(serverRead, data) > 0 {
		t.Fatal("bad request data")
	}

	writer, err := cipher.NewEncryptionWriter(key, writeIv, server)
	if err != nil {
		t.Fatal(err)
	}
	buffer := buf.New()
	defer buf.Release(buffer)
	buffer.Write(data)
	_, err = writer.Write(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait()
}

func testShadowsocksClientTCPWithCipher(t *testing.T, cipherType vs.CipherType, cipherName string) {
	password := "fuck me till the daylight"
	cipher, err := shadowsocks.CreateCipher(cipherName)
	if err != nil {
		t.Log("Skip unsupported method: ", cipherName)
		return
	}
	key := shadowsocks.Key([]byte(password), cipher.KeySize())
	address := socksaddr.AddrFromFqdn("internal.sagernet.org")
	data := crypto.RandomBytes(1024)

	protoAccount := &vs.Account{
		Password:   password,
		CipherType: cipherType,
	}
	memoryAccount, err := protoAccount.AsAccount()
	common.Must(err)
	memoryUser := &vp.MemoryUser{
		Account: memoryAccount,
	}
	account := memoryAccount.(*vs.MemoryAccount)

	wg := new(sync.WaitGroup)
	wg.Add(1)

	client, server := net.Pipe()
	defer common.Close(client, server)

	go func() {
		defer wg.Done()

		session, reader, err := vs.ReadTCPSession(memoryUser, server, nil)
		if err != nil {
			t.Error(err)
			return
		}
		if !session.Address.Family().IsDomain() || session.Address.Domain() != address.Fqdn() {
			t.Error("bad request address")
			return
		}
		if session.Port != 443 {
			t.Error("bad request port")
			return
		}
		writeIv := crypto.RandomBytes(int(account.Cipher.IVSize()))
		writer, err := vs.WriteTCPResponse(session, server, writeIv, nil)
		if err != nil {
			t.Error(err)
			return
		}
		conn := vb.NewConnection(vb.ConnectionOutputMulti(reader), vb.ConnectionInputMulti(writer))
		buffer := vb.New()
		defer buffer.Release()
		buffer.Write(data)
		_, err = conn.Write(buffer.Bytes())
		if err != nil {
			t.Error(err)
			return
		}
		serverRead := make([]byte, 1024)
		_, err = io.ReadFull(conn, serverRead)
		if err != nil {
			t.Error(err)
			return
		}
		if bytes.Compare(serverRead, data) > 0 {
			t.Error("bad request data")
			return
		}
		server.Close()
	}()

	writeIv := crypto.RandomBytes(cipher.IVSize())
	w := bufio.NewWriter(client)
	_, err = w.Write(writeIv)
	if err != nil {
		t.Fatal(err)
	}
	ew, err := cipher.NewEncryptionWriter(key, writeIv, w)
	if err != nil {
		t.Fatal(err)
	}
	bw := bufio.NewWriter(ew)
	err = shadowsocks.AddressSerializer.WriteAddressAndPort(bw, address, 443)
	if err != nil {
		t.Fatal(err)
	}
	buffer := buf.New()
	defer buf.Release(buffer)
	buffer.Write(data)
	_, err = bw.Write(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	err = bw.Flush()
	if err != nil {
		t.Fatal(err)
	}
	err = w.Flush()
	if err != nil {
		t.Fatal(err)
	}
	readIv := make([]byte, cipher.IVSize())
	_, err = io.ReadFull(client, readIv)
	if err != nil {
		t.Fatal(err)
	}
	input, err := cipher.NewDecryptionReader(key, readIv, client)
	if err != nil {
		t.Fatal(err)
	}
	clientRead := make([]byte, 1024)
	_, err = io.ReadFull(input, clientRead)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(clientRead, data) > 0 {
		t.Fatal("bad response data")
	}

	client.Close()
	wg.Wait()
}
