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
	tests := []struct {
		name   string
		suffix string
	}{
		{
			name:   "spaces",
			suffix: strings.Repeat(" ", 11) + `]`,
		},
		{
			name:   "line comment",
			suffix: strings.Repeat(" ", 11) + "// trailing\n]",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			decoder := json.NewDecoder(strings.NewReader(`["` + value + `",` + test.suffix))
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

func TestUnmarshalAcceptsJSONComments(t *testing.T) {
	t.Parallel()
	var value map[string]int
	if err := json.Unmarshal([]byte(`{
		// leading
		"a": 1,
		# hash
		"b": 2, /* block */
	}`), &value); err != nil {
		t.Fatal(err)
	}
	if value["a"] != 1 || value["b"] != 2 {
		t.Fatalf("value = %#v", value)
	}
}

func TestDecoderAcceptsJSONComments(t *testing.T) {
	t.Parallel()
	decoder := json.NewDecoder(strings.NewReader(`// before
	{"value": 1}`))
	var value map[string]int
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	if value["value"] != 1 {
		t.Fatalf("value = %#v", value)
	}
}

func TestTokenAcceptsJSONComments(t *testing.T) {
	t.Parallel()
	decoder := json.NewDecoder(strings.NewReader(`[// before
	1]`))
	token, err := decoder.Token()
	if err != nil {
		t.Fatal(err)
	}
	if token != json.Delim('[') {
		t.Fatalf("start token = %v", token)
	}
	token, err = decoder.Token()
	if err != nil {
		t.Fatal(err)
	}
	if token != float64(1) {
		t.Fatalf("value token = %#v", token)
	}
	token, err = decoder.Token()
	if err != nil {
		t.Fatal(err)
	}
	if token != json.Delim(']') {
		t.Fatalf("end token = %v", token)
	}
}

type commentContextValue struct {
	Value       int `json:"value"`
	Raw         string
	CommentsSet *json.CommentSet
}

func (v *commentContextValue) MarshalJSONContext(ctx context.Context) ([]byte, error) {
	return json.MarshalContext(ctx, struct {
		Value int `json:"value"`
	}{Value: v.Value})
}

func (v *commentContextValue) UnmarshalJSONContext(ctx context.Context, content []byte) error {
	v.Raw = string(content)
	var wire struct {
		Value int `json:"value"`
	}
	if err := json.UnmarshalContext(ctx, content, &wire); err != nil {
		return err
	}
	v.Value = wire.Value
	return nil
}

func (v *commentContextValue) Comments() *json.CommentSet {
	return v.CommentsSet
}

func (v *commentContextValue) SetComments(comments *json.CommentSet) {
	v.CommentsSet = comments
}

func TestCommentUnmarshalerReceivesComments(t *testing.T) {
	t.Parallel()
	var value commentContextValue
	if err := json.UnmarshalContext(context.Background(), []byte(`{
		// value leading
		"value": 1 // value trailing
	}`), &value); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(value.Raw, "//") {
		t.Fatalf("raw content contains comments: %q", value.Raw)
	}
	if value.CommentsSet == nil || len(value.CommentsSet.Comments) != 2 {
		t.Fatalf("comments = %#v", value.CommentsSet)
	}
	leading, trailing := value.CommentsSet.Comments[0], value.CommentsSet.Comments[1]
	if leading.Placement != json.CommentPlacementLeading || len(leading.Path) != 1 || leading.Path[0].Key != "value" {
		t.Fatalf("leading comment = %#v", leading)
	}
	if trailing.Placement != json.CommentPlacementTrailing || len(trailing.Path) != 1 || trailing.Path[0].Key != "value" {
		t.Fatalf("trailing comment = %#v", trailing)
	}
}

func TestCommentMarshalerWritesComments(t *testing.T) {
	t.Parallel()
	value := &commentContextValue{
		Value: 1,
		CommentsSet: &json.CommentSet{Comments: []json.Comment{
			{
				Kind:      json.CommentKindLine,
				Placement: json.CommentPlacementLeading,
				Path:      json.CommentPath{{Kind: json.CommentPathKey, Key: "value"}},
				Text:      " value leading",
			},
			{
				Kind:      json.CommentKindBlock,
				Placement: json.CommentPlacementTrailing,
				Path:      json.CommentPath{{Kind: json.CommentPathKey, Key: "value"}},
				Text:      " value trailing ",
			},
		}},
	}
	content, err := json.MarshalContext(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "// value leading") || !strings.Contains(string(content), "/* value trailing */") {
		t.Fatalf("content = %s", content)
	}
	var decoded map[string]int
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["value"] != 1 {
		t.Fatalf("decoded = %#v", decoded)
	}
}

type commentFormatValue struct {
	Listen      string
	Port        int
	Legacy      bool
	Route       map[string]any
	Items       []int
	CommentsSet *json.CommentSet
}

func (v *commentFormatValue) MarshalJSONContext(ctx context.Context) ([]byte, error) {
	type wire struct {
		Listen string          `json:"listen,omitempty"`
		Port   int             `json:"port,omitempty"`
		Legacy bool            `json:"legacy,omitempty"`
		Route  *map[string]any `json:"route,omitempty"`
		Items  []int           `json:"items,omitempty"`
	}
	var route *map[string]any
	if v.Route != nil {
		route = &v.Route
	}
	return json.MarshalContext(ctx, wire{
		Listen: v.Listen,
		Port:   v.Port,
		Legacy: v.Legacy,
		Route:  route,
		Items:  v.Items,
	})
}

func (v *commentFormatValue) UnmarshalJSONContext(ctx context.Context, content []byte) error {
	type wire struct {
		Listen string         `json:"listen"`
		Port   int            `json:"port"`
		Legacy bool           `json:"legacy"`
		Route  map[string]any `json:"route"`
		Items  []int          `json:"items"`
	}
	var decoded wire
	if err := json.UnmarshalContext(ctx, content, &decoded); err != nil {
		return err
	}
	v.Listen = decoded.Listen
	v.Port = decoded.Port
	v.Legacy = decoded.Legacy
	if decoded.Route != nil {
		v.Route = decoded.Route
	}
	v.Items = decoded.Items
	return nil
}

func (v *commentFormatValue) Comments() *json.CommentSet {
	return v.CommentsSet
}

func (v *commentFormatValue) SetComments(comments *json.CommentSet) {
	v.CommentsSet = comments
}

func TestCommentMarshalIndentPreservesJSONCFormatting(t *testing.T) {
	t.Parallel()
	const input = `{
  // listen address
  "listen": "127.0.0.1",
  "port": 7890, // mixed port
  # legacy option
  "legacy": true,
  /*
   * block line 1
   * block line 2
   */
  "route": {}
}`
	var value commentFormatValue
	if err := json.UnmarshalContext(context.Background(), []byte(input), &value); err != nil {
		t.Fatal(err)
	}
	content, err := json.MarshalIndent(&value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != input {
		t.Fatalf("content mismatch\nwant:\n%s\n\ngot:\n%s", input, content)
	}
}

func TestCommentMarshalIndentFormatsArrayComments(t *testing.T) {
	t.Parallel()
	const input = `{
  "items": [
    // first
    1,
    2, // second
    3
  ]
}`
	var value commentFormatValue
	if err := json.UnmarshalContext(context.Background(), []byte(input), &value); err != nil {
		t.Fatal(err)
	}
	content, err := json.MarshalIndent(&value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != input {
		t.Fatalf("content mismatch\nwant:\n%s\n\ngot:\n%s", input, content)
	}
}

func TestIndentPreservesJSONComments(t *testing.T) {
	t.Parallel()
	const input = `{"a":1, // a
"b":[/* inner */2,3],"c":4, /* c */"d":5}`
	const expected = `{
  "a": 1, // a
  "b": [
    /* inner */
    2,
    3
  ],
  "c": 4, /* c */
  "d": 5
}`
	var out bytes.Buffer
	if err := json.Indent(&out, []byte(input), "", "  "); err != nil {
		t.Fatal(err)
	}
	if out.String() != expected {
		t.Fatalf("content mismatch\nwant:\n%s\n\ngot:\n%s", expected, out.String())
	}
}

func TestEncoderSetIndentPreservesJSONComments(t *testing.T) {
	t.Parallel()
	value := &commentFormatValue{
		Listen: "127.0.0.1",
		Port:   7890,
		CommentsSet: &json.CommentSet{Comments: []json.Comment{
			{
				Kind:      json.CommentKindLine,
				Placement: json.CommentPlacementLeading,
				Path:      json.CommentPath{{Kind: json.CommentPathKey, Key: "listen"}},
				Text:      " listen address",
			},
			{
				Kind:      json.CommentKindLine,
				Placement: json.CommentPlacementTrailing,
				Path:      json.CommentPath{{Kind: json.CommentPathKey, Key: "port"}},
				Text:      " mixed port",
			},
		}},
	}
	const expected = `{
  // listen address
  "listen": "127.0.0.1",
  "port": 7890 // mixed port
}
`
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		t.Fatal(err)
	}
	if out.String() != expected {
		t.Fatalf("content mismatch\nwant:\n%s\n\ngot:\n%s", expected, out.String())
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
