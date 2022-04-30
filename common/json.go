package common

import (
	"encoding/json"

	E "github.com/sagernet/sing/common/exceptions"
)

type JSONMap struct {
	json.RawMessage
	Data map[string]any
}

func (m *JSONMap) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return json.Marshal(m.Data)
}

// UnmarshalJSON sets *m to a copy of data.
func (m *JSONMap) UnmarshalJSON(data []byte) error {
	if m == nil {
		return E.New("JSONMap: UnmarshalJSON on nil pointer")
	}
	if m.Data == nil {
		m.Data = make(map[string]any)
	}
	return json.Unmarshal(data, &m.Data)
}
