package json_test

import (
	"context"
	"testing"

	"github.com/metacubex/sing/common/json/internal/contextjson"

	"github.com/stretchr/testify/require"
)

type myStruct struct {
	value string
}

func (m *myStruct) MarshalJSONContext(ctx context.Context) ([]byte, error) {
	return json.Marshal(ctx.Value("key").(string))
}

func (m *myStruct) UnmarshalJSONContext(ctx context.Context, content []byte) error {
	m.value = ctx.Value("key").(string)
	return nil
}

//nolint:staticcheck
func TestMarshalContext(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), "key", "value")
	var s myStruct
	b, err := json.MarshalContext(ctx, &s)
	require.NoError(t, err)
	require.Equal(t, []byte(`"value"`), b)
}

//nolint:staticcheck
func TestUnmarshalContext(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), "key", "value")
	var s myStruct
	err := json.UnmarshalContext(ctx, []byte(`{}`), &s)
	require.NoError(t, err)
	require.Equal(t, "value", s.value)
}
