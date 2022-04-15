package geosite

import (
	"github.com/sagernet/sing/common/trieset"
)

type Matcher struct {
	ds *trieset.DomainSet
}

func (m *Matcher) Match(domain string) bool {
	return m.ds.Has(domain)
}

func NewMatcher(domains []string) (*Matcher, error) {
	ds, err := trieset.New(domains)
	if err != nil {
		return nil, err
	}
	return &Matcher{ds}, nil
}
