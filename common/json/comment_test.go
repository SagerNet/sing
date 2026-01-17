package json

import (
	"bytes"
	"io"
	"strconv"
	"strings"
	"testing"
)

func readAllWithBuf(t *testing.T, r io.Reader, size int) string {
	t.Helper()
	if size <= 0 {
		t.Fatalf("invalid buffer size %d", size)
	}
	buf := make([]byte, size)
	var out bytes.Buffer
	for i := 0; ; i++ {
		n, err := r.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read error: %v", err)
		}
		if n == 0 {
			t.Fatalf("Read returned 0, nil (possible infinite loop)")
		}
		if i > 1_000_000 {
			t.Fatalf("too many read iterations (possible infinite loop)")
		}
	}
	return out.String()
}

func TestCommentFilter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"path_relative", `{"path": "../../foo"}`, `{"path": "../../foo"}`},
		{"path_absolute", `{"path": "/usr/bin"}`, `{"path": "/usr/bin"}`},
		{"url", `{"url": "https://example.com/api"}`, `{"url": "https://example.com/api"}`},
		{"division", `{"expr": "a/b"}`, `{"expr": "a/b"}`},
		{"regex", `{"re": "/pattern/"}`, `{"re": "/pattern/"}`},
		{"double_quote_escape", `{"s": "a\"b"}`, `{"s": "a\"b"}`},
		{"single_quote_escape", `{'s': 'a\'b'}`, `{'s': 'a\'b'}`},
		{"slash_literal", `{"a": 1}/x`, `{"a": 1}/x`},
		{"line_comment", "{\n// comment\n\"a\": 1}", "{\n\n\"a\": 1}"},
		{"block_comment", "{/* comment */\"a\": 1}", "{\"a\": 1}"},
		{"multiline_star_newline", "{/*star*\n/ still comment\n*/\"a\": 1}", "{\n\n\"a\": 1}"},
		{"hash_comment", "{\n# comment\n\"a\": 1}", "{\n\n\"a\": 1}"},
		{"trailing_slash", `{"a": "/"}`, `{"a": "/"}`},
		{"empty", `{}`, `{}`},
		{"slash_at_eof", `{"a": 1}/`, `{"a": 1}/`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewCommentFilter(strings.NewReader(tt.input))
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("ReadAll error: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("got %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestCommentFilterSmallBuffer(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"path_relative", `{"path": "../../foo"}`, `{"path": "../../foo"}`},
		{"url", `{"url": "https://example.com/api"}`, `{"url": "https://example.com/api"}`},
		{"division", `{"expr": "a/b"}`, `{"expr": "a/b"}`},
		{"double_quote_escape", `{"s": "a\"b"}`, `{"s": "a\"b"}`},
		{"single_quote_escape", `{'s': 'a\'b'}`, `{'s': 'a\'b'}`},
		{"slash_literal", `{"a": 1}/x`, `{"a": 1}/x`},
		{"line_comment", "{\n// comment\n\"a\": 1}", "{\n\n\"a\": 1}"},
		{"block_comment", "{/* comment */\"a\": 1}", "{\"a\": 1}"},
		{"multiline_star_newline", "{/*star*\n/ still comment\n*/\"a\": 1}", "{\n\n\"a\": 1}"},
		{"hash_comment", "{\n# comment\n\"a\": 1}", "{\n\n\"a\": 1}"},
		{"slash_at_eof", `{"a": 1}/`, `{"a": 1}/`},
	}

	for _, size := range []int{1, 2} {
		for _, tt := range tests {
			t.Run(tt.name+"_buf_"+strconv.Itoa(size), func(t *testing.T) {
				r := NewCommentFilter(strings.NewReader(tt.input))
				got := readAllWithBuf(t, r, size)
				if got != tt.want {
					t.Fatalf("got %q, want %q", got, tt.want)
				}
			})
		}
	}
}
