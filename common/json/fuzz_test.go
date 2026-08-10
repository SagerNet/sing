package json_test

import (
	"testing"

	"github.com/sagernet/sing/common/json"
)

// FuzzUnmarshalExtended fuzzes the comment-aware JSON decoder, which parses untrusted JSON
// (e.g. sing-box config / rule-sets). It guards against panics such as the slice-bounds bug in
// the comment scanner when a string escape runs off the end of the input.
func FuzzUnmarshalExtended(f *testing.F) {
	f.Add([]byte("{\"a\":1 // c\n}"))
	f.Add([]byte("{\"a\\"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = json.UnmarshalExtended[any](data)
	})
}
