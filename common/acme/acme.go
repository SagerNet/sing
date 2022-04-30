package acme

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

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

type CertificateUpdateListener func(certificate *tls.Certificate)

type CertificateManager struct {
	email    string
	path     string
	provider string

	access    sync.Mutex
	callbacks map[string][]CertificateUpdateListener
	renew     *time.Ticker
}

func NewCertificateManager(settings *Settings) *CertificateManager {
	m := &CertificateManager{
		email:    settings.Email,
		path:     settings.DataDirectory,
		provider: settings.DNSProvider,
		renew:    time.NewTicker(time.Hour * 24),
	}
	if m.path == "" {
		m.path = "acme"
	}
	go m.loopRenew()
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
		var account registration.Resource
		err = common.ReadJSON(accountPath, &account)
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
		err = common.WriteJSON(accountPath, account)
		if err != nil {
			return nil, err
		}
	}

	var renew bool
	keyPair, err := tls.LoadX509KeyPair(certificatePath, privateKeyPath)
	if err == nil {
		cert, err := x509.ParseCertificate(keyPair.Certificate[0])
		if err == nil {
			keyPair.Leaf = cert
			expiresDays := cert.NotAfter.Sub(time.Now()).Hours() / 24
			if expiresDays > 15 {
				return &keyPair, nil
			} else {
				renew = true
			}
		}
	}

	if renew && common.FileExists(requestPath) {
		var request Certificate
		err = common.ReadJSON(requestPath, &request)
		if err != nil {
			return nil, err
		}
		newCert, err := client.Certificate.Renew((certificate.Resource)(request), true, false, "")
		if err != nil {
			return nil, err
		}
		err = common.WriteJSON(requestPath, (*Certificate)(newCert))
		if err != nil {
			return nil, err
		}
		certResponse, err := http.Get(newCert.CertURL)
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

		keyPair, err = tls.LoadX509KeyPair(certificatePath, privateKeyPath)
		if err == nil {
			return nil, err
		}

		goto finish
	}

	if !common.FileExists(certificatePath) {
		if !common.FileExists(privateKeyPath) {
			err = writeNewPrivateKey(privateKeyPath)
			if err != nil {
				return nil, err
			}
		}

		privateKey, err := readPrivateKey(privateKeyPath)
		if err == nil {
			return &keyPair, nil
		}

		request := certificate.ObtainRequest{
			Domains:    []string{domain},
			Bundle:     true,
			PrivateKey: privateKey,
		}
		certificates, err := client.Certificate.Obtain(request)
		if err != nil {
			return nil, err
		}
		err = common.WriteJSON(requestPath, (*Certificate)(certificates))
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

finish:
	keyPair, err = tls.LoadX509KeyPair(certificatePath, privateKeyPath)
	if err == nil {
		return nil, err
	}

	c.access.Lock()
	for _, listener := range c.callbacks[domain] {
		listener(&keyPair)
	}
	c.access.Unlock()

	return &keyPair, nil
}

func (c *CertificateManager) RegisterUpdateListener(domain string, listener CertificateUpdateListener) {
	c.access.Lock()
	defer c.access.Unlock()

	if c.callbacks == nil {
		c.callbacks = make(map[string][]CertificateUpdateListener)
	}
	c.callbacks[domain] = append(c.callbacks[domain], listener)
}

func (c *CertificateManager) loopRenew() {
	for range c.renew.C {
		c.access.Lock()
		domains := make([]string, 0, len(c.callbacks))
		for domain := range c.callbacks {
			domains = append(domains, domain)
		}
		c.access.Unlock()

		for _, domain := range domains {
			_, _ = c.GetKeyPair(domain)
		}
	}
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

type Certificate struct {
	Domain            string `json:"domain"`
	CertURL           string `json:"certUrl"`
	CertStableURL     string `json:"certStableUrl"`
	PrivateKey        []byte `json:"private_key"`
	Certificate       []byte `json:"certificate"`
	IssuerCertificate []byte `json:"issuer_certificate"`
	CSR               []byte `json:"csr"`
}
