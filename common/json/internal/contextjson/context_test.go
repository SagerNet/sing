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

func TestContextErrorPathEscapedKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "empty",
			content: `{"":{}}`,
			want:    `[""]: context value error`,
		},
		{
			name:    "dot",
			content: `{"a.b":{}}`,
			want:    `["a.b"]: context value error`,
		},
		{
			name:    "bracket",
			content: `{"a[b]":{}}`,
			want:    `["a[b]"]: context value error`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var target map[string]errorContextValue
			err := json.UnmarshalContext(context.Background(), []byte(test.content), &target)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), test.want)
			}
			if !errors.Is(err, errContextValue) {
				t.Fatalf("expected unwrap to contain sentinel, got %v", err)
			}
		})
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

func TestTrailingCommaToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		start   json.Delim
		end     json.Delim
	}{
		{
			name:    "array",
			content: `[1,]`,
			start:   '[',
			end:     ']',
		},
		{
			name:    "object",
			content: `{"a":1,}`,
			start:   '{',
			end:     '}',
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			decoder := json.NewDecoder(strings.NewReader(test.content))
			token, err := decoder.Token()
			if err != nil {
				t.Fatal(err)
			}
			if token != test.start {
				t.Fatalf("start token = %v", token)
			}
			for decoder.More() {
				if _, err := decoder.Token(); err != nil {
					t.Fatal(err)
				}
			}
			token, err = decoder.Token()
			if err != nil {
				t.Fatal(err)
			}
			if token != test.end {
				t.Fatalf("end token = %v", token)
			}
		})
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

type precedenceValue struct {
	value string
}

func (v *precedenceValue) MarshalJSON() ([]byte, error) {
	return json.Marshal("standard")
}

func (v *precedenceValue) MarshalJSONContext(ctx context.Context) ([]byte, error) {
	return json.Marshal("context")
}

func (v *precedenceValue) UnmarshalJSON(content []byte) error {
	v.value = "standard"
	return nil
}

func (v *precedenceValue) UnmarshalJSONContext(ctx context.Context, content []byte) error {
	v.value = "context"
	return nil
}

func TestStandardJSONMethodsTakePrecedence(t *testing.T) {
	t.Parallel()
	content, err := json.MarshalContext(context.Background(), &precedenceValue{})
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != `"standard"` {
		t.Fatalf("content = %s", content)
	}

	var value precedenceValue
	if err := json.UnmarshalContext(context.Background(), []byte(`"value"`), &value); err != nil {
		t.Fatal(err)
	}
	if value.value != "standard" {
		t.Fatalf("value = %q", value.value)
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
