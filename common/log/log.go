package log

import "github.com/sirupsen/logrus"

func init() {
	logrus.StandardLogger().Formatter.(*logrus.TextFormatter).ForceColors = true
}
