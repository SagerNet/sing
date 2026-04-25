//go:build without_contextjson

package json

import (
	"context"
	"io"
)

func MarshalContext(ctx context.Context, value any) ([]byte, error) {
	return Marshal(value)
}

func UnmarshalContext(ctx context.Context, content []byte, value any) error {
	return Unmarshal(content, value)
}

func NewEncoderContext(ctx context.Context, writer io.Writer) *Encoder {
	return NewEncoder(writer)
}

func NewDecoderContext(ctx context.Context, reader io.Reader) *Decoder {
	return NewDecoder(reader)
}

func UnmarshalContextDisallowUnknownFields(ctx context.Context, content []byte, value any) error {
	return UnmarshalDisallowUnknownFields(content, value)
}

type ContextMarshaler interface {
	MarshalJSONContext(ctx context.Context) ([]byte, error)
}

type ContextUnmarshaler interface {
	UnmarshalJSONContext(ctx context.Context, content []byte) error
}
