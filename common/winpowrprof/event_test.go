package winpowrprof

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPowerEvents(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.SkipNow()
	}
	listener, err := NewEventListener(func(event int) {})
	require.NoError(t, err)
	require.NotNil(t, listener)
	require.NoError(t, listener.Start())
	require.NoError(t, listener.Close())
}
