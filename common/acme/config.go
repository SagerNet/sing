package acme

import (
	"os"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/log"
	"github.com/go-acme/lego/v4/providers/dns"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/acme/cloudflare"
)

type Settings struct {
	Enabled       bool            `json:"enabled"`
	DataDirectory string          `json:"data_directory"`
	Email         string          `json:"email"`
	DNSProvider   string          `json:"dns_provider"`
	DNSEnv        *common.JSONMap `json:"dns_env"`
}

func (s *Settings) SetEnv() error {
	for envName, envValue := range s.DNSEnv.Data {
		log.Infof("acme: set dns env %s=%s", envName, envValue)
		err := os.Setenv(envName, envValue.(string))
		if err != nil {
			return err
		}
	}
	return nil
}

func NewDNSChallengeProviderByName(name string) (challenge.Provider, error) {
	switch name {
	case "cloudflare":
		return cloudflare.NewDNSProvider()
	}
	return dns.NewDNSChallengeProviderByName(name)
}
