package dashboard

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRenderSnapshot_Basic(t *testing.T) {
	data := &DashboardData{
		FetchedAt: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC),
		Engine:    EngineData{Status: "running", StartedAt: time.Now().Add(-2 * time.Hour)},
		Summary: ClusterSummary{
			TotalAgents:        2,
			ConnectedAgents:    2,
			TotalDeployments:   1,
			RunningDeployments: 1,
			TotalContainers:    3,
			HealthyContainers:  3,
			CompletedTasks:     3,
			FailedTasks:        0,
		},
		Agents: []AgentData{
			{Name: "worker-1", Status: "ready", CPU: 0.45, MemUsed: 2 << 30, MemTotal: 8 << 30, ContainerCount: 2, VPCSubnet: "10.0.1.0/24"},
		},
		Deployments: []DeploymentData{
			{Name: "my-app", Status: "running", Healthy: 2, Total: 2, Services: 2, CreatedAt: time.Now().Add(-time.Hour)},
		},
		Containers: []ContainerData{
			{Name: "my-app-web-0", ServiceName: "web", AgentName: "worker-1", ContainerStatus: "running", CPUPercent: 23.5, MemUsed: 128 << 20, Ports: []string{"8080:80"}},
			{Name: "my-app-db-0", ServiceName: "db", AgentName: "worker-1", ContainerStatus: "running", CPUPercent: 12.1, MemUsed: 256 << 20, Ports: []string{"5432:5432"}},
		},
		Events: []EventData{
			{Timestamp: time.Now(), Type: "deployment.running", Message: "my-app is running", Severity: "info"},
		},
	}

	var buf bytes.Buffer
	err := RenderSnapshot(data, &buf)
	if err != nil {
		t.Fatalf("RenderSnapshot failed: %v", err)
	}

	html := buf.String()

	// Check basic HTML structure
	checks := []string{
		"<!DOCTYPE html>",
		"Banyan Cluster Snapshot",
		"2026-03-30",
		"worker-1",
		"my-app",
		"my-app-web-0",
		"8080:80",
		"23.5%",
		"var(--bg)",
		"var(--primary)",
	}
	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Errorf("snapshot HTML missing %q", check)
		}
	}
}

func TestRenderSnapshot_EmptyData(t *testing.T) {
	data := &DashboardData{
		FetchedAt: time.Now(),
		Engine:    EngineData{Status: "running"},
	}

	var buf bytes.Buffer
	err := RenderSnapshot(data, &buf)
	if err != nil {
		t.Fatalf("RenderSnapshot failed: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("empty data should still produce valid HTML")
	}
	// No agents/deployments/containers sections
	if strings.Contains(html, "worker-") {
		t.Error("empty data should not contain agent names")
	}
}

func TestRenderSnapshot_NilData(t *testing.T) {
	var buf bytes.Buffer
	err := RenderSnapshot(nil, &buf)
	// nil data should not panic, template will fail gracefully
	if err == nil {
		t.Error("nil data should produce an error")
	}
}
