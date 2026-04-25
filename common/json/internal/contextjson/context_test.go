package json_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	json "github.com/sagernet/sing/common/json/internal/contextjson"
)

type contextKey struct{}

type contextValue struct {
	value string
}

func (v *contextValue) MarshalJSONContext(ctx context.Context) ([]byte, error) {
	return json.Marshal(ctx.Value(contextKey{}).(string))
}

func (v *contextValue) UnmarshalJSONContext(ctx context.Context, content []byte) error {
	v.value = ctx.Value(contextKey{}).(string)
	return nil
}

func contextWithValue(value string) context.Context {
	return context.WithValue(context.Background(), contextKey{}, value)
}

func TestMarshalContextUsesCurrentContext(t *testing.T) {
	t.Parallel()
	var value contextValue
	first, err := json.MarshalContext(contextWithValue("first"), &value)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.MarshalContext(contextWithValue("second"), &value)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != `"first"` {
		t.Fatalf("first marshal = %s", first)
	}
	if string(second) != `"second"` {
		t.Fatalf("second marshal = %s", second)
	}
}

func TestUnmarshalContext(t *testing.T) {
	t.Parallel()
	var value contextValue
	if err := json.UnmarshalContext(contextWithValue("value"), []byte(`{}`), &value); err != nil {
		t.Fatal(err)
	}
	if value.value != "value" {
		t.Fatalf("value = %q", value.value)
	}
}

func TestEncoderDecoderContext(t *testing.T) {
	t.Parallel()
	var buffer bytes.Buffer
	if err := json.NewEncoderContext(contextWithValue("encoded"), &buffer).Encode(&contextValue{}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buffer.String()); got != `"encoded"` {
		t.Fatalf("encoded = %q", got)
	}

	var value contextValue
	if err := json.NewDecoderContext(contextWithValue("decoded"), strings.NewReader(`{}`)).Decode(&value); err != nil {
		t.Fatal(err)
	}
	if value.value != "decoded" {
		t.Fatalf("decoded = %q", value.value)
	}
}

var errContextValue = errors.New("context value error")

type errorContextValue struct{}

func (errorContextValue) UnmarshalJSONContext(ctx context.Context, content []byte) error {
	return errContextValue
}

func TestContextErrorPathAndUnwrap(t *testing.T) {
	t.Parallel()
	var target struct {
		Outer []struct {
			Value errorContextValue `json:"value"`
		} `json:"outer"`
	}
	err := json.UnmarshalContext(context.Background(), []byte(`{"outer":[{"value":{}}]}`), &target)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `outer[0].value: context value error`) {
		t.Fatalf("error = %q", err.Error())
	}
	if !errors.Is(err, errContextValue) {
		t.Fatalf("expected unwrap to contain sentinel, got %v", err)
	}
}

func TestUnknownFieldErrorPath(t *testing.T) {
	t.Parallel()
	var target struct {
		Inner []struct {
			Name string `json:"name"`
		} `json:"inner"`
	}
	err := json.UnmarshalContextDisallowUnknownFields(context.Background(), []byte(`{"inner":[{"unknown":1}]}`), &target)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `inner[0].unknown: json: unknown field "unknown"`) {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestTrailingComma(t *testing.T) {
	t.Parallel()
	var array []int
	if err := json.Unmarshal([]byte(`[1,]`), &array); err != nil {
		t.Fatal(err)
	}
	if len(array) != 1 || array[0] != 1 {
		t.Fatalf("array = %#v", array)
	}
	var object map[string]int
	if err := json.Unmarshal([]byte(`{"a":1,}`), &object); err != nil {
		t.Fatal(err)
	}
	if object["a"] != 1 {
		t.Fatalf("object = %#v", object)
	}
}

type zeroable string

func (z zeroable) IsZero() bool {
	return z == "zero"
}

func TestOmitZero(t *testing.T) {
	t.Parallel()
	type object struct {
		Value zeroable `json:"value,omitzero"`
	}
	content, err := json.Marshal(object{Value: "zero"})
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != `{}` {
		t.Fatalf("content = %s", content)
	}
}
