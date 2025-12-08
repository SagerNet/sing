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

func TestAdGuardWildcardVariants(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name           string
		rule           string
		matchDomains   []string
		noMatchDomains []string
	}{
		{
			name: "wildcard at beginning",
			rule: "||*example.org^",
			matchDomains: []string{
				"example.org",
				"testexample.org",
				"sub.example.org",
				"test.sub.example.org",
				"123example.org",
			},
			noMatchDomains: []string{
				"example.org.cn",
				"notexample.com",
			},
		},
		{
			name: "wildcard in middle",
			rule: "||ex*le.org^",
			matchDomains: []string{
				"example.org",
				"exile.org",
				"exle.org",
				"ex123le.org",
				"www.example.org",
			},
			noMatchDomains: []string{
				"example.com",
				"exile.org.cn",
			},
		},
		{
			name: "wildcard at end",
			rule: "||example.*^",
			matchDomains: []string{
				"example.org",
				"example.com",
				"example.net",
				"example.co.uk",
				"www.example.org",
				"sub.example.net",
			},
			noMatchDomains: []string{
				"notexample.org",
				"exampleorg",
			},
		},
		{
			name: "multiple wildcards",
			rule: "||*example*.org^",
			matchDomains: []string{
				"example.org",
				"testexample123.org",
				"sub.example.test.org",
				"123example456.org",
			},
			noMatchDomains: []string{
				"example.com",
				"test.org",
			},
		},
		{
			name: "wildcard matching dots",
			rule: "||example*org^",
			matchDomains: []string{
				"example.org",
				"example123.org",
				"example.test.org",
				"example.sub.domain.org",
			},
			noMatchDomains: []string{
				"example.org.cn",
				"notexample.org",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			matcher := domain.NewAdGuardMatcher([]string{tc.rule})
			require.NotNil(t, matcher)

			for _, domain := range tc.matchDomains {
				require.True(t, matcher.Match(domain), "Should match: %s with rule: %s", domain, tc.rule)
			}

			for _, domain := range tc.noMatchDomains {
				require.False(t, matcher.Match(domain), "Should not match: %s with rule: %s", domain, tc.rule)
			}
		})
	}
}

func TestAdGuardSyntaxVariants(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name           string
		rule           string
		matchDomains   []string
		noMatchDomains []string
	}{
		{
			name: "|| prefix (domain and subdomains)",
			rule: "||example.org^",
			matchDomains: []string{
				"example.org",
				"www.example.org",
				"sub.example.org",
				"deep.sub.example.org",
			},
			noMatchDomains: []string{
				"notexample.org",
				"example.org.cn",
				"exampleorg",
			},
		},
		{
			name: "| prefix (exact start)",
			rule: "|example.org^",
			matchDomains: []string{
				"example.org",
			},
			noMatchDomains: []string{
				"www.example.org",
				"sub.example.org",
				"notexample.org",
			},
		},
		{
			name: "no prefix (substring match)",
			rule: "example.org^",
			matchDomains: []string{
				"example.org",
				"notexample.org",
				"www.example.org",
				"isexample.org",
			},
			noMatchDomains: []string{
				"example.org.cn",
				"test.com",
			},
		},
		{
			name: "^ suffix (boundary match)",
			rule: "||example.org^",
			matchDomains: []string{
				"example.org",
				"www.example.org",
			},
			noMatchDomains: []string{
				"example.org.cn",
				"example.orgtest",
			},
		},
		{
			name: "no suffix (partial match allowed)",
			rule: "||example.org",
			matchDomains: []string{
				"example.org",
				"example.org.cn",
				"example.orgtest",
				"www.example.org",
				"www.example.org.cn",
			},
			noMatchDomains: []string{
				"notexample.org",
				"example.com",
			},
		},
		{
			name: "| prefix with no ^ suffix",
			rule: "|example.gov",
			matchDomains: []string{
				"example.gov",
				"example.gov.cn",
			},
			noMatchDomains: []string{
				"www.example.gov",
				"notexample.gov",
			},
		},
		{
			name: "plain rule (substring anywhere)",
			rule: "example.arpa",
			matchDomains: []string{
				"example.arpa",
				"www.example.arpa",
				"isexample.arpa",
				"example.arpa.cn",
				"www.example.arpa.cn",
				"isexample.arpa.cn",
			},
			noMatchDomains: []string{
				"example.com",
				"test.arpa",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			matcher := domain.NewAdGuardMatcher([]string{tc.rule})
			require.NotNil(t, matcher)

			for _, domain := range tc.matchDomains {
				require.True(t, matcher.Match(domain), "Should match: %s with rule: %s", domain, tc.rule)
			}

			for _, domain := range tc.noMatchDomains {
				require.False(t, matcher.Match(domain), "Should not match: %s with rule: %s", domain, tc.rule)
			}
		})
	}
}

func TestAdGuardComplexPatterns(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name           string
		rules          []string
		matchDomains   []string
		noMatchDomains []string
	}{
		{
			name: "multiple rules combined",
			rules: []string{
				"||google.*^",
				"||*.youtube.com^",
				"||youtube.com^", // Add explicit rule for youtube.com
				"|ads.*.com^",
			},
			matchDomains: []string{
				"google.com",
				"google.cn",
				"www.google.com",
				"youtube.com",
				"m.youtube.com",
				"www.youtube.com",
				"ads.example.com",
				"ads.test.com",
			},
			noMatchDomains: []string{
				"notgoogle.com",
				"youtube.org",
				"www.ads.example.com",
				"adsexample.com",
			},
		},
		{
			name: "wildcard edge cases",
			rules: []string{
				"||**.example.org^",
				"||*^",
				"||test**test.com^",
			},
			matchDomains: []string{
				"example.org",
				"sub.example.org",
				"deep.sub.example.org",
				"anything.com",
				"test.com",
				"testtest.com",
				"test123test.com",
				"testabc456test.com",
			},
			noMatchDomains: []string{
				// Empty since ||*^ should match everything
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			matcher := domain.NewAdGuardMatcher(tc.rules)
			require.NotNil(t, matcher)

			for _, domain := range tc.matchDomains {
				require.True(t, matcher.Match(domain), "Should match: %s", domain)
			}

			for _, domain := range tc.noMatchDomains {
				require.False(t, matcher.Match(domain), "Should not match: %s", domain)
			}
		})
	}
}
