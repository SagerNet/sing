package geoip

import (
	"net"
	"strings"
	"sync"

	"github.com/oschwald/geoip2-golang"
)

var (
	mmdb     *geoip2.Reader
	loadOnce sync.Once
	loadErr  error
)

func LoadMMDB(path string) error {
	if loadErr != nil {
		loadOnce = sync.Once{}
	}
	loadOnce.Do(func() {
		mmdb, loadErr = geoip2.Open(path)
	})
	return loadErr
}

func Match(code string, ip net.IP) bool {
	country, err := mmdb.Country(ip)
	if err != nil {
		return false
	}
	return strings.EqualFold(country.Country.IsoCode, code)
}
