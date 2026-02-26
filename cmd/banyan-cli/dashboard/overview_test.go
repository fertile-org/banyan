package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderOverview_NilData(t *testing.T) {
	got := renderOverview(nil, 80)
	if !strings.Contains(got, "Loading") {
		t.Error("nil data should show loading message")
	}
}

func TestRenderOverview_WithData(t *testing.T) {
	data := &DashboardData{
		Engine: EngineData{
			Status:    "running",
			StartedAt: time.Now().Add(-2 * time.Hour),
			CPU:       0.35,
			MemUsed:   512 * 1024 * 1024,
			MemTotal:  1024 * 1024 * 1024,
			DiskUsed:  10 * 1024 * 1024 * 1024,
			DiskTotal: 50 * 1024 * 1024 * 1024,
		},
		Agents: []AgentData{
			{
				Name:           "worker-1",
				Status:         "ready",
				Tags:           []string{"us-east"},
				CPU:            0.50,
				MemUsed:        256 * 1024 * 1024,
				MemTotal:       1024 * 1024 * 1024,
				ContainerCount: 5,
			},
		},
		Deployments: []DeploymentData{
			{
				Name:      "my-app",
				Status:    "running",
				Healthy:   3,
				Total:     3,
				Services:  2,
				Tags:      []string{"prod"},
				CreatedAt: time.Now().Add(-30 * time.Minute),
			},
		},
		Summary: ClusterSummary{
			TotalAgents:        1,
			ConnectedAgents:    1,
			RunningDeployments: 1,
			TotalContainers:    5,
			HealthyContainers:  5,
			CompletedTasks:     10,
		},
		Events: []EventData{
			{
				Timestamp: time.Now().Add(-5 * time.Minute),
				Type:      "deployment.created",
				Message:   "Deployed my-app",
				Severity:  "info",
			},
		},
	}

	got := renderOverview(data, 100)

	// Check all sections present
	if !strings.Contains(got, "Engine") {
		t.Error("overview missing Engine card")
	}
	if !strings.Contains(got, "Cluster") {
		t.Error("overview missing Cluster card")
	}
	if !strings.Contains(got, "Agents") {
		t.Error("overview missing Agents table")
	}
	if !strings.Contains(got, "worker-1") {
		t.Error("overview missing agent name")
	}
	if !strings.Contains(got, "Deployments") {
		t.Error("overview missing Deployments table")
	}
	if !strings.Contains(got, "my-app") {
		t.Error("overview missing deployment name")
	}
	if !strings.Contains(got, "Recent Events") {
		t.Error("overview missing Events section")
	}
	if !strings.Contains(got, "Deployed my-app") {
		t.Error("overview missing event message")
	}
}

func TestRenderEngineCard(t *testing.T) {
	e := EngineData{
		Status:    "running",
		StartedAt: time.Now().Add(-time.Hour),
		CPU:       0.45,
		MemUsed:   512 * 1024 * 1024,
		MemTotal:  1024 * 1024 * 1024,
		DiskUsed:  10 * 1024 * 1024 * 1024,
		DiskTotal: 50 * 1024 * 1024 * 1024,
	}

	got := renderEngineCard(&e, 40)
	if !strings.Contains(got, "Engine") {
		t.Error("engine card missing title")
	}
	if !strings.Contains(got, "Running") {
		t.Error("engine card missing status")
	}
	if !strings.Contains(got, "CPU") {
		t.Error("engine card missing CPU label")
	}
	if !strings.Contains(got, "Mem") {
		t.Error("engine card missing Mem label")
	}
	if !strings.Contains(got, "Disk") {
		t.Error("engine card missing Disk label")
	}
}

func TestRenderClusterCard(t *testing.T) {
	s := ClusterSummary{
		TotalAgents:        3,
		ConnectedAgents:    2,
		RunningDeployments: 1,
		TotalContainers:    5,
		HealthyContainers:  5,
		CompletedTasks:     10,
		FailedTasks:        1,
	}

	got := renderClusterCard(s, 40)
	if !strings.Contains(got, "Cluster") {
		t.Error("cluster card missing title")
	}
	if !strings.Contains(got, "2/3") {
		t.Error("cluster card missing agent count")
	}
	if !strings.Contains(got, "5/5") {
		t.Error("cluster card missing container health")
	}
	if !strings.Contains(got, "1 running") {
		t.Error("cluster card missing deployment count")
	}
}

func TestRenderAgentsTable_Empty(t *testing.T) {
	got := renderAgentsTable(nil, 80)
	if !strings.Contains(got, "No agents") {
		t.Error("empty agents should show no agents message")
	}
}

func TestRenderAgentsTable_WithAgents(t *testing.T) {
	agents := []AgentData{
		{
			Name:           "agent-1",
			Status:         "ready",
			Tags:           []string{"gpu"},
			CPU:            0.60,
			MemUsed:        512 * 1024 * 1024,
			MemTotal:       2048 * 1024 * 1024,
			DiskUsed:       10 * 1024 * 1024 * 1024,
			DiskTotal:      50 * 1024 * 1024 * 1024,
			ContainerCount: 3,
		},
		{
			Name:           "agent-2",
			Status:         "stale",
			Tags:           []string{"cpu"},
			CPU:            0.10,
			MemUsed:        128 * 1024 * 1024,
			MemTotal:       1024 * 1024 * 1024,
			DiskUsed:       5 * 1024 * 1024 * 1024,
			DiskTotal:      20 * 1024 * 1024 * 1024,
			ContainerCount: 0,
		},
	}

	got := renderAgentsTable(agents, 120)
	if !strings.Contains(got, "Agents (2)") {
		t.Error("agents table missing count header")
	}
	if !strings.Contains(got, "agent-1") {
		t.Error("agents table missing agent-1")
	}
	if !strings.Contains(got, "agent-2") {
		t.Error("agents table missing agent-2")
	}
	if !strings.Contains(got, "Disk") {
		t.Error("agents table missing Disk column header")
	}
	if !strings.Contains(got, "Containers") {
		t.Error("agents table missing Containers column header")
	}
}

func TestRenderDeploymentsTable_Empty(t *testing.T) {
	got := renderDeploymentsTable(nil, 80)
	if !strings.Contains(got, "No deployments") {
		t.Error("empty deployments should show no deployments message")
	}
}

func TestRenderDeploymentsTable_WithDeploys(t *testing.T) {
	deploys := []DeploymentData{
		{
			Name:      "web-app",
			Status:    "running",
			Healthy:   2,
			Total:     2,
			Services:  2,
			Tags:      []string{"prod"},
			CreatedAt: time.Now().Add(-time.Hour),
		},
	}

	got := renderDeploymentsTable(deploys, 100)
	if !strings.Contains(got, "1 active") {
		t.Error("deployments table missing active count in header")
	}
	if !strings.Contains(got, "web-app") {
		t.Error("deployments table missing deployment name")
	}
}

func TestRenderEventsBox_Empty(t *testing.T) {
	got := renderEventsBox(nil, 80)
	if !strings.Contains(got, "No events") {
		t.Error("empty events should show no events message")
	}
}

func TestRenderEventsBox_WithEvents(t *testing.T) {
	events := []EventData{
		{
			Timestamp: time.Now().Add(-5 * time.Minute),
			Type:      "agent.registered",
			Message:   "Agent worker-1 registered",
			Severity:  "info",
		},
		{
			Timestamp: time.Now().Add(-2 * time.Minute),
			Type:      "container.running",
			Message:   "Container web-0 running",
			Severity:  "info",
		},
	}

	got := renderEventsBox(events, 80)
	if !strings.Contains(got, "Recent Events") {
		t.Error("events box missing title")
	}
	if !strings.Contains(got, "Agent worker-1") {
		t.Error("events box missing event message")
	}
}

func TestRenderEventsBox_TruncatesAtFive(t *testing.T) {
	var events []EventData
	for i := range 8 {
		events = append(events, EventData{
			Timestamp: time.Now().Add(-time.Duration(i) * time.Minute),
			Type:      "test",
			Message:   "Event " + string(rune('A'+i)),
			Severity:  "info",
		})
	}

	got := renderEventsBox(events, 80)
	// Should only contain 5 events (maxEvents) plus box borders
	lines := strings.Split(got, "\n")
	// top border + 5 event lines + bottom border = 7
	if len(lines) != 7 {
		t.Errorf("events box lines = %d, want 7", len(lines))
	}
}

func TestGroupDeployments(t *testing.T) {
	deploys := []DeploymentData{
		{Name: "app-a", Status: "stopped", CreatedAt: time.Now().Add(-2 * time.Hour)},
		{Name: "app-a", Status: "running", CreatedAt: time.Now().Add(-1 * time.Hour)},
		{Name: "app-b", Status: "running", CreatedAt: time.Now().Add(-30 * time.Minute)},
		{Name: "app-a", Status: "stopped", CreatedAt: time.Now().Add(-3 * time.Hour)},
	}

	groups := groupDeployments(deploys)

	if len(groups) != 2 {
		t.Fatalf("groups count = %d, want 2", len(groups))
	}

	// app-b is active and newer, should be first
	// app-a is also active (latest is running), both active → sorted by time
	// app-b (30m ago) is newer than app-a latest (1h ago)
	if groups[0].Name != "app-b" {
		t.Errorf("first group = %q, want app-b (newest active)", groups[0].Name)
	}
	if groups[1].Name != "app-a" {
		t.Errorf("second group = %q, want app-a", groups[1].Name)
	}

	// app-a group: latest should be the newest (1h ago), older should have 2 entries
	appA := groups[1]
	if appA.Latest.Status != "running" {
		t.Errorf("app-a latest status = %q, want running", appA.Latest.Status)
	}
	if len(appA.Older) != 2 {
		t.Errorf("app-a older count = %d, want 2", len(appA.Older))
	}
}

func TestGroupDeployments_ActiveFirst(t *testing.T) {
	deploys := []DeploymentData{
		{Name: "stopped-app", Status: "stopped", CreatedAt: time.Now()},
		{Name: "running-app", Status: "running", CreatedAt: time.Now().Add(-time.Hour)},
	}

	groups := groupDeployments(deploys)
	if groups[0].Name != "running-app" {
		t.Errorf("first group = %q, want running-app (active first)", groups[0].Name)
	}
}

func TestRenderDeploymentsTable_Grouped(t *testing.T) {
	deploys := []DeploymentData{
		{Name: "my-app", Status: "running", Healthy: 4, Total: 4, Services: 2, Tags: []string{"prod"}, CreatedAt: time.Now().Add(-time.Hour)},
		{Name: "my-app", Status: "stopped", Healthy: 0, Total: 4, Services: 2, CreatedAt: time.Now().Add(-2 * time.Hour)},
		{Name: "my-app", Status: "stopped", Healthy: 0, Total: 4, Services: 2, CreatedAt: time.Now().Add(-3 * time.Hour)},
	}

	got := renderDeploymentsTable(deploys, 100)
	// Should show the latest deployment
	if !strings.Contains(got, "my-app") {
		t.Error("grouped table missing deployment name")
	}
	// Should show summary of older deployments
	if !strings.Contains(got, "previous") {
		t.Error("grouped table missing 'previous' summary for older deployments")
	}
	if !strings.Contains(got, "2 stopped") {
		t.Error("grouped table missing stopped count in summary")
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		name  string
		input string
		width int
		want  int // expected visual width of output
	}{
		{"short", "hi", 10, 10},
		{"exact", "hello", 5, 5},
		{"long", "toolong", 3, 7}, // no truncation, just returns as-is
		{"ansi", "\x1b[32mhi\x1b[0m", 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := padRight(tt.input, tt.width)
			gotWidth := lipgloss.Width(got)
			if gotWidth != tt.want {
				t.Errorf("padRight(%q, %d) visual width = %d, want %d", tt.input, tt.width, gotWidth, tt.want)
			}
		})
	}
}

func TestRenderOverview_NarrowWidth(t *testing.T) {
	data := &DashboardData{
		Engine: EngineData{
			Status:    "running",
			StartedAt: time.Now(),
		},
		Summary: ClusterSummary{},
	}

	// Narrow width should stack cards vertically
	got := renderOverview(data, 40)
	if !strings.Contains(got, "Engine") {
		t.Error("narrow overview missing Engine")
	}
}
