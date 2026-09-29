package cache_test

import (
	"testing"

	"github.com/sagernet/sing/common/cache"
)

func TestDeleteIfPreservesReplacement(t *testing.T) {
	old, replacement := new(int), new(int)
	var c *cache.LruCache[string, *int]
	var evictions int
	c = cache.New[string, *int](cache.WithEvict[string, *int](func(key string, value *int) {
		evictions++
		if value == old {
			// The eviction callback must be able to install a replacement.
			c.Store(key, replacement)
		}
	}))
	c.Store("session", old)
	if !c.DeleteIf("session", func(value *int) bool { return value == old }) {
		t.Fatal("old identity not removed")
	}
	if c.DeleteIf("session", func(value *int) bool { return value == old }) {
		t.Fatal("stale identity removed replacement")
	}
	if got, ok := c.Load("session"); !ok || got != replacement || evictions != 1 {
		t.Fatalf("replacement=%p present=%v evictions=%d", got, ok, evictions)
	}
}
