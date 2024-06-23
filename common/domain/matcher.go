package domain

import (
	"encoding/binary"
	"sort"
	"unicode/utf8"

	"github.com/sagernet/sing/common/varbin"
)

type Matcher struct {
	set *succinctSet
}

func NewMatcher(domains []string, domainSuffix []string) *Matcher {
	domainList := make([]string, 0, len(domains)+2*len(domainSuffix))
	seen := make(map[string]bool, len(domainList))
	for _, domain := range domainSuffix {
		if seen[domain] {
			continue
		}
		seen[domain] = true
		if domain[0] == '.' {
			domainList = append(domainList, reverseDomainSuffix(domain))
		} else {
			domainList = append(domainList, reverseDomain(domain))
			domainList = append(domainList, reverseRootDomainSuffix(domain))
		}
	}
	for _, domain := range domains {
		if seen[domain] {
			continue
		}
		seen[domain] = true
		domainList = append(domainList, reverseDomain(domain))
	}
	sort.Strings(domainList)
	return &Matcher{newSuccinctSet(domainList)}
}

type matcherData struct {
	Version     uint8
	Leaves      []uint64
	LabelBitmap []uint64
	Labels      []byte
}

func ReadMatcher(reader varbin.Reader) (*Matcher, error) {
	matcher, err := varbin.ReadValue[matcherData](reader, binary.BigEndian)
	if err != nil {
		return nil, err
	}
	set := &succinctSet{
		leaves:      matcher.Leaves,
		labelBitmap: matcher.LabelBitmap,
		labels:      matcher.Labels,
	}
	set.init()
	return &Matcher{set}, nil
}

func (m *Matcher) Match(domain string) bool {
	return m.set.Has(reverseDomain(domain))
}

func (m *Matcher) Write(writer varbin.Writer) error {
	return varbin.Write(writer, binary.BigEndian, matcherData{
		Version:     1,
		Leaves:      m.set.leaves,
		LabelBitmap: m.set.labelBitmap,
		Labels:      m.set.labels,
	})
}

func reverseDomain(domain string) string {
	l := len(domain)
	b := make([]byte, l)
	for i := 0; i < l; {
		r, n := utf8.DecodeRuneInString(domain[i:])
		i += n
		utf8.EncodeRune(b[l-i:], r)
	}
	return string(b)
}

func reverseDomainSuffix(domain string) string {
	l := len(domain)
	b := make([]byte, l+1)
	for i := 0; i < l; {
		r, n := utf8.DecodeRuneInString(domain[i:])
		i += n
		utf8.EncodeRune(b[l-i:], r)
	}
	b[l] = prefixLabel
	return string(b)
}

func reverseRootDomainSuffix(domain string) string {
	l := len(domain)
	b := make([]byte, l+2)
	for i := 0; i < l; {
		r, n := utf8.DecodeRuneInString(domain[i:])
		i += n
		utf8.EncodeRune(b[l-i:], r)
	}
	b[l] = '.'
	b[l+1] = prefixLabel
	return string(b)
}
