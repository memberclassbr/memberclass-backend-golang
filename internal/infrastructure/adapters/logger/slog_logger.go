package logger

import (
	"log/slog"
	"os"

	"github.com/memberclass-backend-golang/internal/domain/ports"
)

var (
	mapLogLevel = map[string]slog.Level{
		"DEBUG": slog.LevelDebug,
		"INFO":  slog.LevelInfo,
		"WARN":  slog.LevelWarn,
		"ERROR": slog.LevelError,
	}
)

type SlogLogger struct {
}

func (s *SlogLogger) Debug(msg string, args ...any) {
	slog.Debug(msg, args...)
}

func (s *SlogLogger) Info(msg string, args ...any) {
	slog.Info(msg, args...)
}

func (s *SlogLogger) Warn(msg string, args ...any) {
	slog.Warn(msg, args...)
}

func (s *SlogLogger) Error(msg string, args ...any) {
	slog.Error(msg, args...)
}

func parseLevel(level string) slog.Level {
	lvl, ok := mapLogLevel[level]
	if !ok {
		return slog.LevelInfo
	}
	return lvl
}

func NewLogger() ports.Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))

	return &SlogLogger{}
}
