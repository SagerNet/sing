package json

import (
	"context"

	"github.com/sagernet/sing/common/json/internal/contextjson"
)

var (
	MarshalContext                        = json.MarshalContext
	UnmarshalContext                      = json.UnmarshalContext
	NewEncoderContext                     = json.NewEncoderContext
	NewDecoderContext                     = json.NewDecoderContext
	UnmarshalContextDisallowUnknownFields = json.UnmarshalContextDisallowUnknownFields
)

type ContextMarshaler interface {
	MarshalJSONContext(ctx context.Context) ([]byte, error)
}

type ContextUnmarshaler interface {
	UnmarshalJSONContext(ctx context.Context, content []byte) error
}

type (
	CommentKind        = json.CommentKind
	CommentPlacement   = json.CommentPlacement
	CommentPathKind    = json.CommentPathKind
	CommentPathSegment = json.CommentPathSegment
	CommentPath        = json.CommentPath
	CommentPosition    = json.CommentPosition
	Comment            = json.Comment
	CommentSet         = json.CommentSet
	CommentMarshaler   = json.CommentMarshaler
	CommentUnmarshaler = json.CommentUnmarshaler
)

const (
	CommentKindLine          = json.CommentKindLine
	CommentKindHash          = json.CommentKindHash
	CommentKindBlock         = json.CommentKindBlock
	CommentPlacementLeading  = json.CommentPlacementLeading
	CommentPlacementTrailing = json.CommentPlacementTrailing
	CommentPlacementInner    = json.CommentPlacementInner
	CommentPathKey           = json.CommentPathKey
	CommentPathIndex         = json.CommentPathIndex
)
