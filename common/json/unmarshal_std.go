//go:build without_contextjson

package json

import (
	"bytes"
)

func UnmarshalDisallowUnknownFields(content []byte, value any) error {
	decoder := NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}
