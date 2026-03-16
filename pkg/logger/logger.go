package logger

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

type LoggerOptions struct {
	Out   string
	Level string
}

func New(formatter logrus.Formatter, options LoggerOptions) (*logrus.Logger, error) {
	var out *os.File
	var level string

	if strings.Trim(options.Level, " ") != "" {
		level = options.Level
	} else {
		level = "info"
	}

	if strings.Trim(options.Out, " ") != "" {
		logFile, err := os.OpenFile(options.Out,
			os.O_APPEND|os.O_CREATE|os.O_WRONLY,
			0o644,
		)

		if err != nil {
			return nil, err
		}

		out = logFile
	} else {
		out = os.Stdout
	}

	logLevel, err := logrus.ParseLevel(level)

	if err != nil {
		return nil, err
	}

	return &logrus.Logger{
		Out:       out,
		Formatter: formatter,
		Level:     logLevel,
	}, nil
}
