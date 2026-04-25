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

type conflictingObjectKeys struct {
	conflictingObjectA
	conflictingObjectB
	Keep string `json:"keep"`
}

type conflictingObjectA struct {
	Value string `json:"same"`
}

type conflictingObjectB struct {
	Value string `json:"same"`
}

func TestObjectKeys(t *testing.T) {
	t.Parallel()
	keys := json.ObjectKeys(reflect.TypeOf(&objectWithKeys{}))
	if !reflect.DeepEqual(keys, []string{"hello", "world"}) {
		t.Fatalf("keys = %#v", keys)
	}
}

func TestObjectKeysConflict(t *testing.T) {
	t.Parallel()
	keys := json.ObjectKeys(reflect.TypeOf(conflictingObjectKeys{}))
	if !reflect.DeepEqual(keys, []string{"keep"}) {
		t.Fatalf("keys = %#v", keys)
	}
}

func TestObjectKeysReturnsFreshSlice(t *testing.T) {
	t.Parallel()
	objectType := reflect.TypeOf(&objectWithKeys{})
	keys := json.ObjectKeys(objectType)
	keys[0] = "mutated"
	keys = json.ObjectKeys(objectType)
	if !reflect.DeepEqual(keys, []string{"hello", "world"}) {
		t.Fatalf("keys = %#v", keys)
	}
}
