package domain_test

import (
	"bytes"
	"testing"

	"github.com/sagernet/sing/common/domain"
	"github.com/sagernet/sing/common/varbin"
)

// FuzzReadMatcher / FuzzReadAdGuardMatcher fuzz the binary domain-matcher readers, which parse
// untrusted serialized data (e.g. reached from sing-box .srs rule-sets). They guard against
// panics and unbounded allocations when reading the succinct-set length fields.

func FuzzReadMatcher(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = domain.ReadMatcher(varbin.StubReader(bytes.NewReader(data)))
	})
}

func FuzzReadAdGuardMatcher(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = domain.ReadAdGuardMatcher(varbin.StubReader(bytes.NewReader(data)))
	})
}
