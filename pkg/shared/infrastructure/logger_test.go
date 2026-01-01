package infrastructure

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestNewLogger(t *testing.T) {
	t.Run("creates logger with default config", func(t *testing.T) {
		logger := NewLogger(LoggerConfig{})
		if logger == nil {
			t.Error("expected non-nil logger")
		}
	})

	t.Run("creates logger with JSON format", func(t *testing.T) {
		logger := NewLogger(LoggerConfig{
			Level:  LogLevelDebug,
			Format: "json",
		})
		if logger == nil {
			t.Error("expected non-nil logger")
		}
	})

	t.Run("creates logger with text format", func(t *testing.T) {
		logger := NewLogger(LoggerConfig{
			Level:  LogLevelInfo,
			Format: "text",
		})
		if logger == nil {
			t.Error("expected non-nil logger")
		}
	})
}

func TestLoggerLevels(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected logrus.Level
	}{
		{LogLevelDebug, logrus.DebugLevel},
		{LogLevelInfo, logrus.InfoLevel},
		{LogLevelWarn, logrus.WarnLevel},
		{LogLevelError, logrus.ErrorLevel},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			logger := NewLogger(LoggerConfig{Level: tt.level})
			impl, ok := logger.(*logrusLogger)
			if !ok {
				t.Fatal("expected logrusLogger implementation")
			}
			if impl.logger.Logger.Level != tt.expected {
				t.Errorf("expected level %v, got %v", tt.expected, impl.logger.Logger.Level)
			}
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected LogLevel
	}{
		{"debug", LogLevelDebug},
		{"info", LogLevelInfo},
		{"warn", LogLevelWarn},
		{"warning", LogLevelWarn},
		{"error", LogLevelError},
		{"invalid", LogLevelInfo}, // defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseLogLevel(tt.input)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LogLevelDebug, "debug"},
		{LogLevelInfo, "info"},
		{LogLevelWarn, "warn"},
		{LogLevelError, "error"},
		{LogLevel(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.level.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.level.String())
			}
		})
	}
}

func TestLoggerWithFields(t *testing.T) {
	logger := NewLogger(LoggerConfig{Level: LogLevelDebug})

	fieldLogger := logger.WithFields(F("key1", "value1"), F("key2", 42))
	if fieldLogger == nil {
		t.Error("expected non-nil logger")
	}

	// Should be able to chain
	chainedLogger := fieldLogger.WithFields(F("key3", true))
	if chainedLogger == nil {
		t.Error("expected non-nil chained logger")
	}
}

func TestLoggerWithContext(t *testing.T) {
	logger := NewLogger(LoggerConfig{Level: LogLevelDebug})

	ctx := context.Background()
	ctxLogger := logger.WithContext(ctx)
	if ctxLogger == nil {
		t.Error("expected non-nil logger")
	}
}

func TestLoggerWithComponent(t *testing.T) {
	logger := NewLogger(LoggerConfig{Level: LogLevelDebug})

	compLogger := logger.WithComponent("test-component")
	if compLogger == nil {
		t.Error("expected non-nil logger")
	}
}

func TestLoggerOutput(t *testing.T) {
	// Create a buffer to capture output
	var buf bytes.Buffer

	// Create logger with custom output
	logger := NewLogger(LoggerConfig{
		Level:  LogLevelDebug,
		Format: "text",
		Output: &buf,
	})

	t.Run("Debug", func(t *testing.T) {
		buf.Reset()
		logger.Debug("debug message", F("key", "value"))
		output := buf.String()
		if !strings.Contains(output, "debug message") {
			t.Errorf("expected debug message in output: %s", output)
		}
	})

	t.Run("Info", func(t *testing.T) {
		buf.Reset()
		logger.Info("info message")
		output := buf.String()
		if !strings.Contains(output, "info message") {
			t.Errorf("expected info message in output: %s", output)
		}
	})

	t.Run("Warn", func(t *testing.T) {
		buf.Reset()
		logger.Warn("warn message")
		output := buf.String()
		if !strings.Contains(output, "warn message") {
			t.Errorf("expected warn message in output: %s", output)
		}
	})

	t.Run("Error", func(t *testing.T) {
		buf.Reset()
		logger.Error("error message")
		output := buf.String()
		if !strings.Contains(output, "error message") {
			t.Errorf("expected error message in output: %s", output)
		}
	})
}

func TestField(t *testing.T) {
	field := F("test_key", "test_value")

	if field.Key != "test_key" {
		t.Errorf("expected key 'test_key', got '%s'", field.Key)
	}

	if field.Value != "test_value" {
		t.Errorf("expected value 'test_value', got '%v'", field.Value)
	}
}

func TestFieldConstructors(t *testing.T) {
	tests := []struct {
		name     string
		field    Field
		key      string
		valueStr string
	}{
		{"FDeploymentID", FDeploymentID("dep-123"), "deployment_id", "dep-123"},
		{"FAgentID", FAgentID("agent-123"), "agent_id", "agent-123"},
		{"FTaskID", FTaskID("task-123"), "task_id", "task-123"},
		{"FContainerID", FContainerID("ctr-123"), "container_id", "ctr-123"},
		{"FServiceID", FServiceID("svc-123"), "service_id", "svc-123"},
		{"FStatus", FStatus("running"), "status", "running"},
		{"FComponent", FComponent("engine"), "component", "engine"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.field.Key != tt.key {
				t.Errorf("expected key %s, got %s", tt.key, tt.field.Key)
			}
			if tt.field.Value != tt.valueStr {
				t.Errorf("expected value %s, got %v", tt.valueStr, tt.field.Value)
			}
		})
	}
}

func TestNopLogger(t *testing.T) {
	logger := &NopLogger{}

	// All methods should work without panicking
	logger.Debug("test")
	logger.Info("test")
	logger.Warn("test")
	logger.Error("test")

	if logger.WithFields(F("key", "value")) == nil {
		t.Error("expected non-nil logger from WithFields")
	}

	if logger.WithContext(context.Background()) == nil {
		t.Error("expected non-nil logger from WithContext")
	}

	if logger.WithComponent("comp") == nil {
		t.Error("expected non-nil logger from WithComponent")
	}
}

func TestDefaultLoggerConfig(t *testing.T) {
	cfg := DefaultLoggerConfig()

	if cfg.Level != LogLevelInfo {
		t.Errorf("expected level Info, got %v", cfg.Level)
	}
	if cfg.Format != "json" {
		t.Errorf("expected format json, got %s", cfg.Format)
	}
}

func TestNewDefaultLogger(t *testing.T) {
	logger := NewDefaultLogger()
	if logger == nil {
		t.Error("expected non-nil logger")
	}
}

func TestContextWithLogger(t *testing.T) {
	logger := NewDefaultLogger()
	ctx := ContextWithLogger(context.Background(), logger)

	retrieved := LoggerFromContext(ctx)
	if retrieved == nil {
		t.Error("expected non-nil logger from context")
	}
}

func TestLoggerFromContextDefault(t *testing.T) {
	ctx := context.Background()
	logger := LoggerFromContext(ctx)
	if logger == nil {
		t.Error("expected non-nil default logger")
	}
}
