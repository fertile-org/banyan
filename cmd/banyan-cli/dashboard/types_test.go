package dashboard

import (
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
)

func TestConvertFromProto(t *testing.T) {
	now := time.Now().Unix()
	resp := &banyanpb.GetDashboardDataResponse{
		Engine: &banyanpb.EngineStatus{
			Status:        "running",
			StartedAtUnix: now - 3600,
			SystemMetrics: &banyanpb.SystemMetrics{
				CpuUsageRatio:    0.45,
				MemoryUsedBytes:  1024 * 1024 * 512,
				MemoryTotalBytes: 1024 * 1024 * 1024,
				DiskUsedBytes:    1024 * 1024 * 1024 * 10,
				DiskTotalBytes:   1024 * 1024 * 1024 * 50,
				CpuCores:         4,
			},
		},
		Agents: []*banyanpb.AgentDetail{
			{
				Name:           "agent-1",
				Status:         "ready",
				Tags:           []string{"us-east", "gpu"},
				LastSeenUnix:   now - 10,
				CreatedAtUnix:  now - 7200,
				VpcSubnet:      "10.0.1.0/24",
				ApiAddress:     "192.168.1.10:50052",
				ContainerCount: 3,
				SystemMetrics: &banyanpb.SystemMetrics{
					CpuUsageRatio:    0.30,
					MemoryUsedBytes:  1024 * 1024 * 256,
					MemoryTotalBytes: 1024 * 1024 * 2048,
					CpuCores:         8,
				},
			},
		},
		Deployments: []*banyanpb.DeploymentInfo{
			{
				Id:            "dep-001",
				Name:          "my-app",
				Status:        "running",
				Healthy:       2,
				Total:         2,
				Services:      map[string]*banyanpb.ServiceInfo{"web": {}, "db": {}},
				Tags:          []string{"production"},
				CreatedAtUnix: now - 600,
				UpdatedAtUnix: now - 60,
			},
		},
		Summary: &banyanpb.ClusterSummary{
			TotalAgents:        2,
			ConnectedAgents:    1,
			TotalDeployments:   1,
			RunningDeployments: 1,
			TotalContainers:    3,
			HealthyContainers:  2,
			TasksByStatus:      map[string]int32{"completed": 5, "failed": 1},
		},
		RecentEvents: []*banyanpb.ClusterEvent{
			{
				TimestampUnix: now - 30,
				Type:          "agent.registered",
				Message:       "Agent agent-1 registered",
				Severity:      "info",
			},
		},
	}

	data := ConvertFromProto(resp)

	// Engine
	if data.Engine.Status != "running" {
		t.Errorf("engine status = %q, want running", data.Engine.Status)
	}
	if data.Engine.CPU != 0.45 {
		t.Errorf("engine CPU = %f, want 0.45", data.Engine.CPU)
	}
	if data.Engine.MemUsed != 1024*1024*512 {
		t.Errorf("engine MemUsed = %d, want %d", data.Engine.MemUsed, 1024*1024*512)
	}
	if data.Engine.CPUCores != 4 {
		t.Errorf("engine CPUCores = %d, want 4", data.Engine.CPUCores)
	}

	// Agents
	if len(data.Agents) != 1 {
		t.Fatalf("agents count = %d, want 1", len(data.Agents))
	}
	a := data.Agents[0]
	if a.Name != "agent-1" {
		t.Errorf("agent name = %q, want agent-1", a.Name)
	}
	if a.ContainerCount != 3 {
		t.Errorf("agent container count = %d, want 3", a.ContainerCount)
	}
	if len(a.Tags) != 2 {
		t.Errorf("agent tags count = %d, want 2", len(a.Tags))
	}
	if a.CPU != 0.30 {
		t.Errorf("agent CPU = %f, want 0.30", a.CPU)
	}

	// Deployments
	if len(data.Deployments) != 1 {
		t.Fatalf("deployments count = %d, want 1", len(data.Deployments))
	}
	d := data.Deployments[0]
	if d.Name != "my-app" {
		t.Errorf("deployment name = %q, want my-app", d.Name)
	}
	if d.Healthy != 2 || d.Total != 2 {
		t.Errorf("deployment health = %d/%d, want 2/2", d.Healthy, d.Total)
	}
	if d.Services != 2 {
		t.Errorf("deployment services = %d, want 2", d.Services)
	}

	// Summary
	if data.Summary.TotalAgents != 2 {
		t.Errorf("total agents = %d, want 2", data.Summary.TotalAgents)
	}
	if data.Summary.ConnectedAgents != 1 {
		t.Errorf("connected agents = %d, want 1", data.Summary.ConnectedAgents)
	}
	if data.Summary.CompletedTasks != 5 {
		t.Errorf("completed tasks = %d, want 5", data.Summary.CompletedTasks)
	}
	if data.Summary.FailedTasks != 1 {
		t.Errorf("failed tasks = %d, want 1", data.Summary.FailedTasks)
	}

	// Events
	if len(data.Events) != 1 {
		t.Fatalf("events count = %d, want 1", len(data.Events))
	}
	if data.Events[0].Type != "agent.registered" {
		t.Errorf("event type = %q, want agent.registered", data.Events[0].Type)
	}
}

func TestConvertFromProto_NilFields(t *testing.T) {
	resp := &banyanpb.GetDashboardDataResponse{}
	data := ConvertFromProto(resp)

	if data.Engine.Status != "" {
		t.Errorf("engine status = %q, want empty", data.Engine.Status)
	}
	if len(data.Agents) != 0 {
		t.Errorf("agents count = %d, want 0", len(data.Agents))
	}
	if len(data.Deployments) != 0 {
		t.Errorf("deployments count = %d, want 0", len(data.Deployments))
	}
	if data.Summary.TotalAgents != 0 {
		t.Errorf("total agents = %d, want 0", data.Summary.TotalAgents)
	}
}

func TestConvertFromProto_AgentWithoutMetrics(t *testing.T) {
	resp := &banyanpb.GetDashboardDataResponse{
		Agents: []*banyanpb.AgentDetail{
			{
				Name:   "agent-no-metrics",
				Status: "ready",
			},
		},
	}
	data := ConvertFromProto(resp)

	if len(data.Agents) != 1 {
		t.Fatalf("agents count = %d, want 1", len(data.Agents))
	}
	if data.Agents[0].CPU != 0 {
		t.Errorf("agent CPU = %f, want 0", data.Agents[0].CPU)
	}
}
