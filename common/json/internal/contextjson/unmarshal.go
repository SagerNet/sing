package json

import "context"

func UnmarshalDisallowUnknownFields(data []byte, v any) error {
	data, comments, err := stripJSONComments(data)
	if err != nil {
		return err
	}
	var d decodeState
	d.disallowUnknownFields = true
	err = checkValid(data, &d.scan)
	if err != nil {
		return err
	}
	d.init(data)
	d.comments = comments
	return d.unmarshal(v)
}

func UnmarshalContextDisallowUnknownFields(ctx context.Context, data []byte, v any) error {
	data, comments, err := stripJSONComments(data)
	if err != nil {
		return err
	}
	var d decodeState
	d.ctx = ctx
	d.disallowUnknownFields = true
	err = checkValid(data, &d.scan)
	if err != nil {
		return err
	}
	d.init(data)
	d.comments = comments
	return d.unmarshal(v)
}
