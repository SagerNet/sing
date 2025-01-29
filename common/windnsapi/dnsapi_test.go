//go:build windows

package windnsapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDNSAPI(t *testing.T) {
	t.Parallel()
	require.NoError(t, FlushResolverCache())
}
