package dashboard

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWriteCSV_Events(t *testing.T) {
	var buf bytes.Buffer
	data := &DashboardData{
		Events: []EventData{
			{Timestamp: time.Date(2026, 2, 27, 10, 30, 0, 0, time.UTC), Type: "deploy", Message: "Created app", Severity: "info"},
			{Timestamp: time.Date(2026, 2, 27, 10, 31, 0, 0, time.UTC), Type: "error", Message: "Container crashed", Severity: "error"},
		},
	}

	if err := writeCSV(&buf, data, ViewEvents); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3 (header + 2 rows)", len(lines))
	}
	if !strings.Contains(lines[0], "Time") || !strings.Contains(lines[0], "Message") {
		t.Error("missing CSV header fields")
	}
	if !strings.Contains(lines[1], "Created app") {
		t.Error("missing first event data")
	}
}

func TestWriteCSV_Containers(t *testing.T) {
	var buf bytes.Buffer
	data := &DashboardData{
		Containers: []ContainerData{
			{Name: "web-0", ServiceName: "web", AgentName: "worker-1", DeploymentName: "app", ContainerStatus: "running", Ports: []string{"80:80", "443:443"}},
		},
	}

	if err := writeCSV(&buf, data, ViewContainers); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if !strings.Contains(lines[1], "web-0") {
		t.Error("missing container name")
	}
	if !strings.Contains(lines[1], "80:80;443:443") {
		t.Error("ports should be semicolon-separated")
	}
}

func TestWriteCSV_Agents(t *testing.T) {
	var buf bytes.Buffer
	data := &DashboardData{
		Agents: []AgentData{
			{Name: "worker-1", Status: "ready", Tags: []string{"web", "api"}, CPU: 0.25, MemUsed: 512 * 1024 * 1024, MemTotal: 1024 * 1024 * 1024, ContainerCount: 3},
		},
	}

	if err := writeCSV(&buf, data, ViewAgents); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if !strings.Contains(lines[1], "worker-1") {
		t.Error("missing agent name")
	}
	if !strings.Contains(lines[1], "25.0") {
		t.Error("CPU should be 25.0%%")
	}
}

func TestWriteCSV_Deployments(t *testing.T) {
	var buf bytes.Buffer
	data := &DashboardData{
		Deployments: []DeploymentData{
			{Name: "my-app", ID: "dep-001", Status: "running", Healthy: 3, Total: 3, Services: 2, Tags: []string{"prod"}, CreatedAt: time.Date(2026, 2, 27, 10, 0, 0, 0, time.UTC)},
		},
	}

	if err := writeCSV(&buf, data, ViewDeploys); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if !strings.Contains(lines[1], "my-app") {
		t.Error("missing deployment name")
	}
	if !strings.Contains(lines[1], "dep-001") {
		t.Error("missing deployment ID")
	}
}

func TestWriteCSV_EmptyData(t *testing.T) {
	var buf bytes.Buffer
	if err := writeCSV(&buf, &DashboardData{}, ViewEvents); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1 (header only)", len(lines))
	}
}

func TestWriteCSV_UnsupportedView(t *testing.T) {
	var buf bytes.Buffer
	err := writeCSV(&buf, &DashboardData{}, ViewOverview)
	if err == nil {
		t.Error("expected error for unsupported view")
	}
}

func TestViewExportName(t *testing.T) {
	tests := []struct {
		view View
		want string
	}{
		{ViewAgents, "agents"},
		{ViewDeploys, "deployments"},
		{ViewContainers, "containers"},
		{ViewEvents, "events"},
		{ViewOverview, "data"},
	}

	for _, tt := range tests {
		got := viewExportName(tt.view)
		if got != tt.want {
			t.Errorf("viewExportName(%d) = %q, want %q", tt.view, got, tt.want)
		}
	}
}

func TestWriteCSV_ContainerFallbackStatus(t *testing.T) {
	var buf bytes.Buffer
	data := &DashboardData{
		Containers: []ContainerData{
			{Name: "pending-task", Status: "pending", ContainerStatus: ""},
		},
	}

	if err := writeCSV(&buf, data, ViewContainers); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "pending") {
		t.Error("should use task status as fallback")
	}
}
