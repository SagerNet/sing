package metadata

import (
	"strings"
	"testing"
)

func TestIsDomainName(t *testing.T) {
	testCases := []struct {
		name   string
		domain string
		want   bool
	}{
		{
			name:   "ipv4 literal",
			domain: "1.2.3.4",
			want:   false,
		},
		{
			name:   "ipv6 literal",
			domain: "2001:db8::1",
			want:   false,
		},
		{
			name:   "bracketed ipv6 literal",
			domain: "[2001:db8::1]",
			want:   false,
		},
		{
			name:   "regular domain",
			domain: "example.com",
			want:   true,
		},
		{
			name:   "non standard character",
			domain: "a_b.example",
			want:   true,
		},
		{
			name:   "numeric final label is allowed",
			domain: "api.prod.42",
			want:   true,
		},
		{
			name:   "empty label",
			domain: "a..b",
			want:   false,
		},
		{
			name:   "empty string",
			domain: "",
			want:   false,
		},
		{
			name:   "label too long",
			domain: strings.Repeat("a", 64) + ".com",
			want:   false,
		},
		{
			name:   "contains null byte",
			domain: "a\x00b.com",
			want:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsDomainName(tc.domain)
			if got != tc.want {
				t.Fatalf("IsDomainName(%q) = %v, want %v", tc.domain, got, tc.want)
			}
		})
	}
}
