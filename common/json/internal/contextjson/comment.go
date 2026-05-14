package json

import (
	"bytes"
	"sort"
)

type CommentKind string

const (
	CommentKindLine  CommentKind = "//"
	CommentKindHash  CommentKind = "#"
	CommentKindBlock CommentKind = "/*"
)

type CommentPlacement string

const (
	CommentPlacementLeading  CommentPlacement = "leading"
	CommentPlacementTrailing CommentPlacement = "trailing"
	CommentPlacementInner    CommentPlacement = "inner"
)

type CommentPathKind byte

const (
	CommentPathKey CommentPathKind = iota
	CommentPathIndex
)

type CommentPathSegment struct {
	Kind  CommentPathKind
	Key   string
	Index int
}

type CommentPath []CommentPathSegment

type CommentPosition struct {
	Offset int
	Line   int
	Column int
}

type Comment struct {
	Kind      CommentKind
	Placement CommentPlacement
	Path      CommentPath
	Text      string
	Start     CommentPosition
	End       CommentPosition
}

type CommentSet struct {
	Comments []Comment
}

func (s *CommentSet) Add(comment Comment) {
	if s == nil {
		return
	}
	s.Comments = append(s.Comments, comment)
}

func (s *CommentSet) Empty() bool {
	return s == nil || len(s.Comments) == 0
}

func (s *CommentSet) ForPath(path CommentPath) *CommentSet {
	if s == nil {
		return nil
	}
	var filtered CommentSet
	for _, comment := range s.Comments {
		if !commentPathHasPrefix(comment.Path, path) {
			continue
		}
		comment.Path = cloneCommentPath(comment.Path[len(path):])
		filtered.Comments = append(filtered.Comments, comment)
	}
	if len(filtered.Comments) == 0 {
		return nil
	}
	return &filtered
}

type CommentMarshaler interface {
	ContextMarshaler
	Comments() *CommentSet
}

type CommentUnmarshaler interface {
	ContextUnmarshaler
	Comments() *CommentSet
	SetComments(*CommentSet)
}

type commentNode struct {
	path  CommentPath
	start int
	end   int
}

func stripJSONComments(data []byte) ([]byte, *CommentSet, error) {
	clean := append([]byte(nil), data...)
	lineStarts := buildLineStarts(data)
	var comments []Comment
	for i := 0; i < len(data); {
		switch data[i] {
		case '"':
			i = skipJSONString(data, i)
		case '#':
			start := i
			i++
			textStart := i
			for i < len(data) && data[i] != '\n' && data[i] != '\r' {
				i++
			}
			comments = append(comments, newComment(CommentKindHash, data[textStart:i], start, i, lineStarts))
			blankComment(clean, start, i)
		case '/':
			if i+1 >= len(data) {
				i++
				continue
			}
			switch data[i+1] {
			case '/':
				start := i
				i += 2
				textStart := i
				for i < len(data) && data[i] != '\n' && data[i] != '\r' {
					i++
				}
				comments = append(comments, newComment(CommentKindLine, data[textStart:i], start, i, lineStarts))
				blankComment(clean, start, i)
			case '*':
				start := i
				i += 2
				textStart := i
				for i+1 < len(data) && (data[i] != '*' || data[i+1] != '/') {
					i++
				}
				if i+1 >= len(data) {
					return nil, nil, &SyntaxError{"unexpected end of JSON comment", int64(start)}
				}
				textEnd := i
				i += 2
				comments = append(comments, newComment(CommentKindBlock, data[textStart:textEnd], start, i, lineStarts))
				blankComment(clean, start, i)
			default:
				i++
			}
		default:
			i++
		}
	}
	if len(comments) == 0 {
		return clean, nil, nil
	}
	commentSet := assignCommentPaths(data, clean, comments, lineStarts)
	return clean, commentSet, nil
}

func blankComment(data []byte, start, end int) {
	for i := start; i < end; i++ {
		if data[i] != '\n' && data[i] != '\r' {
			data[i] = ' '
		}
	}
}

func newComment(kind CommentKind, text []byte, start, end int, lineStarts []int) Comment {
	return Comment{
		Kind:  kind,
		Text:  string(text),
		Start: commentPosition(start, lineStarts),
		End:   commentPosition(end, lineStarts),
	}
}

func buildLineStarts(data []byte) []int {
	lineStarts := []int{0}
	for i, c := range data {
		if c == '\n' {
			lineStarts = append(lineStarts, i+1)
		}
	}
	return lineStarts
}

func commentPosition(offset int, lineStarts []int) CommentPosition {
	line := sort.Search(len(lineStarts), func(i int) bool {
		return lineStarts[i] > offset
	})
	if line == 0 {
		return CommentPosition{Offset: offset, Line: 1, Column: offset + 1}
	}
	lineStart := lineStarts[line-1]
	return CommentPosition{Offset: offset, Line: line, Column: offset - lineStart + 1}
}

func skipJSONString(data []byte, start int) int {
	i := start + 1
	for i < len(data) {
		switch data[i] {
		case '\\':
			i += 2
		case '"':
			return i + 1
		default:
			i++
		}
	}
	return i
}

func assignCommentPaths(original, clean []byte, comments []Comment, lineStarts []int) *CommentSet {
	nodes := collectCommentNodes(clean)
	set := &CommentSet{Comments: comments}
	for i := range set.Comments {
		placement, path := classifyComment(original, nodes, set.Comments[i])
		set.Comments[i].Placement = placement
		set.Comments[i].Path = path
		set.Comments[i].Start = commentPosition(set.Comments[i].Start.Offset, lineStarts)
		set.Comments[i].End = commentPosition(set.Comments[i].End.Offset, lineStarts)
	}
	return set
}

func classifyComment(data []byte, nodes []commentNode, comment Comment) (CommentPlacement, CommentPath) {
	var containing *commentNode
	for i := range nodes {
		node := &nodes[i]
		if node.start <= comment.Start.Offset && comment.End.Offset <= node.end {
			if containing == nil || len(node.path) > len(containing.path) {
				containing = node
			}
		}
	}

	var previous *commentNode
	for i := range nodes {
		node := &nodes[i]
		if node.end > comment.Start.Offset {
			continue
		}
		if containing != nil && !commentPathHasPrefix(node.path, containing.path) {
			continue
		}
		if previous == nil || node.end > previous.end || node.end == previous.end && len(node.path) > len(previous.path) {
			previous = node
		}
	}
	if previous != nil && isInlineTrailing(data, previous.end, comment.Start.Offset) {
		return CommentPlacementTrailing, cloneCommentPath(previous.path)
	}

	var next *commentNode
	for i := range nodes {
		node := &nodes[i]
		if node.start < comment.End.Offset {
			continue
		}
		if containing != nil && !commentPathHasPrefix(node.path, containing.path) {
			continue
		}
		if next == nil || node.start < next.start || node.start == next.start && len(node.path) > len(next.path) {
			next = node
		}
	}
	if next != nil {
		return CommentPlacementLeading, cloneCommentPath(next.path)
	}
	if containing != nil {
		return CommentPlacementInner, cloneCommentPath(containing.path)
	}
	return CommentPlacementLeading, nil
}

func isInlineTrailing(data []byte, from, to int) bool {
	if from > to || from < 0 || to > len(data) {
		return false
	}
	seenComma := false
	for _, c := range data[from:to] {
		switch c {
		case '\n', '\r':
			return false
		case ' ', '\t':
		case ',':
			if seenComma {
				return false
			}
			seenComma = true
		default:
			return false
		}
	}
	return true
}

func collectCommentNodes(data []byte) []commentNode {
	parser := commentNodeParser{data: data}
	start := parser.skipSpaces(0)
	if start < len(data) {
		parser.parseValue(start, nil, start)
	}
	return parser.nodes
}

type commentNodeParser struct {
	data  []byte
	nodes []commentNode
}

func (p *commentNodeParser) parseValue(i int, path CommentPath, anchor int) (commentNode, int, bool) {
	i = p.skipSpaces(i)
	if anchor < 0 {
		anchor = i
	}
	if i >= len(p.data) {
		return commentNode{}, i, false
	}
	switch p.data[i] {
	case '{':
		return p.parseObject(i, path, anchor)
	case '[':
		return p.parseArray(i, path, anchor)
	case '"':
		end := skipJSONString(p.data, i)
		node := commentNode{path: cloneCommentPath(path), start: anchor, end: end}
		p.nodes = append(p.nodes, node)
		return node, end, true
	default:
		end := p.parseLiteralEnd(i)
		if end == i {
			return commentNode{}, i + 1, false
		}
		node := commentNode{path: cloneCommentPath(path), start: anchor, end: end}
		p.nodes = append(p.nodes, node)
		return node, end, true
	}
}

func (p *commentNodeParser) parseObject(i int, path CommentPath, anchor int) (commentNode, int, bool) {
	i++
	for {
		i = p.skipSpaces(i)
		if i >= len(p.data) {
			return commentNode{}, i, false
		}
		if p.data[i] == '}' {
			end := i + 1
			node := commentNode{path: cloneCommentPath(path), start: anchor, end: end}
			p.nodes = append(p.nodes, node)
			return node, end, true
		}
		keyStart := i
		keyEnd := skipJSONString(p.data, i)
		key, ok := unquote(p.data[keyStart:keyEnd])
		if !ok {
			return commentNode{}, keyEnd, false
		}
		i = p.skipSpaces(keyEnd)
		if i < len(p.data) && p.data[i] == ':' {
			i++
		}
		childPath := appendCommentPathKey(path, key)
		_, i, _ = p.parseValue(i, childPath, keyStart)
		i = p.skipSpaces(i)
		if i < len(p.data) && p.data[i] == ',' {
			i++
			continue
		}
	}
}

func (p *commentNodeParser) parseArray(i int, path CommentPath, anchor int) (commentNode, int, bool) {
	i++
	index := 0
	for {
		i = p.skipSpaces(i)
		if i >= len(p.data) {
			return commentNode{}, i, false
		}
		if p.data[i] == ']' {
			end := i + 1
			node := commentNode{path: cloneCommentPath(path), start: anchor, end: end}
			p.nodes = append(p.nodes, node)
			return node, end, true
		}
		childPath := appendCommentPathIndex(path, index)
		_, i, _ = p.parseValue(i, childPath, i)
		index++
		i = p.skipSpaces(i)
		if i < len(p.data) && p.data[i] == ',' {
			i++
			continue
		}
	}
}

func (p *commentNodeParser) parseLiteralEnd(i int) int {
	for i < len(p.data) {
		c := p.data[i]
		if isSpace(c) || c == ',' || c == '}' || c == ']' || c == ':' {
			return i
		}
		i++
	}
	return i
}

func (p *commentNodeParser) skipSpaces(i int) int {
	for i < len(p.data) && isSpace(p.data[i]) {
		i++
	}
	return i
}

func appendCommentPathKey(path CommentPath, key string) CommentPath {
	next := cloneCommentPath(path)
	next = append(next, CommentPathSegment{Kind: CommentPathKey, Key: key})
	return next
}

func appendCommentPathIndex(path CommentPath, index int) CommentPath {
	next := cloneCommentPath(path)
	next = append(next, CommentPathSegment{Kind: CommentPathIndex, Index: index})
	return next
}

func cloneCommentPath(path CommentPath) CommentPath {
	if len(path) == 0 {
		return nil
	}
	next := make(CommentPath, len(path))
	copy(next, path)
	return next
}

func commentPathHasPrefix(path, prefix CommentPath) bool {
	if len(prefix) > len(path) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

func decodeContextPathToCommentPath(path []decodePathSegment) CommentPath {
	if len(path) == 0 {
		return nil
	}
	commentPath := make(CommentPath, 0, len(path))
	for _, segment := range path {
		switch segment.kind {
		case decodePathKey:
			commentPath = append(commentPath, CommentPathSegment{Kind: CommentPathKey, Key: segment.key})
		case decodePathIndex:
			commentPath = append(commentPath, CommentPathSegment{Kind: CommentPathIndex, Index: segment.index})
		}
	}
	return commentPath
}

func (d *decodeState) commentsForCurrentValue() *CommentSet {
	if d.comments == nil {
		return nil
	}
	return d.comments.ForPath(decodeContextPathToCommentPath(d.context))
}

func insertJSONComments(data []byte, comments *CommentSet) ([]byte, error) {
	if comments.Empty() {
		return data, nil
	}
	nodes := collectCommentNodes(data)
	insertions := make(map[int][][]byte)
	for _, comment := range comments.Comments {
		offset := commentInsertOffset(data, nodes, comment)
		insertions[offset] = append(insertions[offset], formatComment(comment))
	}
	var out bytes.Buffer
	out.Grow(len(data) + len(comments.Comments)*16)
	for i := 0; i <= len(data); i++ {
		if values := insertions[i]; len(values) > 0 {
			for _, value := range values {
				out.Write(value)
			}
		}
		if i < len(data) {
			out.WriteByte(data[i])
		}
	}
	return out.Bytes(), nil
}

func commentInsertOffset(data []byte, nodes []commentNode, comment Comment) int {
	node, ok := findCommentNode(nodes, comment.Path)
	if !ok {
		if comment.Placement == CommentPlacementTrailing {
			return len(data)
		}
		return 0
	}
	switch comment.Placement {
	case CommentPlacementTrailing:
		return trailingCommentInsertOffset(data, node.end)
	case CommentPlacementInner:
		if node.end > node.start && (data[node.start] == '{' || data[node.start] == '[') {
			return node.start + 1
		}
		return node.start
	default:
		return node.start
	}
}

func trailingCommentInsertOffset(data []byte, offset int) int {
	for i := offset; i < len(data); i++ {
		switch data[i] {
		case ' ', '\t':
			continue
		case ',':
			return i + 1
		default:
			return offset
		}
	}
	return offset
}

func findCommentNode(nodes []commentNode, path CommentPath) (commentNode, bool) {
	for _, node := range nodes {
		if len(node.path) != len(path) {
			continue
		}
		if commentPathHasPrefix(node.path, path) {
			return node, true
		}
	}
	return commentNode{}, false
}

func formatComment(comment Comment) []byte {
	switch comment.Kind {
	case CommentKindHash:
		if comment.Placement == CommentPlacementTrailing {
			return []byte(" #" + comment.Text + "\n")
		}
		return []byte("#" + comment.Text + "\n")
	case CommentKindBlock:
		if comment.Placement == CommentPlacementTrailing {
			return []byte(" /*" + comment.Text + "*/")
		}
		return []byte("/*" + comment.Text + "*/\n")
	default:
		if comment.Placement == CommentPlacementTrailing {
			return []byte(" //" + comment.Text + "\n")
		}
		return []byte("//" + comment.Text + "\n")
	}
}
