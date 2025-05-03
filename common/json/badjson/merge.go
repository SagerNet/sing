package badjson

import (
	"os"
	"reflect"

	"github.com/metacubex/sing/common"
	E "github.com/metacubex/sing/common/exceptions"
	"github.com/metacubex/sing/common/json"
)

func Omitempty[T any](value T) (T, error) {
	objectContent, err := json.Marshal(value)
	if err != nil {
		return common.DefaultValue[T](), E.Cause(err, "marshal object")
	}
	rawNewObject, err := Decode(objectContent)
	if err != nil {
		return common.DefaultValue[T](), err
	}
	newObjectContent, err := json.Marshal(rawNewObject)
	if err != nil {
		return common.DefaultValue[T](), E.Cause(err, "marshal new object")
	}
	var newObject T
	err = json.Unmarshal(newObjectContent, &newObject)
	if err != nil {
		return common.DefaultValue[T](), E.Cause(err, "unmarshal new object")
	}
	return newObject, nil
}

func Merge[T any](source T, destination T, disableAppend bool) (T, error) {
	rawSource, err := json.Marshal(source)
	if err != nil {
		return common.DefaultValue[T](), E.Cause(err, "marshal source")
	}
	rawDestination, err := json.Marshal(destination)
	if err != nil {
		return common.DefaultValue[T](), E.Cause(err, "marshal destination")
	}
	return MergeFrom[T](rawSource, rawDestination, disableAppend)
}

func MergeFromSource[T any](rawSource json.RawMessage, destination T, disableAppend bool) (T, error) {
	if rawSource == nil {
		return destination, nil
	}
	rawDestination, err := json.Marshal(destination)
	if err != nil {
		return common.DefaultValue[T](), E.Cause(err, "marshal destination")
	}
	return MergeFrom[T](rawSource, rawDestination, disableAppend)
}

func MergeFromDestination[T any](source T, rawDestination json.RawMessage, disableAppend bool) (T, error) {
	if rawDestination == nil {
		return source, nil
	}
	rawSource, err := json.Marshal(source)
	if err != nil {
		return common.DefaultValue[T](), E.Cause(err, "marshal source")
	}
	return MergeFrom[T](rawSource, rawDestination, disableAppend)
}

func MergeFrom[T any](rawSource json.RawMessage, rawDestination json.RawMessage, disableAppend bool) (T, error) {
	rawMerged, err := MergeJSON(rawSource, rawDestination, disableAppend)
	if err != nil {
		return common.DefaultValue[T](), E.Cause(err, "merge options")
	}
	var merged T
	err = json.Unmarshal(rawMerged, &merged)
	if err != nil {
		return common.DefaultValue[T](), E.Cause(err, "unmarshal merged options")
	}
	return merged, nil
}

func MergeJSON(rawSource json.RawMessage, rawDestination json.RawMessage, disableAppend bool) (json.RawMessage, error) {
	if rawSource == nil && rawDestination == nil {
		return nil, os.ErrInvalid
	} else if rawSource == nil {
		return rawDestination, nil
	} else if rawDestination == nil {
		return rawSource, nil
	}
	source, err := Decode(rawSource)
	if err != nil {
		return nil, E.Cause(err, "decode source")
	}
	destination, err := Decode(rawDestination)
	if err != nil {
		return nil, E.Cause(err, "decode destination")
	}
	if source == nil {
		return json.Marshal(destination)
	} else if destination == nil {
		return json.Marshal(source)
	}
	merged, err := mergeJSON(source, destination, disableAppend)
	if err != nil {
		return nil, err
	}
	return json.Marshal(merged)
}

func mergeJSON(anySource any, anyDestination any, disableAppend bool) (any, error) {
	switch destination := anyDestination.(type) {
	case JSONArray:
		if !disableAppend {
			switch source := anySource.(type) {
			case JSONArray:
				destination = append(destination, source...)
			default:
				destination = append(destination, source)
			}
		}
		return destination, nil
	case *JSONObject:
		switch source := anySource.(type) {
		case *JSONObject:
			for _, entry := range source.Entries() {
				oldValue, loaded := destination.Get(entry.Key)
				if loaded {
					var err error
					entry.Value, err = mergeJSON(entry.Value, oldValue, disableAppend)
					if err != nil {
						return nil, E.Cause(err, "merge object item ", entry.Key)
					}
				}
				destination.Put(entry.Key, entry.Value)
			}
		default:
			return nil, E.New("cannot merge json object into ", reflect.TypeOf(source))
		}
		return destination, nil
	default:
		return destination, nil
	}
}
