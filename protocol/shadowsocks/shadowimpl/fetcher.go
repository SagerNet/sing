package shadowimpl

import (
	"encoding/base64"
	"io"
	"strings"

	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/protocol/shadowsocks"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead_2022"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowstream"
)

func FetchMethod(method string, key string, password string, secureRNG io.Reader) (shadowsocks.Method, error) {
	if method == "none" {
		return shadowsocks.NewNone(), nil
	} else if common.Contains(shadowstream.List, method) {
		var keyBytes []byte
		if key != "" {
			kb, err := base64.StdEncoding.DecodeString(key)
			if err != nil {
				return nil, E.Cause(err, "decode key")
			}
			keyBytes = kb
		}
		return shadowstream.New(method, keyBytes, password, secureRNG)
	} else if common.Contains(shadowaead.List, method) {
		var keyBytes []byte
		if key != "" {
			kb, err := base64.StdEncoding.DecodeString(key)
			if err != nil {
				return nil, E.Cause(err, "decode key")
			}
			keyBytes = kb
		}
		return shadowaead.New(method, keyBytes, password, secureRNG)
	} else if common.Contains(shadowaead_2022.List, method) {
		var pskList [][]byte
		if key != "" {
			keyStrList := strings.Split(key, ":")
			pskList = make([][]byte, len(keyStrList))
			for i, keyStr := range keyStrList {
				kb, err := base64.StdEncoding.DecodeString(keyStr)
				if err != nil {
					return nil, E.Cause(err, "decode key")
				}
				pskList[i] = kb
			}
		}
		return shadowaead_2022.New(method, pskList, password, secureRNG)
	} else {
		return nil, E.New("shadowsocks: unsupported method ", method)
	}
}
