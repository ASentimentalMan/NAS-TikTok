package logging

import (
	"log"
	"os"
	"strings"
)

type Level uint8

const (
	LevelOff Level = iota
	LevelInfo
	LevelDebug
)

// Logger applies one level policy to every application module. Errors remain
// visible at every setting so failures can still be diagnosed when logging is
// otherwise disabled.
type Logger struct {
	sink  *log.Logger
	level Level
}

func New(level string, sink *log.Logger) *Logger {
	if sink == nil {
		sink = log.Default()
	}
	return &Logger{sink: sink, level: parseLevel(level)}
}

func FromEnv() *Logger {
	return New(os.Getenv("LOG_LEVEL"), log.Default())
}

func (logger *Logger) Debugf(format string, args ...any) {
	if logger != nil && logger.level >= LevelDebug {
		logger.sink.Printf(format, args...)
	}
}

func (logger *Logger) Infof(format string, args ...any) {
	if logger != nil && logger.level >= LevelInfo {
		logger.sink.Printf(format, args...)
	}
}

func (logger *Logger) Errorf(format string, args ...any) {
	if logger != nil {
		logger.sink.Printf(format, args...)
	}
}

func (logger *Logger) ErrorLogger() *log.Logger {
	if logger == nil {
		return log.Default()
	}
	return log.New(errorWriter{logger: logger}, "", 0)
}

type errorWriter struct {
	logger *Logger
}

func (writer errorWriter) Write(message []byte) (int, error) {
	text := strings.TrimSuffix(string(message), "\n")
	if text != "" {
		writer.logger.Errorf("%s", text)
	}
	return len(message), nil
}

func parseLevel(level string) Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return LevelDebug
	case "off":
		return LevelOff
	case "", "info":
		return LevelInfo
	default:
		return LevelInfo
	}
}
