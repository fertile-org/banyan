package dashboard

import (
	"strings"
	"testing"
	"time"
)

func TestRenderEventList_NilData(t *testing.T) {
	got := renderEventList(nil, 120, 0)
	if !strings.Contains(got, "Loading") {
		t.Error("nil data should show loading")
	}
}

func TestRenderEventList_Empty(t *testing.T) {
	data := &DashboardData{}
	got := renderEventList(data, 120, 0)
	if !strings.Contains(got, "No events yet") {
		t.Error("empty should show no events yet")
	}
}

func TestRenderEventList_WithData(t *testing.T) {
	now := time.Now()
	data := &DashboardData{
		Events: []EventData{
			{Timestamp: now, Type: "deployment.created", Message: "Created deployment my-app", Severity: "info"},
			{Timestamp: now.Add(-time.Minute), Type: "agent.registered", Message: "Agent worker-1 registered", Severity: "info"},
			{Timestamp: now.Add(-2 * time.Minute), Type: "container.failed", Message: "Container api-0 exited with code 1", Severity: "error"},
		},
	}

	got := renderEventList(data, 140, 0)

	if !strings.Contains(got, "Events (3)") {
		t.Error("event list missing count")
	}
	if !strings.Contains(got, "deployment.created") {
		t.Error("event list missing deployment.created type")
	}
	if !strings.Contains(got, "agent.registered") {
		t.Error("event list missing agent.registered type")
	}
	if !strings.Contains(got, "container.failed") {
		t.Error("event list missing container.failed type")
	}
	if !strings.Contains(got, "Created deployment my-app") {
		t.Error("event list missing event message")
	}
}

func TestRenderEventList_CursorPosition(t *testing.T) {
	now := time.Now()
	data := &DashboardData{
		Events: []EventData{
			{Timestamp: now, Type: "event-a", Message: "First event", Severity: "info"},
			{Timestamp: now.Add(-time.Minute), Type: "event-b", Message: "Second event", Severity: "info"},
		},
	}

	got := renderEventList(data, 120, 1)
	lines := strings.Split(got, "\n")

	foundCursorOnB := false
	for _, l := range lines {
		if strings.Contains(l, "event-b") && strings.Contains(l, ">") {
			foundCursorOnB = true
		}
	}
	if !foundCursorOnB {
		t.Error("cursor should be on event-b when cursor=1")
	}
}

func TestRenderEventList_SeverityColors(t *testing.T) {
	now := time.Now()
	data := &DashboardData{
		Events: []EventData{
			{Timestamp: now, Type: "test.error", Message: "Something broke", Severity: "error"},
			{Timestamp: now, Type: "test.warning", Message: "Watch out", Severity: "warning"},
			{Timestamp: now, Type: "test.info", Message: "All good", Severity: "info"},
		},
	}

	got := renderEventList(data, 140, -1)

	if !strings.Contains(got, "error") {
		t.Error("event list missing error severity")
	}
	if !strings.Contains(got, "warning") {
		t.Error("event list missing warning severity")
	}
	if !strings.Contains(got, "info") {
		t.Error("event list missing info severity")
	}
}

func TestSeverityLabel(t *testing.T) {
	tests := []struct {
		severity string
		want     string
	}{
		{"error", "error"},
		{"warning", "warning"},
		{"info", "info"},
		{"", ""},
	}

	for _, tt := range tests {
		got := severityLabel(tt.severity)
		if !strings.Contains(got, tt.want) {
			t.Errorf("severityLabel(%q) = %q, want to contain %q", tt.severity, got, tt.want)
		}
	}
}
