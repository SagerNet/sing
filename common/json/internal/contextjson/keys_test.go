package json_test

import (
	"reflect"
	"testing"

	json "github.com/sagernet/sing/common/json/internal/contextjson"
)

type objectWithKeys struct {
	Hello string `json:"hello,omitempty"`
	embeddedObjectWithKeys
	MyWorld2 string `json:"-"`
}

type embeddedObjectWithKeys struct {
	World string `json:"world,omitempty"`
}

func TestObjectKeys(t *testing.T) {
	t.Parallel()
	keys := json.ObjectKeys(reflect.TypeOf(&objectWithKeys{}))
	if !reflect.DeepEqual(keys, []string{"hello", "world"}) {
		t.Fatalf("keys = %#v", keys)
	}
}
