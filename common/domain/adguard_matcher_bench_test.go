package domain

import (
	"fmt"
	"testing"
)

func BenchmarkAdGuardMatcher(b *testing.B) {
	benchCases := []struct {
		name        string
		rules       []string
		testDomains []string
	}{
		{
			name: "simple_exact",
			rules: []string{
				"||example.com^",
				"||test.org^",
				"||demo.net^",
			},
			testDomains: []string{
				"example.com",
				"www.example.com",
				"test.org",
				"sub.test.org",
				"notmatched.com",
			},
		},
		{
			name: "wildcard_simple",
			rules: []string{
				"||*.example.com^",
				"||google.*^",
				"||test*.org^",
			},
			testDomains: []string{
				"sub.example.com",
				"deep.sub.example.com",
				"google.com",
				"google.cn",
				"test123.org",
				"notmatched.com",
			},
		},
		{
			name: "wildcard_complex",
			rules: []string{
				"||*example*.com^",
				"||*test*demo*.org^",
				"||**.google.com^",
			},
			testDomains: []string{
				"myexample123.com",
				"test.example.sub.com",
				"atestbdemo123.org",
				"deep.sub.google.com",
				"notmatched.net",
			},
		},
		{
			name: "mixed_patterns",
			rules: []string{
				"||example.com^",
				"|ads.*.com^",
				"tracking.*.net",
				"||*.youtube.com^",
				"||facebook.*^",
			},
			testDomains: []string{
				"example.com",
				"ads.test.com",
				"tracking.demo.net",
				"m.youtube.com",
				"facebook.com",
				"regular.site.com",
			},
		},
		{
			name:  "large_ruleset",
			rules: generateLargeRuleset(100),
			testDomains: []string{
				"example1.com",
				"sub.example50.com",
				"test.example99.com",
				"notmatched.com",
				"google.com",
				"facebook.com",
			},
		},
		{
			name: "deep_subdomains",
			rules: []string{
				"||*.example.com^",
				"||**.test.org^",
				"||deep.*.sub.*.com^",
			},
			testDomains: []string{
				"a.b.c.d.e.f.example.com",
				"very.deep.nested.subdomain.test.org",
				"deep.middle.sub.domain.com",
				"simple.com",
			},
		},
	}

	for _, bc := range benchCases {
		b.Run(bc.name, func(b *testing.B) {
			matcher := NewAdGuardMatcher(bc.rules)
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				for _, domain := range bc.testDomains {
					_ = matcher.Match(domain)
				}
			}
		})
	}
}

func BenchmarkAdGuardMatcherParallel(b *testing.B) {
	rules := []string{
		"||*.example.com^",
		"||google.*^",
		"||*test*.org^",
		"||facebook.com^",
		"|ads.*.com^",
	}

	testDomains := []string{
		"sub.example.com",
		"google.com",
		"mytest123.org",
		"facebook.com",
		"ads.network.com",
		"regular.site.com",
	}

	matcher := NewAdGuardMatcher(rules)

	b.RunParallel(func(pb *testing.PB) {
		domainIdx := 0
		for pb.Next() {
			_ = matcher.Match(testDomains[domainIdx%len(testDomains)])
			domainIdx++
		}
	})
}

func BenchmarkAdGuardMatcherConstruction(b *testing.B) {
	benchCases := []struct {
		name  string
		count int
	}{
		{"10_rules", 10},
		{"100_rules", 100},
		{"1000_rules", 1000},
	}

	for _, bc := range benchCases {
		b.Run(bc.name, func(b *testing.B) {
			rules := generateLargeRuleset(bc.count)
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = NewAdGuardMatcher(rules)
			}
		})
	}
}

func BenchmarkAdGuardWildcardDepth(b *testing.B) {
	benchCases := []struct {
		name   string
		rule   string
		domain string
	}{
		{
			name:   "no_wildcard",
			rule:   "||example.com^",
			domain: "example.com",
		},
		{
			name:   "single_wildcard_match",
			rule:   "||*.example.com^",
			domain: "sub.example.com",
		},
		{
			name:   "single_wildcard_long_match",
			rule:   "||*example.com^",
			domain: "verylongprefixexample.com",
		},
		{
			name:   "double_wildcard_match",
			rule:   "||*test*demo.com^",
			domain: "prefixtest123demo.com",
		},
		{
			name:   "triple_wildcard_match",
			rule:   "||*a*b*c.com^",
			domain: "123a456b789c.com",
		},
		{
			name:   "wildcard_no_match",
			rule:   "||*test*.com^",
			domain: "notmatching.com",
		},
	}

	for _, bc := range benchCases {
		b.Run(bc.name, func(b *testing.B) {
			matcher := NewAdGuardMatcher([]string{bc.rule})
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = matcher.Match(bc.domain)
			}
		})
	}
}

func generateLargeRuleset(count int) []string {
	rules := make([]string, 0, count)
	for i := 0; i < count; i++ {
		switch i % 4 {
		case 0:
			rules = append(rules, fmt.Sprintf("||example%d.com^", i))
		case 1:
			rules = append(rules, fmt.Sprintf("||*.test%d.org^", i))
		case 2:
			rules = append(rules, fmt.Sprintf("||demo%d.*^", i))
		case 3:
			rules = append(rules, fmt.Sprintf("||*site%d*.net^", i))
		}
	}
	return rules
}

func BenchmarkAdGuardMatcherMemory(b *testing.B) {
	benchCases := []struct {
		name  string
		count int
	}{
		{"10_rules", 10},
		{"100_rules", 100},
		{"1000_rules", 1000},
	}

	for _, bc := range benchCases {
		b.Run(bc.name, func(b *testing.B) {
			rules := generateLargeRuleset(bc.count)
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				matcher := NewAdGuardMatcher(rules)
				_ = matcher.Match("test.example.com")
			}
		})
	}
}
