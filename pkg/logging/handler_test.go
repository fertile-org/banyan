package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func newRecord(t time.Time, level slog.Level, msg string) slog.Record {
	return slog.NewRecord(t, level, msg, 0)
}

func TestColorHandler_Handle(t *testing.T) {
	var buf bytes.Buffer
	h := NewColorHandler(&ColorHandlerOptions{
		Level:     slog.LevelDebug,
		Output:    &buf,
		NoColor:   true,
		Component: "engine",
	})

	r := newRecord(time.Date(2024, 1, 10, 10, 30, 0, 0, time.UTC), slog.LevelInfo, "Starting OCI registry")
	r.AddAttrs(slog.Int("port", 5000))

	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "10:30:00") {
		t.Errorf("expected time in output, got: %s", got)
	}
	if !strings.Contains(got, "INFO") {
		t.Errorf("expected INFO level, got: %s", got)
	}
	if !strings.Contains(got, "engine:") {
		t.Errorf("expected component prefix, got: %s", got)
	}
	if !strings.Contains(got, "Starting OCI registry") {
		t.Errorf("expected message, got: %s", got)
	}
	if !strings.Contains(got, "port=5000") {
		t.Errorf("expected port attr, got: %s", got)
	}
}

func TestColorHandler_Levels(t *testing.T) {
	tests := []struct {
		level    slog.Level
		wantTag  string
		wantAnsi string
	}{
		{slog.LevelDebug, "DEBUG", colorGray},
		{slog.LevelInfo, "INFO", colorGreen},
		{slog.LevelWarn, "WARN", colorYellow},
		{slog.LevelError, "ERROR", colorRed},
	}

	for _, tt := range tests {
		t.Run(tt.wantTag, func(t *testing.T) {
			var buf bytes.Buffer
			h := NewColorHandler(&ColorHandlerOptions{
				Level:   slog.LevelDebug,
				Output:  &buf,
				NoColor: false,
			})

			r := newRecord(time.Now(), tt.level, "test")
			if err := h.Handle(context.Background(), r); err != nil {
				t.Fatalf("Handle: %v", err)
			}

			got := buf.String()
			if !strings.Contains(got, tt.wantTag) {
				t.Errorf("expected %s in output, got: %s", tt.wantTag, got)
			}
			if !strings.Contains(got, tt.wantAnsi) {
				t.Errorf("expected ANSI code %q in output, got: %s", tt.wantAnsi, got)
			}
		})
	}
}

func TestColorHandler_NoColor(t *testing.T) {
	var buf bytes.Buffer
	h := NewColorHandler(&ColorHandlerOptions{
		Level:   slog.LevelInfo,
		Output:  &buf,
		NoColor: true,
	})

	r := newRecord(time.Now(), slog.LevelInfo, "test message")
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := buf.String()
	if strings.Contains(got, "\033[") {
		t.Errorf("expected no ANSI codes with NoColor, got: %s", got)
	}
}

func TestColorHandler_Enabled(t *testing.T) {
	h := NewColorHandler(&ColorHandlerOptions{
		Level:  slog.LevelWarn,
		Output: &bytes.Buffer{},
	})

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("DEBUG should not be enabled when level is WARN")
	}
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("INFO should not be enabled when level is WARN")
	}
	if !h.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("WARN should be enabled when level is WARN")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("ERROR should be enabled when level is WARN")
	}
}

func TestColorHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := NewColorHandler(&ColorHandlerOptions{
		Level:   slog.LevelInfo,
		Output:  &buf,
		NoColor: true,
	})

	h2 := h.WithAttrs([]slog.Attr{slog.String("task_id", "abc-123")})

	r := newRecord(time.Now(), slog.LevelInfo, "pulling image")
	if err := h2.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "task_id=abc-123") {
		t.Errorf("expected pre-set attr, got: %s", got)
	}
}

func TestColorHandler_StringQuoting(t *testing.T) {
	var buf bytes.Buffer
	h := NewColorHandler(&ColorHandlerOptions{
		Level:   slog.LevelInfo,
		Output:  &buf,
		NoColor: true,
	})

	r := newRecord(time.Now(), slog.LevelInfo, "test")
	r.AddAttrs(slog.String("error", "image pull failed"))

	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, `error="image pull failed"`) {
		t.Errorf("expected quoted string value, got: %s", got)
	}
}

func TestColorHandler_SimpleString(t *testing.T) {
	var buf bytes.Buffer
	h := NewColorHandler(&ColorHandlerOptions{
		Level:   slog.LevelInfo,
		Output:  &buf,
		NoColor: true,
	})

	r := newRecord(time.Now(), slog.LevelInfo, "test")
	r.AddAttrs(slog.String("name", "my-app"))

	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "name=my-app") {
		t.Errorf("expected unquoted string value, got: %s", got)
	}
}

func TestColorHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	h := NewColorHandler(&ColorHandlerOptions{
		Level:   slog.LevelInfo,
		Output:  &buf,
		NoColor: true,
	})

	h2 := h.WithGroup("request")

	r := newRecord(time.Now(), slog.LevelInfo, "test")
	r.AddAttrs(slog.String("id", "req-123"))
	if err := h2.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "request.id=req-123") {
		t.Errorf("expected grouped attr key, got: %s", got)
	}
}

func TestColorHandler_NoComponent(t *testing.T) {
	var buf bytes.Buffer
	h := NewColorHandler(&ColorHandlerOptions{
		Level:   slog.LevelInfo,
		Output:  &buf,
		NoColor: true,
	})

	r := newRecord(time.Now(), slog.LevelInfo, "test message")
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "test message") {
		t.Errorf("expected message, got: %s", got)
	}
}
