package log

import (
	"strings"

	"github.com/sirupsen/logrus"
)

func init() {
	logrus.SetLevel(logrus.TraceLevel)
	logrus.StandardLogger().Formatter.(*logrus.TextFormatter).ForceColors = true
	logrus.AddHook(new(TaggedHook))
}

func NewLogger(tag string) *logrus.Entry {
	return logrus.NewEntry(logrus.StandardLogger()).WithField("tag", tag)
}

type TaggedHook struct{}

func (h *TaggedHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *TaggedHook) Fire(entry *logrus.Entry) error {
	if tagObj, loaded := entry.Data["tag"]; loaded {
		tag := tagObj.(string)
		delete(entry.Data, "tag")
		entry.Message = strings.ReplaceAll(entry.Message, tag+": ", "")
		entry.Message = "[" + tag + "]: " + entry.Message

	}
	return nil
}
