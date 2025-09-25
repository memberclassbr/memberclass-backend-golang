package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
)

func TestSlogLogger_Debug(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		args     []any
		expected string
	}{
		{
			name:     "debug with simple message",
			msg:      "test debug message",
			args:     nil,
			expected: "test debug message",
		},
		{
			name:     "debug with arguments",
			msg:      "debug with data",
			args:     []any{"key", "value", "number", 42},
			expected: "debug with data",
		},
		{
			name:     "debug with empty message",
			msg:      "",
			args:     nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})
			logger := slog.New(handler)
			slog.SetDefault(logger)

			slogLogger := &SlogLogger{}
			slogLogger.Debug(tt.msg, tt.args...)

			var logEntry map[string]interface{}
			err := json.Unmarshal(buf.Bytes(), &logEntry)
			if err != nil {
				t.Fatalf("Failed to unmarshal log entry: %v", err)
			}

			if logEntry["msg"] != tt.expected {
				t.Errorf("Expected message %q, got %q", tt.expected, logEntry["msg"])
			}

			if logEntry["level"] != "DEBUG" {
				t.Errorf("Expected level DEBUG, got %v", logEntry["level"])
			}

			if len(tt.args) > 0 {
				for i := 0; i < len(tt.args); i += 2 {
					if i+1 < len(tt.args) {
						key := tt.args[i].(string)
						expectedValue := tt.args[i+1]
						actualValue := logEntry[key]

						if expectedValue != actualValue {
							if expectedInt, ok := expectedValue.(int); ok {
								if actualFloat, ok := actualValue.(float64); ok && float64(expectedInt) == actualFloat {
									continue
								}
							}
							t.Errorf("Expected %s=%v, got %v", key, expectedValue, actualValue)
						}
					}
				}
			}
		})
	}
}

func TestSlogLogger_Info(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		args     []any
		expected string
	}{
		{
			name:     "info with simple message",
			msg:      "test info message",
			args:     nil,
			expected: "test info message",
		},
		{
			name:     "info with arguments",
			msg:      "info with data",
			args:     []any{"user", "john", "id", 123},
			expected: "info with data",
		},
		{
			name:     "info with empty message",
			msg:      "",
			args:     nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			})
			logger := slog.New(handler)
			slog.SetDefault(logger)

			slogLogger := &SlogLogger{}
			slogLogger.Info(tt.msg, tt.args...)

			var logEntry map[string]interface{}
			err := json.Unmarshal(buf.Bytes(), &logEntry)
			if err != nil {
				t.Fatalf("Failed to unmarshal log entry: %v", err)
			}

			if logEntry["msg"] != tt.expected {
				t.Errorf("Expected message %q, got %q", tt.expected, logEntry["msg"])
			}

			if logEntry["level"] != "INFO" {
				t.Errorf("Expected level INFO, got %v", logEntry["level"])
			}

			if len(tt.args) > 0 {
				for i := 0; i < len(tt.args); i += 2 {
					if i+1 < len(tt.args) {
						key := tt.args[i].(string)
						expectedValue := tt.args[i+1]
						actualValue := logEntry[key]

						if expectedValue != actualValue {
							if expectedInt, ok := expectedValue.(int); ok {
								if actualFloat, ok := actualValue.(float64); ok && float64(expectedInt) == actualFloat {
									continue
								}
							}
							t.Errorf("Expected %s=%v, got %v", key, expectedValue, actualValue)
						}
					}
				}
			}
		})
	}
}

func TestSlogLogger_Warn(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		args     []any
		expected string
	}{
		{
			name:     "warn with simple message",
			msg:      "test warn message",
			args:     nil,
			expected: "test warn message",
		},
		{
			name:     "warn with arguments",
			msg:      "warn with data",
			args:     []any{"error", "connection failed", "retry", 3},
			expected: "warn with data",
		},
		{
			name:     "warn with empty message",
			msg:      "",
			args:     nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
				Level: slog.LevelWarn,
			})
			logger := slog.New(handler)
			slog.SetDefault(logger)

			slogLogger := &SlogLogger{}
			slogLogger.Warn(tt.msg, tt.args...)

			var logEntry map[string]interface{}
			err := json.Unmarshal(buf.Bytes(), &logEntry)
			if err != nil {
				t.Fatalf("Failed to unmarshal log entry: %v", err)
			}

			if logEntry["msg"] != tt.expected {
				t.Errorf("Expected message %q, got %q", tt.expected, logEntry["msg"])
			}

			if logEntry["level"] != "WARN" {
				t.Errorf("Expected level WARN, got %v", logEntry["level"])
			}

			if len(tt.args) > 0 {
				for i := 0; i < len(tt.args); i += 2 {
					if i+1 < len(tt.args) {
						key := tt.args[i].(string)
						expectedValue := tt.args[i+1]
						actualValue := logEntry[key]

						if expectedValue != actualValue {
							if expectedInt, ok := expectedValue.(int); ok {
								if actualFloat, ok := actualValue.(float64); ok && float64(expectedInt) == actualFloat {
									continue
								}
							}
							t.Errorf("Expected %s=%v, got %v", key, expectedValue, actualValue)
						}
					}
				}
			}
		})
	}
}

func TestSlogLogger_Error(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		args     []any
		expected string
	}{
		{
			name:     "error with simple message",
			msg:      "test error message",
			args:     nil,
			expected: "test error message",
		},
		{
			name:     "error with arguments",
			msg:      "error with data",
			args:     []any{"err", "database connection failed", "code", 500},
			expected: "error with data",
		},
		{
			name:     "error with empty message",
			msg:      "",
			args:     nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
				Level: slog.LevelError,
			})
			logger := slog.New(handler)
			slog.SetDefault(logger)

			slogLogger := &SlogLogger{}
			slogLogger.Error(tt.msg, tt.args...)

			var logEntry map[string]interface{}
			err := json.Unmarshal(buf.Bytes(), &logEntry)
			if err != nil {
				t.Fatalf("Failed to unmarshal log entry: %v", err)
			}

			if logEntry["msg"] != tt.expected {
				t.Errorf("Expected message %q, got %q", tt.expected, logEntry["msg"])
			}

			if logEntry["level"] != "ERROR" {
				t.Errorf("Expected level ERROR, got %v", logEntry["level"])
			}

			if len(tt.args) > 0 {
				for i := 0; i < len(tt.args); i += 2 {
					if i+1 < len(tt.args) {
						key := tt.args[i].(string)
						expectedValue := tt.args[i+1]
						actualValue := logEntry[key]

						if expectedValue != actualValue {
							if expectedInt, ok := expectedValue.(int); ok {
								if actualFloat, ok := actualValue.(float64); ok && float64(expectedInt) == actualFloat {
									continue
								}
							}
							t.Errorf("Expected %s=%v, got %v", key, expectedValue, actualValue)
						}
					}
				}
			}
		})
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		expected slog.Level
	}{
		{
			name:     "debug level",
			level:    "DEBUG",
			expected: slog.LevelDebug,
		},
		{
			name:     "info level",
			level:    "INFO",
			expected: slog.LevelInfo,
		},
		{
			name:     "warn level",
			level:    "WARN",
			expected: slog.LevelWarn,
		},
		{
			name:     "error level",
			level:    "ERROR",
			expected: slog.LevelError,
		},
		{
			name:     "invalid level",
			level:    "INVALID",
			expected: slog.LevelInfo,
		},
		{
			name:     "empty level",
			level:    "",
			expected: slog.LevelInfo,
		},
		{
			name:     "lowercase level",
			level:    "debug",
			expected: slog.LevelInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLevel(tt.level)
			if result != tt.expected {
				t.Errorf("Expected level %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name        string
		envLogLevel string
	}{
		{
			name:        "new logger with debug level",
			envLogLevel: "DEBUG",
		},
		{
			name:        "new logger with info level",
			envLogLevel: "INFO",
		},
		{
			name:        "new logger with warn level",
			envLogLevel: "WARN",
		},
		{
			name:        "new logger with error level",
			envLogLevel: "ERROR",
		},
		{
			name:        "new logger with invalid level",
			envLogLevel: "INVALID",
		},
		{
			name:        "new logger without env var",
			envLogLevel: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalEnv := os.Getenv("LOG_LEVEL")
			defer os.Setenv("LOG_LEVEL", originalEnv)

			os.Setenv("LOG_LEVEL", tt.envLogLevel)

			logger := NewLogger()
			if logger == nil {
				t.Error("Expected logger to be created, got nil")
			}
		})
	}
}
