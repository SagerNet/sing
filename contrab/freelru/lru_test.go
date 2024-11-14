package freelru_test

import (
	"testing"
	"time"

	"github.com/sagernet/sing/contrab/freelru"
	"github.com/sagernet/sing/contrab/maphash"

	"github.com/stretchr/testify/require"
)

func TestMyChange0(t *testing.T) {
	t.Parallel()
	lru, err := freelru.New[string, string](1024, maphash.NewHasher[string]().Hash32)
	require.NoError(t, err)
	lru.SetUpdateLifetimeOnGet(true)
	lru.AddWithLifetime("hello", "world", 2*time.Second)
	time.Sleep(time.Second)
	_, ok := lru.Get("hello")
	require.True(t, ok)
	time.Sleep(time.Second + time.Millisecond*100)
	_, ok = lru.Get("hello")
	require.True(t, ok)
}

func TestMyChange1(t *testing.T) {
	t.Parallel()
	lru, err := freelru.New[string, string](1024, maphash.NewHasher[string]().Hash32)
	require.NoError(t, err)
	lru.SetUpdateLifetimeOnGet(true)
	lru.AddWithLifetime("hello", "world", 2*time.Second)
	time.Sleep(time.Second)
	lru.Peek("hello")
	time.Sleep(time.Second + time.Millisecond*100)
	_, ok := lru.Get("hello")
	require.False(t, ok)
}
