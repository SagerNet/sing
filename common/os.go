package common

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"
)

func FileExists(path string) bool {
	return Error(os.Stat(path)) == nil
}

func WriteFile(path string, content []byte) error {
	if strings.Contains(path, "/") {
		parent := path[:strings.LastIndex(path, "/")]
		if !FileExists(parent) {
			err := os.MkdirAll(parent, 0o755)
			if err != nil {
				return err
			}
		}
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(content)
	return err
}

func ReadJSON(path string, data any) error {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	err = json.Unmarshal(content, data)
	if err != nil {
		return err
	}
	return nil
}

func WriteJSON(path string, data any) error {
	content, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return WriteFile(path, content)
}
