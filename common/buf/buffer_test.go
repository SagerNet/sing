package buf_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	vb "github.com/v2fly/v2ray-core/v5/common/buf"
	"sing/common/buf"
)

func TestBuffer(t *testing.T) {
	v := vb.New()
	v.ReadFullFrom(rand.Reader, 1024)
	buffer := buf.New()
	buffer.Write(v.Bytes())
	v.Write(v.Bytes())
	buffer.Write(buffer.Bytes())

	if bytes.Compare(v.Bytes(), buffer.Bytes()) > 0 {
		t.Fatal("bad request data\n", v.Bytes(), "\n", buffer.Bytes())
	}
}
