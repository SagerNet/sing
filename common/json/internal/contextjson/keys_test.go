package json_test

import (
	"reflect"
	"testing"

	json "github.com/metacubex/sing/common/json/internal/contextjson"

	"github.com/stretchr/testify/require"
)

type MyObject struct {
	Hello string `json:"hello,omitempty"`
	MyWorld
	MyWorld2 string `json:"-"`
}

type MyWorld struct {
	World string `json:"world,omitempty"`
}

func TestObjectKeys(t *testing.T) {
	t.Parallel()
	keys := json.ObjectKeys(reflect.TypeOf(&MyObject{}))
	require.Equal(t, []string{"hello", "world"}, keys)
}
