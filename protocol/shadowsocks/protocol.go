package shadowsocks

import (
	"crypto/md5"
	"crypto/sha1"
	"io"

	"golang.org/x/crypto/hkdf"
	"sing/common"
	"sing/common/socksaddr"
)

const (
	MaxPacketSize          = 16*1024 - 1
	PacketLengthBufferSize = 2
)

func Kdf(key, iv []byte, keyLength int) []byte {
	subKey := make([]byte, keyLength)
	kdf := hkdf.New(sha1.New, key, iv, []byte("ss-subkey"))
	common.Must1(io.ReadFull(kdf, subKey))
	return subKey
}

func Key(password []byte, keySize int) []byte {
	const md5Len = 16

	cnt := (keySize-1)/md5Len + 1
	m := make([]byte, cnt*md5Len)
	sum := md5.Sum(password)
	copy(m, sum[:])

	// Repeatedly call md5 until bytes generated is enough.
	// Each call to md5 uses data: prev md5 sum + password.
	d := make([]byte, md5Len+len(password))
	start := 0
	for i := 1; i < cnt; i++ {
		start += md5Len
		copy(d, m[start-md5Len:start])
		copy(d[md5Len:], password)
		sum = md5.Sum(d)
		copy(m[start:], sum[:])
	}
	return m[:keySize]
}

var AddressSerializer = socksaddr.NewSerializer(
	socksaddr.AddressFamilyByte(0x01, socksaddr.AddressFamilyIPv4),
	socksaddr.AddressFamilyByte(0x04, socksaddr.AddressFamilyIPv6),
	socksaddr.AddressFamilyByte(0x03, socksaddr.AddressFamilyFqdn),
	socksaddr.WithFamilyParser(func(b byte) byte {
		return b & 0x0F
	}),
)
