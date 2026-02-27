// Package logging provides structured colored logging for Banyan components.
// It wraps log/slog with a custom handler that adds colors and component prefixes.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// Format determines the log output format.
type Format int

const (
	// FormatText produces human-readable colored output.
	FormatText Format = iota
	// FormatJSON produces structured JSON output.
	FormatJSON
)

// ANSI color codes.
const (
	colorReset  = "\033[0m"
	colorGray   = "\033[90m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
)

// ColorHandler is a custom slog.Handler that produces colored terminal output.
// It formats log records as: TIME LEVEL component: message key=value key=value
type ColorHandler struct {
	mu        sync.Mutex
	out       io.Writer
	level     slog.Level
	noColor   bool
	component string // extracted from attrs
	attrs     []slog.Attr
	groups    []string
}

// ColorHandlerOptions configures the ColorHandler.
type ColorHandlerOptions struct {
	Level     slog.Level
	Output    io.Writer
	NoColor   bool
	Component string
}

// NewColorHandler creates a new ColorHandler.
func NewColorHandler(opts *ColorHandlerOptions) *ColorHandler {
	out := opts.Output
	if out == nil {
		out = os.Stderr
	}
	return &ColorHandler{
		out:       out,
		level:     opts.Level,
		noColor:   opts.NoColor,
		component: opts.Component,
	}
}

// Enabled reports whether the handler handles records at the given level.
func (h *ColorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle formats and writes the log record.
func (h *ColorHandler) Handle(_ context.Context, r slog.Record) error {
	timeStr := r.Time.Format(time.TimeOnly) // HH:MM:SS

	levelStr, levelColor := h.formatLevel(r.Level)

	var b strings.Builder

	// Time
	if h.noColor {
		fmt.Fprintf(&b, "%s %s", timeStr, levelStr)
	} else {
		fmt.Fprintf(&b, "%s%s%s %s%s%s", colorGray, timeStr, colorReset, levelColor, levelStr, colorReset)
	}

	// Component
	if h.component != "" {
		fmt.Fprintf(&b, " %s:", h.component)
	}

	// Message
	fmt.Fprintf(&b, " %s", r.Message)

	// Pre-set attrs
	for _, a := range h.attrs {
		h.appendAttr(&b, a)
	}

	// Record attrs
	r.Attrs(func(a slog.Attr) bool {
		h.appendAttr(&b, a)
		return true
	})

	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.out, b.String())
	return err
}

// WithAttrs returns a new handler with the given attrs added.
func (h *ColorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)

	return &ColorHandler{
		out:       h.out,
		level:     h.level,
		noColor:   h.noColor,
		component: h.component,
		attrs:     newAttrs,
		groups:    h.groups,
	}
}

// WithGroup returns a new handler with the given group name.
func (h *ColorHandler) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	return &ColorHandler{
		out:       h.out,
		level:     h.level,
		noColor:   h.noColor,
		component: h.component,
		attrs:     h.attrs,
		groups:    newGroups,
	}
}

func (h *ColorHandler) formatLevel(level slog.Level) (string, string) {
	switch {
	case level >= slog.LevelError:
		return "ERROR", colorRed
	case level >= slog.LevelWarn:
		return "WARN ", colorYellow
	case level >= slog.LevelInfo:
		return "INFO ", colorGreen
	default:
		return "DEBUG", colorGray
	}
}

func (h *ColorHandler) appendAttr(b *strings.Builder, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}

	key := a.Key
	if len(h.groups) > 0 {
		key = strings.Join(h.groups, ".") + "." + key
	}

	val := a.Value.Resolve()
	if val.Kind() == slog.KindString {
		v := val.String()
		if strings.ContainsAny(v, " \t\n\"") {
			fmt.Fprintf(b, "  %s=%q", key, v)
		} else {
			fmt.Fprintf(b, "  %s=%s", key, v)
		}
	} else {
		fmt.Fprintf(b, "  %s=%s", key, val.String())
	}
}
