package domain_test

import (
	"encoding/json"
	"net/http"
	"sort"
	"testing"

	"github.com/sagernet/sing/common/domain"

	"github.com/stretchr/testify/require"
)

func TestMatcher(t *testing.T) {
	t.Parallel()
	testDomain := []string{"example.com", "example.org"}
	testDomainSuffix := []string{".com.cn", ".org.cn", "sagernet.org"}
	matcher := domain.NewMatcher(testDomain, testDomainSuffix, false)
	require.NotNil(t, matcher)
	require.True(t, matcher.Match("example.com"))
	require.True(t, matcher.Match("example.org"))
	require.False(t, matcher.Match("example.cn"))
	require.True(t, matcher.Match("example.com.cn"))
	require.True(t, matcher.Match("example.org.cn"))
	require.False(t, matcher.Match("com.cn"))
	require.False(t, matcher.Match("org.cn"))
	require.True(t, matcher.Match("sagernet.org"))
	require.True(t, matcher.Match("sing-box.sagernet.org"))
	dDomain, dDomainSuffix := matcher.Dump()
	require.Equal(t, testDomain, dDomain)
	require.Equal(t, testDomainSuffix, dDomainSuffix)
}

func TestMatcherLegacy(t *testing.T) {
	t.Parallel()
	testDomain := []string{"example.com", "example.org"}
	testDomainSuffix := []string{".com.cn", ".org.cn", "sagernet.org"}
	matcher := domain.NewMatcher(testDomain, testDomainSuffix, true)
	require.NotNil(t, matcher)
	require.True(t, matcher.Match("example.com"))
	require.True(t, matcher.Match("example.org"))
	require.False(t, matcher.Match("example.cn"))
	require.True(t, matcher.Match("example.com.cn"))
	require.True(t, matcher.Match("example.org.cn"))
	require.False(t, matcher.Match("com.cn"))
	require.False(t, matcher.Match("org.cn"))
	require.True(t, matcher.Match("sagernet.org"))
	require.True(t, matcher.Match("sing-box.sagernet.org"))
	dDomain, dDomainSuffix := matcher.Dump()
	require.Equal(t, testDomain, dDomain)
	require.Equal(t, testDomainSuffix, dDomainSuffix)
}

type simpleRuleSet struct {
	Rules []struct {
		Domain       []string `json:"domain"`
		DomainSuffix []string `json:"domain_suffix"`
	}
}

func TestDumpLarge(t *testing.T) {
	t.Parallel()
	response, err := http.Get("https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/cn.json")
	require.NoError(t, err)
	defer response.Body.Close()
	var ruleSet simpleRuleSet
	err = json.NewDecoder(response.Body).Decode(&ruleSet)
	require.NoError(t, err)
	domainList := ruleSet.Rules[0].Domain
	domainSuffixList := ruleSet.Rules[0].DomainSuffix
	require.Len(t, ruleSet.Rules, 1)
	require.True(t, len(domainList)+len(domainSuffixList) > 0)
	sort.Strings(domainList)
	sort.Strings(domainSuffixList)
	matcher := domain.NewMatcher(domainList, domainSuffixList, false)
	require.NotNil(t, matcher)
	dDomain, dDomainSuffix := matcher.Dump()
	require.Equal(t, domainList, dDomain)
	require.Equal(t, domainSuffixList, dDomainSuffix)
}
