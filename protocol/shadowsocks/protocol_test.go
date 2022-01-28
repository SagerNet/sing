package shadowsocks_test

import (
	"bytes"
	"testing"

	vs "github.com/v2fly/v2ray-core/v5/proxy/shadowsocks"
	"sing/common"
	"sing/protocol/shadowsocks"
)

func TestGenerateKey(t *testing.T) {
	password := "fuck me till the daylight"

	protoAccount := &vs.Account{
		Password:   password,
		CipherType: vs.CipherType_AES_128_GCM,
	}
	memoryAccount, err := protoAccount.AsAccount()
	common.Must(err)
	account := memoryAccount.(*vs.MemoryAccount)
	if bytes.Compare(account.Key, shadowsocks.Key([]byte(password), int(account.Cipher.KeySize()))) > 0 {
		t.Fatal("bad key")
	}
}
