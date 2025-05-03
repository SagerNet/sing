//go:build go1.20 && !without_contextjson

package json

import (
	json "github.com/metacubex/sing/common/json/internal/contextjson"
)

var UnmarshalDisallowUnknownFields = json.UnmarshalDisallowUnknownFields
