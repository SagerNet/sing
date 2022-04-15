package common

import (
	"os"
	"strings"

	"github.com/v2fly/v2ray-core/v5/common"
)

func FileExists(path string) bool {
	return common.Error2(os.Stat(path)) == nil
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
