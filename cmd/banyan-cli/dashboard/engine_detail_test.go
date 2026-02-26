package dashboard

import (
	"strings"
	"testing"
	"time"
)

func TestRenderEngineDetail_NilData(t *testing.T) {
	got := renderEngineDetail(nil, 100)
	if !strings.Contains(got, "Loading") {
		t.Error("nil data should show loading")
	}
}

func TestRenderEngineDetail_WithData(t *testing.T) {
	data := &DashboardData{
		Engine: EngineData{
			Status:    "running",
			StartedAt: time.Now().Add(-5 * time.Hour),
			CPU:       0.35,
			MemUsed:   512 * 1024 * 1024,
			MemTotal:  2048 * 1024 * 1024,
			DiskUsed:  10 * 1024 * 1024 * 1024,
			DiskTotal: 50 * 1024 * 1024 * 1024,
			CPUCores:  4,
		},
		Summary: ClusterSummary{
			TotalAgents:        3,
			ConnectedAgents:    2,
			TotalDeployments:   5,
			RunningDeployments: 3,
			TotalContainers:    10,
			HealthyContainers:  8,
			CompletedTasks:     45,
			FailedTasks:        2,
		},
		Events: []EventData{
			{Timestamp: time.Now().Add(-5 * time.Minute), Type: "agent.registered", Message: "Agent worker-1 registered", Severity: "info"},
		},
	}

	got := renderEngineDetail(data, 120)

	// Engine card
	if !strings.Contains(got, "Engine") {
		t.Error("engine detail missing Engine title")
	}
	if !strings.Contains(got, "Running") {
		t.Error("engine detail missing status")
	}
	if !strings.Contains(got, "CPU") {
		t.Error("engine detail missing CPU")
	}
	if !strings.Contains(got, "4 cores") {
		t.Error("engine detail missing core count")
	}
	if !strings.Contains(got, "Mem") {
		t.Error("engine detail missing Mem")
	}
	if !strings.Contains(got, "Disk") {
		t.Error("engine detail missing Disk")
	}

	// Cluster summary
	if !strings.Contains(got, "Cluster Summary") {
		t.Error("engine detail missing cluster summary")
	}
	if !strings.Contains(got, "2/3") {
		t.Error("engine detail missing agent count")
	}
	if !strings.Contains(got, "3 running") {
		t.Error("engine detail missing deployment count")
	}
	if !strings.Contains(got, "45 completed") {
		t.Error("engine detail missing task count")
	}

	// Events
	if !strings.Contains(got, "Recent Events") {
		t.Error("engine detail missing events")
	}
	if !strings.Contains(got, "worker-1") {
		t.Error("engine detail missing event message")
	}
}

func TestRenderEventsBoxExpanded_Empty(t *testing.T) {
	got := renderEventsBoxExpanded(nil, 80)
	if !strings.Contains(got, "No events") {
		t.Error("empty events should show no events message")
	}
}

func TestRenderEventsBoxExpanded_TruncatesAtFifteen(t *testing.T) {
	var events []EventData
	for i := range 20 {
		events = append(events, EventData{
			Timestamp: time.Now().Add(-time.Duration(i) * time.Minute),
			Type:      "test",
			Message:   "Event",
			Severity:  "info",
		})
	}

	got := renderEventsBoxExpanded(events, 80)
	lines := strings.Split(got, "\n")
	// top border + 15 event lines + bottom border = 17
	if len(lines) != 17 {
		t.Errorf("expanded events lines = %d, want 17", len(lines))
	}
}
