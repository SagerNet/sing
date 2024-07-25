package domain_test

import (
	"sort"
	"testing"

	"github.com/sagernet/sing/common/domain"

	"github.com/stretchr/testify/require"
)

func TestAdGuardMatcher(t *testing.T) {
	t.Parallel()
	ruleLines := []string{
		"||example.org^",
		"|example.com^",
		"example.net^",
		"||example.edu",
		"||example.edu.tw^",
		"|example.gov",
		"example.arpa",
	}
	matcher := domain.NewAdGuardMatcher(ruleLines)
	require.NotNil(t, matcher)
	matchDomain := []string{
		"example.org",
		"www.example.org",
		"example.com",
		"example.net",
		"isexample.net",
		"www.example.net",
		"example.edu",
		"example.edu.cn",
		"example.edu.tw",
		"www.example.edu",
		"www.example.edu.cn",
		"example.gov",
		"example.gov.cn",
		"example.arpa",
		"www.example.arpa",
		"isexample.arpa",
		"example.arpa.cn",
		"www.example.arpa.cn",
		"isexample.arpa.cn",
	}
	notMatchDomain := []string{
		"example.org.cn",
		"notexample.org",
		"example.com.cn",
		"www.example.com.cn",
		"example.net.cn",
		"notexample.edu",
		"notexample.edu.cn",
		"www.example.gov",
		"notexample.gov",
	}
	for _, domain := range matchDomain {
		require.True(t, matcher.Match(domain), domain)
	}
	for _, domain := range notMatchDomain {
		require.False(t, matcher.Match(domain), domain)
	}
	dLines := matcher.Dump()
	sort.Strings(ruleLines)
	sort.Strings(dLines)
	require.Equal(t, ruleLines, dLines)
}
