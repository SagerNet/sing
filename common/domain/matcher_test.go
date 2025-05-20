package domain_test

import (
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
