package shadowsocks_test

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"sing/common/rw"
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
	_ "sing/protocol/shadowsocks/shadowstream"
)

func TestShadowsocks(t *testing.T) {
	for index := 1; index <= int(vs.CipherType_XCHACHA20); index++ {
		cipherType := vs.CipherType(index)
		cipher := strings.ReplaceAll(strings.ToLower(cipherType.String()), "_", "-")
		t.Log("Test", cipher, "server")
		testShadowsocksServerTCPWithCipher(t, cipherType, cipher)
		t.Log("Test", cipher, "client")
		testShadowsocksClientTCPWithCipher(t, cipherType, cipher)
		t.Log("Test", cipher, "udp")
		testShadowsocksUDPWithCipher(t, cipherType, cipher)
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
	data := buf.New()
	defer data.Release()
	data.WriteRandom(1024)

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
		_, err = conn.Write(data.ToOwned().Bytes())
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
		if bytes.Compare(clientRead, data.Bytes()) > 0 {
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
	defer common.Close(reader)

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

	if bytes.Compare(serverRead, data.Bytes()) > 0 {
		t.Fatal("bad request data")
	}

	writer, err := cipher.NewEncryptionWriter(key, writeIv, server)
	if err != nil {
		t.Fatal(err)
	}
	writer = rw.GetWriter(writer)
	defer common.Close(writer)
	_, err = writer.Write(data.ToOwned().Bytes())
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait()
}

func BenchmarkShadowsocks(b *testing.B) {
	b.ReportAllocs()
	for _, cipher := range shadowsocks.ListCiphers() {
		b.Run(cipher, func(b *testing.B) {
			benchmarkShadowsocksCipher(b, cipher, 14*1024)
		})
	}
}

func benchmarkShadowsocksCipher(b *testing.B, method string, data int) {
	b.StopTimer()
	b.ResetTimer()
	b.SetBytes(int64(data))
	cipher, _ := shadowsocks.CreateCipher(method)
	iv := buf.New()
	defer iv.Release()
	iv.WriteRandom(cipher.IVSize())
	writer, _ := cipher.NewEncryptionWriter(shadowsocks.Key([]byte("test"), cipher.KeySize()), iv.Bytes(), io.Discard)
	defer common.Close(writer)

	buffer := buf.New()
	defer buffer.Release()
	buffer.Extend(data)

	b.StartTimer()
	if output, ok := writer.(rw.OutputStream); ok {
		for i := 0; i < b.N; i++ {
			output.Process(buffer.Bytes())
		}
	} else {
		writer.Write(buffer.Bytes())
	}

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
	data := buf.New()
	data.WriteRandom(1024)
	defer data.Release()

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
		_, err = conn.Write(data.Bytes())
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
		if bytes.Compare(serverRead, data.Bytes()) > 0 {
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
	defer common.Close(ew)
	bw := bufio.NewWriter(ew)
	err = shadowsocks.AddressSerializer.WriteAddressAndPort(bw, address, 443)
	if err != nil {
		t.Fatal(err)
	}
	_, err = bw.Write(data.ToOwned().Bytes())
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
	defer common.Close(input)
	clientRead := make([]byte, 1024)
	_, err = io.ReadFull(input, clientRead)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(clientRead, data.Bytes()) > 0 {
		t.Fatal("bad response data")
	}

	client.Close()
	wg.Wait()
}

func testShadowsocksUDPWithCipher(t *testing.T, cipherType vs.CipherType, cipherName string) {
	password := "fuck me till the daylight"
	cipher, err := shadowsocks.CreateCipher(cipherName)
	if err != nil {
		t.Log("Skip unsupported method: ", cipherName)
		return
	}
	key := shadowsocks.Key([]byte(password), cipher.KeySize())
	address := socksaddr.AddrFromFqdn("internal.sagernet.org")
	data := buf.New()
	defer data.Release()
	data.WriteRandom(1024)

	protoAccount := &vs.Account{
		Password:   password,
		CipherType: cipherType,
	}
	memoryAccount, err := protoAccount.AsAccount()
	common.Must(err)
	memoryUser := &vp.MemoryUser{
		Account: memoryAccount,
	}

	req := &vp.RequestHeader{
		Version: vs.Version,
		Command: vp.RequestCommandUDP,
		Address: vn.DomainAddress(address.Fqdn()),
		Port:    443,
		User:    memoryUser,
	}
	packet, err := vs.EncodeUDPPacket(req, data.Bytes(), nil)
	if err != nil {
		t.Fatal(err)
	}

	buffer := buf.New()
	defer buffer.Release()
	buffer.Write(packet.BytesTo(int32(cipher.IVSize())))
	err = shadowsocks.AddressSerializer.WriteAddressAndPort(buffer, address, 443)
	if err != nil {
		t.Fatal(err)
	}
	buffer.Write(data.Bytes())

	err = cipher.EncodePacket(key, buffer)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Compare(packet.Bytes(), buffer.Bytes()) > 0 {
		t.Fatal("bad request data\n", packet.Bytes(), "\n", buffer.Bytes())
	}
}
