// Package infrastructure provides shared infrastructure components used across Engine and Agent.
package infrastructure

import (
	"context"
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

// LogLevel represents logging severity levels.
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "debug"
	case LogLevelInfo:
		return "info"
	case LogLevelWarn:
		return "warn"
	case LogLevelError:
		return "error"
	default:
		return "unknown"
	}
}

// ParseLogLevel parses a log level string.
func ParseLogLevel(s string) LogLevel {
	switch s {
	case "debug":
		return LogLevelDebug
	case "info":
		return LogLevelInfo
	case "warn", "warning":
		return LogLevelWarn
	case "error":
		return LogLevelError
	default:
		return LogLevelInfo
	}
}

// Logger defines the interface for structured logging.
type Logger interface {
	// Debug logs a message at debug level.
	Debug(msg string, fields ...Field)
	// Info logs a message at info level.
	Info(msg string, fields ...Field)
	// Warn logs a message at warn level.
	Warn(msg string, fields ...Field)
	// Error logs a message at error level.
	Error(msg string, fields ...Field)

	// WithFields returns a new logger with additional fields.
	WithFields(fields ...Field) Logger
	// WithContext returns a new logger with context values.
	WithContext(ctx context.Context) Logger
	// WithComponent returns a new logger for a specific component.
	WithComponent(component string) Logger
}

// Field represents a key-value pair for structured logging.
type Field struct {
	Value any
	Key   string
}

// F creates a new Field.
func F(key string, value any) Field {
	return Field{Key: key, Value: value}
}

// Common field constructors for consistency.
func FDeploymentID(id string) Field     { return Field{Key: "deployment_id", Value: id} }
func FAgentID(id string) Field          { return Field{Key: "agent_id", Value: id} }
func FTaskID(id string) Field           { return Field{Key: "task_id", Value: id} }
func FContainerID(id string) Field      { return Field{Key: "container_id", Value: id} }
func FServiceID(id string) Field        { return Field{Key: "service_id", Value: id} }
func FError(err error) Field            { return Field{Key: "error", Value: err} }
func FDuration(d any) Field             { return Field{Key: "duration", Value: d} }
func FStatus(status string) Field       { return Field{Key: "status", Value: status} }
func FComponent(component string) Field { return Field{Key: "component", Value: component} }

// LoggerConfig configures the logger.
type LoggerConfig struct {
	Output     io.Writer
	Format     string
	TimeFormat string
	Level      LogLevel
}

// DefaultLoggerConfig returns the default logger configuration.
func DefaultLoggerConfig() LoggerConfig {
	return LoggerConfig{
		Level:      LogLevelInfo,
		Format:     "json",
		Output:     os.Stdout,
		TimeFormat: "2006-01-02T15:04:05.000Z07:00",
	}
}

// logrusLogger implements Logger using logrus.
type logrusLogger struct {
	logger *logrus.Entry
}

// NewLogger creates a new logger with the given configuration.
func NewLogger(config LoggerConfig) Logger {
	log := logrus.New()

	// Set level
	switch config.Level {
	case LogLevelDebug:
		log.SetLevel(logrus.DebugLevel)
	case LogLevelInfo:
		log.SetLevel(logrus.InfoLevel)
	case LogLevelWarn:
		log.SetLevel(logrus.WarnLevel)
	case LogLevelError:
		log.SetLevel(logrus.ErrorLevel)
	}

	// Set formatter
	if config.Format == "json" {
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: config.TimeFormat,
		})
	} else {
		log.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: config.TimeFormat,
			FullTimestamp:   true,
		})
	}

	// Set output
	if config.Output != nil {
		log.SetOutput(config.Output)
	}

	return &logrusLogger{logger: logrus.NewEntry(log)}
}

// NewDefaultLogger creates a logger with default configuration.
func NewDefaultLogger() Logger {
	return NewLogger(DefaultLoggerConfig())
}

func (l *logrusLogger) Debug(msg string, fields ...Field) {
	l.logger.WithFields(toLogrusFields(fields)).Debug(msg)
}

func (l *logrusLogger) Info(msg string, fields ...Field) {
	l.logger.WithFields(toLogrusFields(fields)).Info(msg)
}

func (l *logrusLogger) Warn(msg string, fields ...Field) {
	l.logger.WithFields(toLogrusFields(fields)).Warn(msg)
}

func (l *logrusLogger) Error(msg string, fields ...Field) {
	l.logger.WithFields(toLogrusFields(fields)).Error(msg)
}

func (l *logrusLogger) WithFields(fields ...Field) Logger {
	return &logrusLogger{
		logger: l.logger.WithFields(toLogrusFields(fields)),
	}
}

func (l *logrusLogger) WithContext(ctx context.Context) Logger {
	return &logrusLogger{
		logger: l.logger.WithContext(ctx),
	}
}

func (l *logrusLogger) WithComponent(component string) Logger {
	return l.WithFields(FComponent(component))
}

func toLogrusFields(fields []Field) logrus.Fields {
	lf := make(logrus.Fields, len(fields))
	for _, f := range fields {
		lf[f.Key] = f.Value
	}
	return lf
}

// NopLogger is a no-operation logger for testing.
type NopLogger struct{}

func (NopLogger) Debug(msg string, fields ...Field)        {}
func (NopLogger) Info(msg string, fields ...Field)         {}
func (NopLogger) Warn(msg string, fields ...Field)         {}
func (NopLogger) Error(msg string, fields ...Field)        {}
func (n NopLogger) WithFields(fields ...Field) Logger      { return n }
func (n NopLogger) WithContext(ctx context.Context) Logger { return n }
func (n NopLogger) WithComponent(component string) Logger  { return n }

// contextKey is a type for context keys to avoid collisions.
type contextKey string

const loggerContextKey contextKey = "logger"

// ContextWithLogger adds a logger to a context.
func ContextWithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey, logger)
}

// LoggerFromContext retrieves a logger from context, or returns a default.
func LoggerFromContext(ctx context.Context) Logger {
	if logger, ok := ctx.Value(loggerContextKey).(Logger); ok {
		return logger
	}
	return NewDefaultLogger()
}
