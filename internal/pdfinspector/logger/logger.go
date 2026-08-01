package logger

import (
	"go.uber.org/zap"
)

var globalLogger *zap.Logger

func init() {
	var err error
	globalLogger, err = zap.NewProduction()
	if err != nil {
		globalLogger = zap.NewExample()
	}
}

// Get returns the global Uber Zap structured logger instance.
func Get() *zap.Logger {
	return globalLogger
}

// SetLogger overrides the global Zap logger instance.
func SetLogger(l *zap.Logger) {
	globalLogger = l
}

// Sync flushes buffered log entries.
func Sync() {
	if globalLogger != nil {
		_ = globalLogger.Sync()
	}
}
