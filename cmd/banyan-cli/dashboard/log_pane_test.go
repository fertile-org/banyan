package dashboard

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderLogPane(t *testing.T) {
	state := logPaneState{
		containerName: "myapp-web-0",
		lines:         []string{"2026-03-30 INFO starting", "2026-03-30 INFO ready"},
		streaming:     true,
	}
	got := renderLogPane(state, 80, 10)

	if !strings.Contains(got, "myapp-web-0") {
		t.Error("log pane missing container name")
	}
	if !strings.Contains(got, "starting") {
		t.Error("log pane missing log line")
	}
	if !strings.Contains(got, "ready") {
		t.Error("log pane missing second log line")
	}
}

func TestRenderLogPane_Empty(t *testing.T) {
	state := logPaneState{
		containerName: "myapp-web-0",
		streaming:     true,
	}
	got := renderLogPane(state, 80, 10)

	if !strings.Contains(got, "Waiting for logs") {
		t.Error("empty log pane should show waiting message")
	}
}

func TestRenderLogPane_Error(t *testing.T) {
	state := logPaneState{
		containerName: "myapp-web-0",
		err:           errors.New("connection refused"),
	}
	got := renderLogPane(state, 80, 10)

	if !strings.Contains(got, "connection refused") {
		t.Error("error log pane should show error message")
	}
}

func TestRenderLogPane_ManyLines(t *testing.T) {
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, "log line")
	}
	state := logPaneState{
		containerName: "test",
		lines:         lines,
		streaming:     false,
	}
	got := renderLogPane(state, 80, 10)

	// Should not crash and should produce output
	if !strings.Contains(got, "test") {
		t.Error("log pane missing container name")
	}
}
