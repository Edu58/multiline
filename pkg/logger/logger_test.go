package logger

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestLoggerNew(t *testing.T) {
	testCases := []struct {
		name          string
		out           string
		level         string
		expectedLevel logrus.Level
		wantErr       bool
	}{
		{
			name:          "stdout logging with info level",
			out:           "",
			level:         "info",
			wantErr:       false,
			expectedLevel: logrus.InfoLevel,
		},
		{
			name:          "file logging with debug level",
			out:           "./log.txt",
			level:         "debug",
			wantErr:       false,
			expectedLevel: logrus.DebugLevel,
		},
		{
			name:          "error when invalid level provided",
			out:           "",
			level:         "invalid_level",
			wantErr:       true,
			expectedLevel: logrus.InfoLevel,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer os.Remove(tc.out)
			fakeFormatter := &logrus.TextFormatter{}
			logger, err := New(fakeFormatter, LoggerOptions{
				Out:   tc.out,
				Level: tc.level,
			})

			if err != nil && !tc.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if logger != nil && logger.Level != tc.expectedLevel {
				t.Errorf("New() level = %v, expected %v", logger.Level, tc.expectedLevel)
			}
		})
	}
}
