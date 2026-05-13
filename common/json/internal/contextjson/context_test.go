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

func TestContextNestedContainers(t *testing.T) {
	t.Parallel()
	type nested struct {
		Items []contextValue             `json:"items"`
		Map   map[string]*contextValue   `json:"map"`
		Ptr   *contextValue              `json:"ptr"`
		Any   any                        `json:"any"`
		Raw   []map[string]contextValue  `json:"raw"`
		Ptrs  []map[string]*contextValue `json:"ptrs"`
	}

	input := nested{
		Items: []contextValue{{}},
		Map: map[string]*contextValue{
			"item": {},
		},
		Ptr: &contextValue{},
		Any: &contextValue{},
		Ptrs: []map[string]*contextValue{{
			"item": {},
		}},
	}
	content, err := json.MarshalContext(contextWithValue("nested"), input)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"items":["nested"]`,
		`"map":{"item":"nested"}`,
		`"ptr":"nested"`,
		`"any":"nested"`,
		`"ptrs":[{"item":"nested"}]`,
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("content = %s, want to contain %s", content, want)
		}
	}

	var output nested
	if err := json.UnmarshalContext(contextWithValue("decoded"), []byte(`{
		"items":[{}],
		"map":{"item":{}},
		"ptr":{},
		"any":{},
		"raw":[{"item":{}}],
		"ptrs":[{"item":{}}]
	}`), &output); err != nil {
		t.Fatal(err)
	}
	if output.Items[0].value != "decoded" {
		t.Fatalf("items value = %q", output.Items[0].value)
	}
	if output.Map["item"].value != "decoded" {
		t.Fatalf("map value = %q", output.Map["item"].value)
	}
	if output.Ptr.value != "decoded" {
		t.Fatalf("ptr value = %q", output.Ptr.value)
	}
	if output.Raw[0]["item"].value != "decoded" {
		t.Fatalf("raw value = %q", output.Raw[0]["item"].value)
	}
	if output.Ptrs[0]["item"].value != "decoded" {
		t.Fatalf("ptrs value = %q", output.Ptrs[0]["item"].value)
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

func TestTypeErrorPath(t *testing.T) {
	t.Parallel()
	var target struct {
		Outer map[string][]struct {
			Count int `json:"count"`
		} `json:"outer"`
	}
	err := json.Unmarshal([]byte(`{"outer":{"a.b":[{"count":"bad"}]}}`), &target)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `outer["a.b"][0].count: json: cannot unmarshal string into Go struct field`) {
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

func TestTokenMoreAfterCommaAcrossRefill(t *testing.T) {
	t.Parallel()
	// Positions the comma at decoder buffer offset 500 and the next value
	// beyond the initial 512-byte read.
	value := strings.Repeat("a", 497)
	decoder := json.NewDecoder(strings.NewReader(`["` + value + `",` + strings.Repeat(" ", 11) + `2]`))
	token, err := decoder.Token()
	if err != nil {
		t.Fatal(err)
	}
	if token != json.Delim('[') {
		t.Fatalf("start token = %v", token)
	}
	if !decoder.More() {
		t.Fatal("expected first array value")
	}
	token, err = decoder.Token()
	if err != nil {
		t.Fatal(err)
	}
	if token != value {
		t.Fatalf("first value = %#v", token)
	}
	if !decoder.More() {
		t.Fatal("expected second array value")
	}
	token, err = decoder.Token()
	if err != nil {
		t.Fatal(err)
	}
	if token != float64(2) {
		t.Fatalf("second value = %#v", token)
	}
	if decoder.More() {
		t.Fatal("expected end of array")
	}
	token, err = decoder.Token()
	if err != nil {
		t.Fatal(err)
	}
	if token != json.Delim(']') {
		t.Fatalf("end token = %v", token)
	}
}

func TestTrailingCommaTokenAcrossRefill(t *testing.T) {
	t.Parallel()
	value := strings.Repeat("a", 497)
	decoder := json.NewDecoder(strings.NewReader(`["` + value + `",` + strings.Repeat(" ", 11) + `]`))
	token, err := decoder.Token()
	if err != nil {
		t.Fatal(err)
	}
	if token != json.Delim('[') {
		t.Fatalf("start token = %v", token)
	}
	if !decoder.More() {
		t.Fatal("expected array value")
	}
	token, err = decoder.Token()
	if err != nil {
		t.Fatal(err)
	}
	if token != value {
		t.Fatalf("value = %#v", token)
	}
	if decoder.More() {
		t.Fatal("expected trailing comma to end array")
	}
	token, err = decoder.Token()
	if err != nil {
		t.Fatal(err)
	}
	if token != json.Delim(']') {
		t.Fatalf("end token = %v", token)
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

func TestTrailingCommaDecoderDecode(t *testing.T) {
	t.Parallel()
	var array []int
	if err := json.NewDecoder(strings.NewReader(`[1,]`)).Decode(&array); err != nil {
		t.Fatal(err)
	}
	if len(array) != 1 || array[0] != 1 {
		t.Fatalf("array = %#v", array)
	}
	var object map[string]int
	if err := json.NewDecoder(strings.NewReader(`{"a":1,}`)).Decode(&object); err != nil {
		t.Fatal(err)
	}
	if object["a"] != 1 {
		t.Fatalf("object = %#v", object)
	}
}

func TestTrailingCommaDoesNotAllowMissingValues(t *testing.T) {
	t.Parallel()
	tests := []string{
		`[,]`,
		`[1,,]`,
		`{,}`,
		`{"a":1,,}`,
	}
	for _, test := range tests {
		t.Run(test, func(t *testing.T) {
			t.Parallel()
			var value any
			if err := json.Unmarshal([]byte(test), &value); err == nil {
				t.Fatal("expected error")
			}
		})
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

type contextTextValue struct {
	value string
}

func (v *contextTextValue) MarshalJSONContext(ctx context.Context) ([]byte, error) {
	return json.Marshal("context")
}

func (v *contextTextValue) MarshalText() ([]byte, error) {
	return []byte("text"), nil
}

func (v *contextTextValue) UnmarshalJSONContext(ctx context.Context, content []byte) error {
	v.value = "context"
	return nil
}

func (v *contextTextValue) UnmarshalText(content []byte) error {
	v.value = "text"
	return nil
}

func TestContextJSONMethodsTakePrecedenceOverTextMethods(t *testing.T) {
	t.Parallel()
	content, err := json.MarshalContext(context.Background(), &contextTextValue{})
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != `"context"` {
		t.Fatalf("content = %s", content)
	}

	var value contextTextValue
	if err := json.UnmarshalContext(context.Background(), []byte(`"value"`), &value); err != nil {
		t.Fatal(err)
	}
	if value.value != "context" {
		t.Fatalf("value = %q", value.value)
	}
}

type pointerContextValue struct{}

func (v *pointerContextValue) MarshalJSONContext(ctx context.Context) ([]byte, error) {
	return json.Marshal(ctx.Value(contextKey{}).(string))
}

func TestPointerContextMarshalerOnAddressableValue(t *testing.T) {
	t.Parallel()
	var input struct {
		Value pointerContextValue `json:"value"`
	}
	content, err := json.MarshalContext(contextWithValue("pointer"), &input)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != `{"value":"pointer"}` {
		t.Fatalf("content = %s", content)
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
