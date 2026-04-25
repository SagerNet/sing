package json

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestContextAPISurface(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	content, err := MarshalContext(ctx, map[string]int{"value": 1})
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != `{"value":1}` {
		t.Fatalf("content = %s", content)
	}

	var decoded map[string]int
	if err := UnmarshalContext(ctx, []byte(`{"value":1}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["value"] != 1 {
		t.Fatalf("decoded = %#v", decoded)
	}

	var buffer bytes.Buffer
	if err := NewEncoderContext(ctx, &buffer).Encode(map[string]int{"value": 1}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(buffer.String()) != `{"value":1}` {
		t.Fatalf("encoded = %q", buffer.String())
	}

	var streamed map[string]int
	if err := NewDecoderContext(ctx, strings.NewReader(`{"value":1}`)).Decode(&streamed); err != nil {
		t.Fatal(err)
	}
	if streamed["value"] != 1 {
		t.Fatalf("streamed = %#v", streamed)
	}
}

func TestUnmarshalContextDisallowUnknownFields(t *testing.T) {
	t.Parallel()
	var target struct {
		Value int `json:"value"`
	}
	if err := UnmarshalContextDisallowUnknownFields(context.Background(), []byte(`{"value":1}`), &target); err != nil {
		t.Fatal(err)
	}
	if target.Value != 1 {
		t.Fatalf("target = %#v", target)
	}
	if err := UnmarshalContextDisallowUnknownFields(context.Background(), []byte(`{"unknown":1}`), &target); err == nil {
		t.Fatal("expected error")
	}
}
