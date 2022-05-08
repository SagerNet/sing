package shadowsocks

import (
	"crypto/md5"
	"hash/crc32"
	"io"
	"math/rand"
	"net"

	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type Method interface {
	Name() string
	KeyLength() int
	DialConn(conn net.Conn, destination M.Socksaddr) (net.Conn, error)
	DialEarlyConn(conn net.Conn, destination M.Socksaddr) net.Conn
	DialPacketConn(conn net.Conn) N.NetPacketConn
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

type ReducedEntropyReader struct {
	io.Reader
}

func (r *ReducedEntropyReader) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)
	if n > 6 {
		const charSet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!#$%&()*+,./:;<=>?@[]^_`{|}~\\\""
		seed := rand.New(rand.NewSource(int64(crc32.ChecksumIEEE(p[:6]))))
		for i := range p[:6] {
			p[i] = charSet[seed.Intn(len(charSet))]
		}
	}
	return
}
