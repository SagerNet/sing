package auth

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/param"
)

const Realm = "sing-box"

type Challenge struct {
	Username  string
	Nonce     string
	Algorithm string
	Uri       string
	CNonce    string
	Nc        string
	Response  string
}

type User struct {
	Username string
	Password string
}

type Authenticator struct {
	userMap map[string][]string
}

func NewAuthenticator(users []User) *Authenticator {
	if len(users) == 0 {
		return nil
	}
	au := &Authenticator{
		userMap: make(map[string][]string),
	}
	for _, user := range users {
		au.userMap[user.Username] = append(au.userMap[user.Username], user.Password)
	}
	return au
}

func (au *Authenticator) Verify(username string, password string) bool {
	passwordList, ok := au.userMap[username]
	return ok && common.Contains(passwordList, password)
}

func (au *Authenticator) VerifyDigest(method string, uri string, s string) (string, bool) {
	c, err := ParseChallenge(s)
	if err != nil {
		return "", false
	}
	if c.Username == "" || c.Nonce == "" || c.Nc == "" || c.CNonce == "" || c.Response == "" {
		return "", false
	}
	if c.Uri != "" {
		uri = c.Uri
	}
	passwordList, ok := au.userMap[c.Username]
	if ok {
		for _, password := range passwordList {
			resp := ""
			if c.Algorithm == "SHA-256" {
				ha1 := sha256str(c.Username + ":" + Realm + ":" + password)
				ha2 := sha256str(method + ":" + uri)
				resp = sha256str(ha1 + ":" + c.Nonce + ":" + c.Nc + ":" + c.CNonce + ":auth:" + ha2)
			} else {
				ha1 := md5str(c.Username + ":" + Realm + ":" + password)
				ha2 := md5str(method + ":" + uri)
				resp = md5str(ha1 + ":" + c.Nonce + ":" + c.Nc + ":" + c.CNonce + ":auth:" + ha2)
			}
			if resp != "" && resp == c.Response {
				return c.Username, true
			}
		}
	}
	return "", false
}

func ParseChallenge(s string) (*Challenge, error) {
	pp, err := param.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("digest: invalid challenge: %w", err)
	}
	var c Challenge

	for _, p := range pp {
		switch p.Key {
		case "username":
			c.Username = p.Value
		case "nonce":
			c.Nonce = p.Value
		case "algorithm":
			c.Algorithm = p.Value
		case "uri":
			c.Uri = p.Value
		case "cnonce":
			c.CNonce = p.Value
		case "nc":
			c.Nc = p.Value
		case "response":
			c.Response = p.Value
		}
	}
	return &c, nil
}

func md5str(str string) string  {
	h := md5.New()
	h.Write([]byte(str))
	return hex.EncodeToString(h.Sum(nil))
}

func sha256str(str string) string  {
	h := sha256.New()
	h.Write([]byte(str))
	return hex.EncodeToString(h.Sum(nil))
}
