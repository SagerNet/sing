package json

func UnmarshalDisallowUnknownFields(data []byte, v any) error {
	var d decodeState
	d.disallowUnknownFields = true
	err := checkValid(data, &d.scan)
	if err != nil {
		return err
	}
	d.init(data)
	return d.unmarshal(v)
}
