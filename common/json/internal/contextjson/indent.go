// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package json

import "bytes"

// HTMLEscape appends to dst the JSON-encoded src with <, >, &, U+2028 and U+2029
// characters inside string literals changed to \u003c, \u003e, \u0026, \u2028, \u2029
// so that the JSON will be safe to embed inside HTML <script> tags.
// For historical reasons, web browsers don't honor standard HTML
// escaping within <script> tags, so an alternative JSON encoding must be used.
func HTMLEscape(dst *bytes.Buffer, src []byte) {
	dst.Grow(len(src))
	dst.Write(appendHTMLEscape(dst.AvailableBuffer(), src))
}

func appendHTMLEscape(dst, src []byte) []byte {
	// The characters can only appear in string literals,
	// so just scan the string one byte at a time.
	start := 0
	for i, c := range src {
		if c == '<' || c == '>' || c == '&' {
			dst = append(dst, src[start:i]...)
			dst = append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xF])
			start = i + 1
		}
		// Convert U+2028 and U+2029 (E2 80 A8 and E2 80 A9).
		if c == 0xE2 && i+2 < len(src) && src[i+1] == 0x80 && src[i+2]&^1 == 0xA8 {
			dst = append(dst, src[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hex[src[i+2]&0xF])
			start = i + len("\u2029")
		}
	}
	return append(dst, src[start:]...)
}

// Compact appends to dst the JSON-encoded src with
// insignificant space characters elided.
func Compact(dst *bytes.Buffer, src []byte) error {
	dst.Grow(len(src))
	b := dst.AvailableBuffer()
	b, err := appendCompact(b, src, false)
	dst.Write(b)
	return err
}

func appendCompact(dst, src []byte, escape bool) ([]byte, error) {
	origLen := len(dst)
	scan := newScanner()
	defer freeScanner(scan)
	start := 0
	for i, c := range src {
		if escape && (c == '<' || c == '>' || c == '&') {
			if start < i {
				dst = append(dst, src[start:i]...)
			}
			dst = append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xF])
			start = i + 1
		}
		// Convert U+2028 and U+2029 (E2 80 A8 and E2 80 A9).
		if escape && c == 0xE2 && i+2 < len(src) && src[i+1] == 0x80 && src[i+2]&^1 == 0xA8 {
			if start < i {
				dst = append(dst, src[start:i]...)
			}
			dst = append(dst, '\\', 'u', '2', '0', '2', hex[src[i+2]&0xF])
			start = i + 3
		}
		v := scan.step(scan, c)
		if v >= scanSkipSpace {
			if v == scanError {
				break
			}
			if start < i {
				dst = append(dst, src[start:i]...)
			}
			start = i + 1
		}
	}
	if scan.eof() == scanError {
		return dst[:origLen], scan.err
	}
	if start < len(src) {
		dst = append(dst, src[start:]...)
	}
	return dst, nil
}

type jsonCommentToken struct {
	kind   CommentKind
	text   []byte
	end    int
	closed bool
}

func readJSONComment(src []byte, start int) (jsonCommentToken, bool) {
	if start >= len(src) {
		return jsonCommentToken{}, false
	}
	switch src[start] {
	case '#':
		textStart := start + 1
		textEnd := textStart
		for textEnd < len(src) && src[textEnd] != '\n' && src[textEnd] != '\r' {
			textEnd++
		}
		end := consumeLineEnd(src, textEnd)
		return jsonCommentToken{kind: CommentKindHash, text: src[textStart:textEnd], end: end, closed: true}, true
	case '/':
		if start+1 >= len(src) {
			return jsonCommentToken{}, false
		}
		switch src[start+1] {
		case '/':
			textStart := start + 2
			textEnd := textStart
			for textEnd < len(src) && src[textEnd] != '\n' && src[textEnd] != '\r' {
				textEnd++
			}
			end := consumeLineEnd(src, textEnd)
			return jsonCommentToken{kind: CommentKindLine, text: src[textStart:textEnd], end: end, closed: true}, true
		case '*':
			textStart := start + 2
			for i := textStart; i+1 < len(src); i++ {
				if src[i] == '*' && src[i+1] == '/' {
					return jsonCommentToken{kind: CommentKindBlock, text: src[textStart:i], end: i + 2, closed: true}, true
				}
			}
			return jsonCommentToken{kind: CommentKindBlock, text: src[textStart:], end: len(src)}, true
		}
	}
	return jsonCommentToken{}, false
}

func consumeLineEnd(src []byte, i int) int {
	if i >= len(src) {
		return i
	}
	if src[i] == '\r' {
		i++
		if i < len(src) && src[i] == '\n' {
			i++
		}
		return i
	}
	if src[i] == '\n' {
		return i + 1
	}
	return i
}

func appendLineBreak(dst []byte) []byte {
	return append(dst, '\n')
}

func appendIndentPrefix(dst []byte, prefix, indent string, depth int) []byte {
	dst = append(dst, prefix...)
	for i := 0; i < depth; i++ {
		dst = append(dst, indent...)
	}
	return dst
}

func nextCommentInline(src []byte, i int) bool {
	for i < len(src) {
		switch src[i] {
		case ' ', '\t':
			i++
			continue
		case '\n', '\r':
			return false
		}
		break
	}
	comment, ok := readJSONComment(src, i)
	if !ok || !comment.closed {
		return false
	}
	return comment.kind != CommentKindBlock || !containsLineBreak(comment.text)
}

func containsLineBreak(value []byte) bool {
	for _, c := range value {
		if c == '\n' || c == '\r' {
			return true
		}
	}
	return false
}

func appendJSONComment(dst []byte, comment jsonCommentToken, prefix, indent string, depth int, atLineStart, lineHasContent bool) ([]byte, bool, bool) {
	if comment.kind == CommentKindBlock {
		return appendBlockJSONComment(dst, comment.text, prefix, indent, depth, atLineStart, lineHasContent)
	}
	if lineHasContent {
		dst = appendSpaceBeforeInlineComment(dst)
	} else if atLineStart {
		dst = appendIndentPrefix(dst, prefix, indent, depth)
		atLineStart = false
	}
	dst = appendCommentMarker(dst, comment.kind)
	dst = append(dst, comment.text...)
	dst = appendLineBreak(dst)
	return dst, true, false
}

func appendBlockJSONComment(dst []byte, text []byte, prefix, indent string, depth int, atLineStart, lineHasContent bool) ([]byte, bool, bool) {
	if !containsLineBreak(text) {
		leading := !lineHasContent
		inlineAfterComma := lineHasContent && lastNonSpaceByte(dst) == ','
		if lineHasContent {
			dst = appendSpaceBeforeInlineComment(dst)
		} else if atLineStart {
			dst = appendIndentPrefix(dst, prefix, indent, depth)
			atLineStart = false
		}
		dst = append(dst, '/', '*')
		dst = append(dst, text...)
		dst = append(dst, '*', '/')
		if leading || inlineAfterComma {
			dst = appendLineBreak(dst)
			return dst, true, false
		}
		return dst, false, true
	}
	if lineHasContent {
		dst = appendLineBreak(dst)
		atLineStart = true
		lineHasContent = false
	}
	if atLineStart {
		dst = appendIndentPrefix(dst, prefix, indent, depth)
	}
	dst = appendMultilineBlockComment(dst, text, prefix, indent, depth)
	dst = appendLineBreak(dst)
	return dst, true, false
}

func appendSpaceBeforeInlineComment(dst []byte) []byte {
	if len(dst) == 0 {
		return dst
	}
	switch dst[len(dst)-1] {
	case ' ', '\t', '\n', '\r':
		return dst
	default:
		return append(dst, ' ')
	}
}

func lastNonSpaceByte(value []byte) byte {
	for i := len(value) - 1; i >= 0; i-- {
		switch value[i] {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return value[i]
		}
	}
	return 0
}

func appendCommentMarker(dst []byte, kind CommentKind) []byte {
	switch kind {
	case CommentKindHash:
		return append(dst, '#')
	case CommentKindBlock:
		return append(dst, '/', '*')
	default:
		return append(dst, '/', '/')
	}
}

func appendMultilineBlockComment(dst []byte, text []byte, prefix, indent string, depth int) []byte {
	lines := splitCommentLines(text)
	trim := blockCommentTrim(lines)
	dst = append(dst, '/', '*')
	for i, line := range lines {
		if i > 0 {
			dst = appendLineBreak(dst)
			dst = appendIndentPrefix(dst, prefix, indent, depth)
			line = trimBlockCommentLine(line, trim)
		}
		dst = append(dst, line...)
	}
	dst = append(dst, '*', '/')
	return dst
}

func splitCommentLines(text []byte) [][]byte {
	lines := make([][]byte, 0, bytes.Count(text, []byte{'\n'})+1)
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] != '\n' && text[i] != '\r' {
			continue
		}
		lines = append(lines, text[start:i])
		if text[i] == '\r' && i+1 < len(text) && text[i+1] == '\n' {
			i++
		}
		start = i + 1
	}
	lines = append(lines, text[start:])
	return lines
}

func blockCommentTrim(lines [][]byte) int {
	minIndent := -1
	for _, line := range lines[1:] {
		if len(line) == 0 {
			continue
		}
		indent := leadingCommentIndent(line)
		if indent == len(line) {
			continue
		}
		if minIndent < 0 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent <= 1 {
		return 0
	}
	return minIndent - 1
}

func leadingCommentIndent(line []byte) int {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return i
}

func trimBlockCommentLine(line []byte, trim int) []byte {
	if trim == 0 {
		return line
	}
	if indent := leadingCommentIndent(line); indent < trim {
		return line[indent:]
	}
	return line[trim:]
}

// indentGrowthFactor specifies the growth factor of indenting JSON input.
// Empirically, the growth factor was measured to be between 1.4x to 1.8x
// for some set of compacted JSON with the indent being a single tab.
// Specify a growth factor slightly larger than what is observed
// to reduce probability of allocation in appendIndent.
// A factor no higher than 2 ensures that wasted space never exceeds 50%.
const indentGrowthFactor = 2

// Indent appends to dst an indented form of the JSON-encoded src.
// Each element in a JSON object or array begins on a new,
// indented line beginning with prefix followed by one or more
// copies of indent according to the indentation nesting.
// The data appended to dst does not begin with the prefix nor
// any indentation, to make it easier to embed inside other formatted JSON data.
// Although leading space characters (space, tab, carriage return, newline)
// at the beginning of src are dropped, trailing space characters
// at the end of src are preserved and copied to dst.
// For example, if src has no trailing spaces, neither will dst;
// if src ends in a trailing newline, so will dst.
func Indent(dst *bytes.Buffer, src []byte, prefix, indent string) error {
	dst.Grow(indentGrowthFactor * len(src))
	b := dst.AvailableBuffer()
	b, err := appendIndent(b, src, prefix, indent)
	dst.Write(b)
	return err
}

func appendIndent(dst, src []byte, prefix, indent string) ([]byte, error) {
	origLen := len(dst)
	scan := newScanner()
	defer freeScanner(scan)
	needIndent := false
	depth := 0
	atLineStart := true
	lineHasContent := false
Input:
	for i := 0; i < len(src); i++ {
		c := src[i]
		scan.bytes++
		v := scan.step(scan, c)
		if v == scanSkipSpace {
			if comment, ok := readJSONComment(src, i); ok {
				for j := i + 1; j < comment.end; j++ {
					scan.bytes++
					if scan.step(scan, src[j]) == scanError {
						break Input
					}
				}
				if !comment.closed {
					i = comment.end - 1
					continue
				}
				if needIndent {
					dst = appendLineBreak(dst)
					atLineStart = true
					lineHasContent = false
					needIndent = false
				}
				dst, atLineStart, lineHasContent = appendJSONComment(dst, comment, prefix, indent, depth, atLineStart, lineHasContent)
				i = comment.end - 1
				continue
			}
			continue
		}
		if v == scanError {
			break
		}
		if needIndent && v != scanEndObject && v != scanEndArray {
			needIndent = false
			dst = appendLineBreak(dst)
			atLineStart = true
			lineHasContent = false
		}

		// Emit semantically uninteresting bytes
		// (in particular, punctuation in strings) unmodified.
		if v == scanContinue {
			if atLineStart {
				dst = appendIndentPrefix(dst, prefix, indent, depth)
				atLineStart = false
			}
			dst = append(dst, c)
			lineHasContent = true
			continue
		}

		// Add spacing around real punctuation.
		switch c {
		case '{', '[':
			if atLineStart {
				dst = appendIndentPrefix(dst, prefix, indent, depth)
				atLineStart = false
			}
			// delay indent so that empty object and array are formatted as {} and [].
			needIndent = true
			dst = append(dst, c)
			lineHasContent = true
			depth++
		case ',':
			dst = append(dst, c)
			lineHasContent = true
			if !nextCommentInline(src, i+1) {
				dst = appendLineBreak(dst)
				atLineStart = true
				lineHasContent = false
			}
		case ':':
			dst = append(dst, c, ' ')
			lineHasContent = true
			atLineStart = false
		case '}', ']':
			depth--
			if needIndent {
				// suppress indent in empty object/array
				needIndent = false
			} else if lineHasContent {
				dst = appendLineBreak(dst)
				atLineStart = true
				lineHasContent = false
			}
			if atLineStart {
				dst = appendIndentPrefix(dst, prefix, indent, depth)
				atLineStart = false
			}
			dst = append(dst, c)
			lineHasContent = true
		default:
			if atLineStart {
				dst = appendIndentPrefix(dst, prefix, indent, depth)
				atLineStart = false
			}
			dst = append(dst, c)
			lineHasContent = true
		}
	}
	if scan.eof() == scanError {
		return dst[:origLen], scan.err
	}
	return dst, nil
}
