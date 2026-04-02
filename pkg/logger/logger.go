// Package logger provides a shared logger for ix-toolkit components.
package logger

import (
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

// New creates a logrus logger configured with the given level and optional file output.
func New(level, filePath string) *logrus.Logger {
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		lvl = logrus.InfoLevel
	}
	log.SetLevel(lvl)

	if filePath != "" {
		f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err == nil {
			log.SetOutput(io.MultiWriter(os.Stderr, f))
			return log
		}
	}
	log.SetOutput(os.Stderr)
	return log
}
