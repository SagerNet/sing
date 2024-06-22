package badjson

import (
	"bytes"
	"strings"

	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/common/x/linkedhashmap"
)

type TypedMap[K comparable, V any] struct {
	linkedhashmap.Map[K, V]
}

func (m TypedMap[K, V]) MarshalJSON() ([]byte, error) {
	buffer := new(bytes.Buffer)
	buffer.WriteString("{")
	items := m.Entries()
	iLen := len(items)
	for i, entry := range items {
		keyContent, err := json.Marshal(entry.Key)
		if err != nil {
			return nil, err
		}
		buffer.WriteString(strings.TrimSpace(string(keyContent)))
		buffer.WriteString(": ")
		valueContent, err := json.Marshal(entry.Value)
		if err != nil {
			return nil, err
		}
		buffer.WriteString(strings.TrimSpace(string(valueContent)))
		if i < iLen-1 {
			buffer.WriteString(", ")
		}
	}
	buffer.WriteString("}")
	return buffer.Bytes(), nil
}

func (m *TypedMap[K, V]) UnmarshalJSON(content []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	m.Clear()
	objectStart, err := decoder.Token()
	if err != nil {
		return err
	} else if objectStart != json.Delim('{') {
		return E.New("expected json object start, but starts with ", objectStart)
	}
	err = m.decodeJSON(decoder)
	if err != nil {
		return E.Cause(err, "decode json object content")
	}
	objectEnd, err := decoder.Token()
	if err != nil {
		return err
	} else if objectEnd != json.Delim('}') {
		return E.New("expected json object end, but ends with ", objectEnd)
	}
	return nil
}

func (m *TypedMap[K, V]) decodeJSON(decoder *json.Decoder) error {
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		keyContent, err := json.Marshal(keyToken)
		if err != nil {
			return err
		}
		var entryKey K
		err = json.Unmarshal(keyContent, &entryKey)
		if err != nil {
			return err
		}
		var entryValue V
		err = decoder.Decode(&entryValue)
		if err != nil {
			return err
		}
		m.Put(entryKey, entryValue)
	}
	return nil
}
