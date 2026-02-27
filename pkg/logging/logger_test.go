package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	l := New("engine")
	if l == nil {
		t.Fatal("New returned nil")
	}
	if l.component != "engine" {
		t.Errorf("expected component 'engine', got %q", l.component)
	}
}

func TestLogger_Sub(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithOptions("engine", &Options{
		Output:  &buf,
		NoColor: true,
	})

	sub := l.Sub("bluegreen")
	if sub.component != "engine.bluegreen" {
		t.Errorf("expected component 'engine.bluegreen', got %q", sub.component)
	}

	sub.Info("Tearing down old deployment", "id", "old-123")
	got := buf.String()
	if !strings.Contains(got, "engine.bluegreen:") {
		t.Errorf("expected sub-component prefix, got: %s", got)
	}
	if !strings.Contains(got, "Tearing down old deployment") {
		t.Errorf("expected message, got: %s", got)
	}
	if !strings.Contains(got, "id=old-123") {
		t.Errorf("expected attr, got: %s", got)
	}
}

func TestLogger_With(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithOptions("agent", &Options{
		Output:  &buf,
		NoColor: true,
	})

	taskLog := l.With("task_id", "task-abc")
	taskLog.Info("Pulling image", "image", "nginx:latest")

	got := buf.String()
	if !strings.Contains(got, "task_id=task-abc") {
		t.Errorf("expected with-attr, got: %s", got)
	}
	if !strings.Contains(got, "image=nginx:latest") {
		t.Errorf("expected message attr, got: %s", got)
	}
}

func TestLogger_Levels(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithOptions("test", &Options{
		Output:  &buf,
		NoColor: true,
		Level:   slog.LevelDebug,
	})

	l.Debug("debug msg")
	l.Info("info msg")
	l.Warn("warn msg")
	l.Error("error msg")

	got := buf.String()
	if !strings.Contains(got, "DEBUG") {
		t.Errorf("expected DEBUG, got: %s", got)
	}
	if !strings.Contains(got, "INFO") {
		t.Errorf("expected INFO, got: %s", got)
	}
	if !strings.Contains(got, "WARN") {
		t.Errorf("expected WARN, got: %s", got)
	}
	if !strings.Contains(got, "ERROR") {
		t.Errorf("expected ERROR, got: %s", got)
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithOptions("test", &Options{
		Output:  &buf,
		NoColor: true,
		Level:   slog.LevelWarn,
	})

	l.Debug("should not appear")
	l.Info("should not appear")
	l.Warn("should appear")
	l.Error("should appear")

	got := buf.String()
	if strings.Contains(got, "should not appear") {
		t.Errorf("debug/info should be filtered out, got: %s", got)
	}
	if !strings.Contains(got, "should appear") {
		t.Errorf("warn/error should appear, got: %s", got)
	}
}

func TestNewWithOptions_JSON(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithOptions("engine", &Options{
		Output: &buf,
		Format: FormatJSON,
	})

	l.Info("Starting OCI registry", "port", 5000)

	got := buf.String()
	if !strings.Contains(got, `"msg":"Starting OCI registry"`) {
		t.Errorf("expected JSON msg field, got: %s", got)
	}
	if !strings.Contains(got, `"port":5000`) {
		t.Errorf("expected JSON port field, got: %s", got)
	}
	if !strings.Contains(got, `"component":"engine"`) {
		t.Errorf("expected JSON component field, got: %s", got)
	}
}

func TestSetup_Text(t *testing.T) {
	var buf bytes.Buffer
	Setup(&Options{
		Output:  &buf,
		NoColor: true,
	})

	Info("test setup message")

	got := buf.String()
	if !strings.Contains(got, "test setup message") {
		t.Errorf("expected message from package-level Info, got: %s", got)
	}

	// Restore default
	Setup(nil)
}

func TestSetup_JSON(t *testing.T) {
	var buf bytes.Buffer
	Setup(&Options{
		Output: &buf,
		Format: FormatJSON,
	})

	Info("json test")

	got := buf.String()
	if !strings.Contains(got, `"msg":"json test"`) {
		t.Errorf("expected JSON output, got: %s", got)
	}

	// Restore default
	Setup(nil)
}

func TestPackageLevelFunctions(t *testing.T) {
	var buf bytes.Buffer
	Setup(&Options{
		Output:  &buf,
		NoColor: true,
		Level:   slog.LevelDebug,
	})

	Debug("debug")
	Info("info")
	Warn("warn")
	Error("error")

	got := buf.String()
	for _, level := range []string{"debug", "info", "warn", "error"} {
		if !strings.Contains(got, level) {
			t.Errorf("expected %q in output, got: %s", level, got)
		}
	}

	// Restore default
	Setup(nil)
}
