package badjson

import (
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/json"
)

func MarshallObjects(objects ...any) ([]byte, error) {
	if len(objects) == 1 {
		return json.Marshal(objects[0])
	}
	var content JSONObject
	for _, object := range objects {
		objectMap, err := newJSONObject(object)
		if err != nil {
			return nil, err
		}
		content.PutAll(objectMap)
	}
	return content.MarshalJSON()
}

func UnmarshallExcluded(inputContent []byte, parentObject any, object any) error {
	parentContent, err := newJSONObject(parentObject)
	if err != nil {
		return err
	}
	var content JSONObject
	err = content.UnmarshalJSON(inputContent)
	if err != nil {
		return err
	}
	for _, key := range parentContent.Keys() {
		content.Remove(key)
	}
	if object == nil {
		if content.IsEmpty() {
			return nil
		}
		return E.New("unexpected key: ", content.Keys()[0])
	}
	inputContent, err = content.MarshalJSON()
	if err != nil {
		return err
	}
	return json.UnmarshalDisallowUnknownFields(inputContent, object)
}

func newJSONObject(object any) (*JSONObject, error) {
	inputContent, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	var content JSONObject
	err = content.UnmarshalJSON(inputContent)
	if err != nil {
		return nil, err
	}
	return &content, nil
}
