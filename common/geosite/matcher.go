package geosite

import (
	"regexp"
	"strings"

	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/trieset"
)

type Matcher struct {
	ds    *trieset.DomainSet
	regex []*regexp.Regexp
}

func (m *Matcher) Match(domain string) bool {
	match := m.ds.Has(domain)
	if match {
		return match
	}
	if m.regex != nil {
		for _, pattern := range m.regex {
			match = pattern.MatchString(domain)
			if match {
				return match
			}
		}
	}
	return false
}

func NewMatcher(domains []string) (*Matcher, error) {
	var regex []*regexp.Regexp
	for i := range domains {
		domain := domains[i]
		if strings.HasPrefix(domain, "regexp:") {
			domain = domain[7:]
			pattern, err := regexp.Compile(domain)
			if err != nil {
				return nil, E.CauseF(err, "compile regex rule ", domain)
			}
			regex = append(regex, pattern)
		}
	}
	ds, err := trieset.New(domains)
	if err != nil {
		return nil, err
	}
	return &Matcher{ds, regex}, nil
}
