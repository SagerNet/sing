//go:build debug

package log

import (
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

var basePath string

func init() {
	basePath, _ = filepath.Abs(".")
}

func init() {
	logrus.StandardLogger().SetReportCaller(true)
	logrus.StandardLogger().Formatter.(*logrus.TextFormatter).CallerPrettyfier = func(frame *runtime.Frame) (function string, file string) {
		file = frame.File + ":" + strconv.Itoa(frame.Line)
		if strings.HasPrefix(file, basePath) {
			file = file[len(basePath)+1:]
		}

		file = " " + file
		return
	}
}
