package json

import (
	"strconv"
	"strings"
)

type decodePathKind byte

const (
	decodePathKey decodePathKind = iota
	decodePathIndex
)

type decodePathSegment struct {
	kind  decodePathKind
	key   string
	index int
}

func (d *decodeState) pushContextKey(key string) {
	d.context = append(d.context, decodePathSegment{kind: decodePathKey, key: key})
}

func (d *decodeState) pushContextIndex(index int) {
	d.context = append(d.context, decodePathSegment{kind: decodePathIndex, index: index})
}

func (d *decodeState) popContext(length int) {
	d.context = d.context[:length]
}

func (d *decodeState) formatContext() string {
	var description strings.Builder
	for _, segment := range d.context {
		switch segment.kind {
		case decodePathKey:
			if isSimpleContextKey(segment.key) {
				if description.Len() > 0 {
					description.WriteByte('.')
				}
				description.WriteString(segment.key)
			} else {
				description.WriteByte('[')
				description.WriteString(strconv.Quote(segment.key))
				description.WriteByte(']')
			}
		case decodePathIndex:
			description.WriteByte('[')
			description.WriteString(strconv.Itoa(segment.index))
			description.WriteByte(']')
		}
	}
	return description.String()
}

func isSimpleContextKey(key string) bool {
	return key != "" && !strings.ContainsAny(key, ".[]")
}

type contextError struct {
	parent  error
	context string
}

func (c *contextError) Unwrap() error {
	return c.parent
}

func (c *contextError) Error() string {
	switch c.parent.(type) {
	case *contextError:
		return c.context + "." + c.parent.Error()
	default:
		return c.context + ": " + c.parent.Error()
	}
}
