package acme

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io/ioutil"
	"net/http"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/log"
	"github.com/go-acme/lego/v4/registration"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sirupsen/logrus"
)

func init() {
	// log.Logger = logger.New("acme")
	log.Logger = logrus.StandardLogger()
}

type CertificateManager struct {
	email    string
	path     string
	provider string
}

func NewCertificateManager(settings *Settings) *CertificateManager {
	m := &CertificateManager{
		email:    settings.Email,
		path:     settings.DataDirectory,
		provider: settings.DNSProvider,
	}
	if m.path == "" {
		m.path = "acme"
	}
	return m
}

func (c *CertificateManager) GetKeyPair(domain string) (*tls.Certificate, error) {
	if domain == "" {
		return nil, E.New("acme: empty domain name")
	}

	dnsProvider, err := NewDNSChallengeProviderByName(c.provider)
	if err != nil {
		return nil, err
	}

	accountPath := c.path + "/account.json"
	accountKeyPath := c.path + "/account.key"

	privateKeyPath := c.path + "/" + domain + ".key"
	certificatePath := c.path + "/" + domain + ".crt"
	requestPath := c.path + "/" + domain + ".json"

	if !common.FileExists(accountKeyPath) {
		err = writeNewPrivateKey(accountKeyPath)
		if err != nil {
			return nil, err
		}
	}

	accountKey, err := readPrivateKey(accountKeyPath)
	if err != nil {
		return nil, err
	}

	user := &acmeUser{
		email:      c.email,
		privateKey: accountKey,
	}

	if common.FileExists(accountPath) {
		var content []byte
		content, err = ioutil.ReadFile(accountPath)
		if err != nil {
			return nil, err
		}
		var account registration.Resource
		err = json.Unmarshal(content, &account)
		if err != nil {
			return nil, err
		}
		user.registration = &account
	}

	config := lego.NewConfig(user)
	config.Certificate.KeyType = certcrypto.RSA4096

	client, err := lego.NewClient(config)
	if err != nil {
		return nil, err
	}

	err = client.Challenge.SetDNS01Provider(dnsProvider)
	if err != nil {
		return nil, err
	}

	if user.GetRegistration() == nil {
		account, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return nil, err
		}
		user.registration = account
		err = writeJSON(accountPath, account)
		if err != nil {
			return nil, err
		}
	}

	if !common.FileExists(privateKeyPath) {
		err = writeNewPrivateKey(privateKeyPath)
		if err != nil {
			return nil, err
		}
	}

	privateKey, err := readPrivateKey(privateKeyPath)
	if err != nil {
		return nil, err
	}

	if !common.FileExists(certificatePath) {
		request := certificate.ObtainRequest{
			Domains:    []string{domain},
			Bundle:     true,
			PrivateKey: privateKey,
		}
		certificates, err := client.Certificate.Obtain(request)
		if err != nil {
			return nil, err
		}
		err = writeJSON(requestPath, certificates)
		if err != nil {
			return nil, err
		}
		certResponse, err := http.Get(certificates.CertURL)
		if err != nil {
			return nil, err
		}
		defer certResponse.Body.Close()
		content, err := ioutil.ReadAll(certResponse.Body)
		if err != nil {
			return nil, err
		}
		if certResponse.StatusCode != 200 {
			return nil, E.New("HTTP ", certResponse.StatusCode, ": ", string(content))
		}
		err = ioutil.WriteFile(certificatePath, content, 0o644)
		if err != nil {
			return nil, err
		}
	}

	keyPair, err := tls.LoadX509KeyPair(certificatePath, privateKeyPath)
	if err != nil {
		return nil, err
	}
	return &keyPair, nil
}

type acmeUser struct {
	email        string
	privateKey   crypto.PrivateKey
	registration *registration.Resource
}

func (u *acmeUser) GetEmail() string {
	return u.email
}

func (u *acmeUser) GetRegistration() *registration.Resource {
	return u.registration
}

func (u *acmeUser) GetPrivateKey() crypto.PrivateKey {
	return u.privateKey
}

func writeJSON(path string, data any) error {
	content, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return common.WriteFile(path, content)
}

func readPrivateKey(path string) (crypto.PrivateKey, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(content)
	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return privateKey.(crypto.PrivateKey), nil
}

func writeNewPrivateKey(path string) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}
	pkcsBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return err
	}
	return common.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcsBytes}))
}
