package dashboard

import (
	"strings"
	"testing"
	"time"
)

func TestFilteredData_EmptyFilter(t *testing.T) {
	data := &DashboardData{
		Events: []EventData{{Type: "test", Message: "hello"}},
	}
	got := filteredData(data, ViewEvents, "")
	if got != data {
		t.Error("empty filter should return same data pointer")
	}
}

func TestFilteredData_NilData(t *testing.T) {
	got := filteredData(nil, ViewEvents, "test")
	if got != nil {
		t.Error("nil data should return nil")
	}
}

func TestFilteredData_Events(t *testing.T) {
	data := &DashboardData{
		Events: []EventData{
			{Type: "deployment.created", Message: "Created my-app", Severity: "info"},
			{Type: "container.failed", Message: "Container crashed", Severity: "error"},
			{Type: "agent.registered", Message: "Agent connected", Severity: "info"},
		},
	}

	got := filteredData(data, ViewEvents, "error")
	if len(got.Events) != 1 {
		t.Fatalf("filter 'error': got %d events, want 1", len(got.Events))
	}
	if got.Events[0].Type != "container.failed" {
		t.Errorf("expected container.failed, got %s", got.Events[0].Type)
	}
}

func TestFilteredData_Events_MatchesMessage(t *testing.T) {
	data := &DashboardData{
		Events: []EventData{
			{Type: "deploy", Message: "Created my-app", Severity: "info"},
			{Type: "deploy", Message: "Created api-server", Severity: "info"},
		},
	}

	got := filteredData(data, ViewEvents, "api")
	if len(got.Events) != 1 {
		t.Fatalf("filter 'api': got %d events, want 1", len(got.Events))
	}
}

func TestFilteredData_Containers(t *testing.T) {
	data := &DashboardData{
		Containers: []ContainerData{
			{Name: "web-0", ServiceName: "web", AgentName: "worker-1", ContainerStatus: "running"},
			{Name: "api-0", ServiceName: "api", AgentName: "worker-2", ContainerStatus: "running"},
			{Name: "db-0", ServiceName: "db", AgentName: "worker-1", ContainerStatus: "stopped"},
		},
	}

	got := filteredData(data, ViewContainers, "worker-1")
	if len(got.Containers) != 2 {
		t.Fatalf("filter 'worker-1': got %d containers, want 2", len(got.Containers))
	}
}

func TestFilteredData_Agents(t *testing.T) {
	data := &DashboardData{
		Agents: []AgentData{
			{Name: "worker-1", Status: "ready", Tags: []string{"web"}},
			{Name: "worker-2", Status: "ready", Tags: []string{"api"}},
			{Name: "gateway", Status: "ready", Tags: []string{"proxy"}},
		},
	}

	got := filteredData(data, ViewAgents, "worker")
	if len(got.Agents) != 2 {
		t.Fatalf("filter 'worker': got %d agents, want 2", len(got.Agents))
	}
}

func TestFilteredData_Deployments(t *testing.T) {
	data := &DashboardData{
		Deployments: []DeploymentData{
			{Name: "my-app", Status: "running", CreatedAt: time.Now()},
			{Name: "api-server", Status: "stopped", CreatedAt: time.Now()},
		},
	}

	got := filteredData(data, ViewDeploys, "stopped")
	if len(got.Deployments) != 1 {
		t.Fatalf("filter 'stopped': got %d deployments, want 1", len(got.Deployments))
	}
	if got.Deployments[0].Name != "api-server" {
		t.Errorf("expected api-server, got %s", got.Deployments[0].Name)
	}
}

func TestFilteredData_CaseInsensitive(t *testing.T) {
	data := &DashboardData{
		Events: []EventData{
			{Type: "DEPLOYMENT.CREATED", Message: "test", Severity: "info"},
		},
	}

	got := filteredData(data, ViewEvents, "deployment")
	if len(got.Events) != 1 {
		t.Error("filter should be case-insensitive")
	}
}

func TestFilteredData_NoMatches(t *testing.T) {
	data := &DashboardData{
		Events: []EventData{
			{Type: "test", Message: "hello", Severity: "info"},
		},
	}

	got := filteredData(data, ViewEvents, "zzzzz")
	if len(got.Events) != 0 {
		t.Errorf("filter 'zzzzz': got %d events, want 0", len(got.Events))
	}
}

func TestMatchesFilter(t *testing.T) {
	tests := []struct {
		name   string
		lower  string
		fields []string
		want   bool
	}{
		{"match first", "hello", []string{"hello world", "foo"}, true},
		{"match second", "foo", []string{"hello", "foobar"}, true},
		{"no match", "zzz", []string{"hello", "world"}, false},
		{"empty fields", "test", []string{}, false},
		{"case insensitive", "test", []string{"Testing"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesFilter(tt.lower, tt.fields...)
			if got != tt.want {
				t.Errorf("matchesFilter(%q, %v) = %v, want %v", tt.lower, tt.fields, got, tt.want)
			}
		})
	}
}

func TestRenderFilterBar_Editing(t *testing.T) {
	got := renderFilterBar("error", true, 80)
	if !strings.Contains(got, "/") {
		t.Error("editing filter bar missing / prompt")
	}
	if !strings.Contains(got, "error") {
		t.Error("editing filter bar missing filter text")
	}
	if !strings.Contains(got, "Esc") {
		t.Error("editing filter bar missing Esc hint")
	}
}

func TestRenderFilterBar_Applied(t *testing.T) {
	got := renderFilterBar("error", false, 80)
	if !strings.Contains(got, "Filter:") {
		t.Error("applied filter bar missing Filter: label")
	}
	if !strings.Contains(got, "error") {
		t.Error("applied filter bar missing filter text")
	}
	if !strings.Contains(got, "export") {
		t.Error("applied filter bar missing export hint")
	}
}
