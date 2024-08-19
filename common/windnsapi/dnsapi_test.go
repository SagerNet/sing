package windnsapi

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDNSAPI(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.SkipNow()
	}
	t.Parallel()
	require.NoError(t, FlushResolverCache())
}
