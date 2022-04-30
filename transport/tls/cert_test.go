package tls_test

import (
	"testing"

	"github.com/sagernet/sing/transport/tls"
)

func TestGenerateCertificate(t *testing.T) {
	_, err := tls.GenerateCertificate("cn.bing.com")
	if err != nil {
		t.Fatal(err)
	}
}
