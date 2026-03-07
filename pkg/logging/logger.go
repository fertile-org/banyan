package logging

import (
	"context"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
)

// Logger is a structured logger for a specific component.
type Logger struct {
	slog      *slog.Logger
	component string
}

// Options configures the global logging setup.
type Options struct {
	Level   slog.Level // default: Info
	Format  Format     // FormatText (colored) or FormatJSON
	Output  io.Writer  // default: os.Stderr
	NoColor bool       // disable ANSI (auto-detected from NO_COLOR env)
}

// defaultLogger is the global default logger.
var defaultLogger = New("banyan")

// Setup configures the global default logger from options and environment variables.
// Environment variables override programmatic options:
//   - BANYAN_LOG_LEVEL: debug, info, warn, error
//   - BANYAN_LOG_FORMAT: text, json
//   - NO_COLOR: any value disables color
func Setup(opts *Options) {
	if opts == nil {
		opts = &Options{}
	}

	// Apply environment overrides
	if envLevel := os.Getenv("BANYAN_LOG_LEVEL"); envLevel != "" {
		switch strings.ToLower(envLevel) {
		case "debug":
			opts.Level = slog.LevelDebug
		case "info":
			opts.Level = slog.LevelInfo
		case "warn", "warning":
			opts.Level = slog.LevelWarn
		case "error":
			opts.Level = slog.LevelError
		}
	}

	if envFormat := os.Getenv("BANYAN_LOG_FORMAT"); envFormat != "" {
		switch strings.ToLower(envFormat) {
		case "json":
			opts.Format = FormatJSON
		case "text":
			opts.Format = FormatText
		}
	}

	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		opts.NoColor = true
	}

	out := opts.Output
	if out == nil {
		out = os.Stderr
	}

	if opts.Format == FormatJSON {
		handler := slog.NewJSONHandler(out, &slog.HandlerOptions{
			Level: opts.Level,
		})
		defaultLogger = &Logger{
			slog: slog.New(handler),
		}
	} else {
		handler := NewColorHandler(&ColorHandlerOptions{
			Level:   opts.Level,
			Output:  out,
			NoColor: opts.NoColor,
		})
		defaultLogger = &Logger{
			slog: slog.New(handler),
		}
	}
}

// New creates a Logger for the given component.
func New(component string) *Logger {
	handler := NewColorHandler(&ColorHandlerOptions{
		Level:     slog.LevelInfo,
		Output:    os.Stderr,
		NoColor:   isNoColor(),
		Component: component,
	})
	return &Logger{
		slog:      slog.New(handler),
		component: component,
	}
}

// NewWithOptions creates a Logger for the given component with custom options.
func NewWithOptions(component string, opts *Options) *Logger {
	if opts == nil {
		opts = &Options{}
	}

	out := opts.Output
	if out == nil {
		out = os.Stderr
	}

	noColor := opts.NoColor
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		noColor = true
	}

	if opts.Format == FormatJSON {
		handler := slog.NewJSONHandler(out, &slog.HandlerOptions{
			Level: opts.Level,
		})
		return &Logger{
			slog:      slog.New(handler).With("component", component),
			component: component,
		}
	}

	handler := NewColorHandler(&ColorHandlerOptions{
		Level:     opts.Level,
		Output:    out,
		NoColor:   noColor,
		Component: component,
	})
	return &Logger{
		slog:      slog.New(handler),
		component: component,
	}
}

// Sub creates a sub-component logger (e.g., "engine" → "engine.bluegreen").
func (l *Logger) Sub(name string) *Logger {
	subComponent := l.component + "." + name
	// Clone the handler with the new component
	if ch, ok := l.slog.Handler().(*ColorHandler); ok {
		newHandler := &ColorHandler{
			out:       ch.out,
			level:     ch.level,
			noColor:   ch.noColor,
			component: subComponent,
			attrs:     ch.attrs,
			groups:    ch.groups,
		}
		return &Logger{
			slog:      slog.New(newHandler),
			component: subComponent,
		}
	}
	// For JSON handler, use With
	return &Logger{
		slog:      l.slog.With("component", subComponent),
		component: subComponent,
	}
}

// With returns a new Logger with the given key-value pairs attached.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		slog:      l.slog.With(args...),
		component: l.component,
	}
}

// Debug logs at DEBUG level.
func (l *Logger) Debug(msg string, args ...any) {
	l.slog.Debug(msg, args...)
}

// Info logs at INFO level.
func (l *Logger) Info(msg string, args ...any) {
	l.slog.Info(msg, args...)
}

// Warn logs at WARN level.
func (l *Logger) Warn(msg string, args ...any) {
	l.slog.Warn(msg, args...)
}

// Error logs at ERROR level.
func (l *Logger) Error(msg string, args ...any) {
	l.slog.Error(msg, args...)
}

// InfoContext logs at INFO level with context.
func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.slog.InfoContext(ctx, msg, args...)
}

// WarnContext logs at WARN level with context.
func (l *Logger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.slog.WarnContext(ctx, msg, args...)
}

// ErrorContext logs at ERROR level with context.
func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.slog.ErrorContext(ctx, msg, args...)
}

// --- Package-level functions (use global default logger) ---

// Info logs at INFO level using the default logger.
func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

// Warn logs at WARN level using the default logger.
func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

// Error logs at ERROR level using the default logger.
func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}

// Debug logs at DEBUG level using the default logger.
func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}

// StdLogger returns a *log.Logger that writes to this Logger at DEBUG level.
// Useful for bridging libraries that accept *log.Logger (e.g., OCI registry).
func (l *Logger) StdLogger() *log.Logger {
	return log.New(&logWriter{logger: l}, "", 0)
}

// logWriter adapts a Logger to io.Writer for use with *log.Logger.
type logWriter struct {
	logger *Logger
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		w.logger.Debug(msg)
	}
	return len(p), nil
}

func isNoColor() bool {
	_, ok := os.LookupEnv("NO_COLOR")
	return ok
}
