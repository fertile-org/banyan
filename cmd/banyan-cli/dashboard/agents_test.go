package dashboard

import (
	"strings"
	"testing"
	"time"
)

func TestRenderAgentList_NilData(t *testing.T) {
	got := renderAgentList(nil, 120, 0)
	if !strings.Contains(got, "Loading") {
		t.Error("nil data should show loading")
	}
}

func TestRenderAgentList_Empty(t *testing.T) {
	data := &DashboardData{}
	got := renderAgentList(data, 120, 0)
	if !strings.Contains(got, "No agents") {
		t.Error("empty agents should show no agents message")
	}
}

func TestRenderAgentList_WithAgents(t *testing.T) {
	data := &DashboardData{
		Agents: []AgentData{
			{Name: "worker-1", Status: "ready", VPCSubnet: "10.0.1.0/24", Tags: []string{"gpu"}, CPU: 0.5, ContainerCount: 3},
			{Name: "worker-2", Status: "stale", Tags: []string{"cpu"}, CPU: 0.1, ContainerCount: 0},
		},
	}

	got := renderAgentList(data, 120, 0)
	if !strings.Contains(got, "worker-1") {
		t.Error("agent list missing worker-1")
	}
	if !strings.Contains(got, "worker-2") {
		t.Error("agent list missing worker-2")
	}
	if !strings.Contains(got, "Agents (2)") {
		t.Error("agent list missing count")
	}
	if !strings.Contains(got, ">") {
		t.Error("agent list missing cursor marker")
	}
}

func TestRenderAgentList_CursorPosition(t *testing.T) {
	data := &DashboardData{
		Agents: []AgentData{
			{Name: "agent-a", Status: "ready"},
			{Name: "agent-b", Status: "ready"},
		},
	}

	// Cursor on second item
	got := renderAgentList(data, 120, 1)
	lines := strings.Split(got, "\n")

	// Find the line with agent-b - it should have the cursor
	foundCursorOnB := false
	for _, l := range lines {
		if strings.Contains(l, "agent-b") && strings.Contains(l, ">") {
			foundCursorOnB = true
		}
	}
	if !foundCursorOnB {
		t.Error("cursor should be on agent-b when listCursor=1")
	}
}

func TestRenderAgentDetail_NilData(t *testing.T) {
	got := renderAgentDetail(nil, "worker-1", 100)
	if !strings.Contains(got, "Loading") {
		t.Error("nil data should show loading")
	}
}

func TestRenderAgentDetail_NotFound(t *testing.T) {
	data := &DashboardData{}
	got := renderAgentDetail(data, "nonexistent", 100)
	if !strings.Contains(got, "not found") {
		t.Error("missing agent should show not found")
	}
}

func TestRenderAgentDetail_WithData(t *testing.T) {
	data := &DashboardData{
		Agents: []AgentData{
			{
				Name:           "worker-1",
				Status:         "ready",
				APIAddress:     "192.168.1.10:50052",
				VPCSubnet:      "10.0.1.0/24",
				Tags:           []string{"us-east", "gpu"},
				CPU:            0.45,
				MemUsed:        512 * 1024 * 1024,
				MemTotal:       2048 * 1024 * 1024,
				DiskUsed:       10 * 1024 * 1024 * 1024,
				DiskTotal:      50 * 1024 * 1024 * 1024,
				CPUCores:       8,
				ContainerCount: 2,
				CreatedAt:      time.Now().Add(-2 * time.Hour),
			},
		},
		Containers: []ContainerData{
			{Name: "app-web-0", ServiceName: "web", AgentName: "worker-1", ContainerStatus: "running", Image: "nginx", Ports: []string{"8080:80"}},
			{Name: "app-db-0", ServiceName: "db", AgentName: "worker-1", ContainerStatus: "running", Image: "postgres", Ports: []string{"5432:5432"}},
			{Name: "other-0", ServiceName: "web", AgentName: "worker-2", ContainerStatus: "running"},
		},
	}

	got := renderAgentDetail(data, "worker-1", 120)

	// Info card
	if !strings.Contains(got, "worker-1") {
		t.Error("agent detail missing name")
	}
	if !strings.Contains(got, "192.168.1.10:50052") {
		t.Error("agent detail missing address")
	}
	if !strings.Contains(got, "10.0.1.0/24") {
		t.Error("agent detail missing VPC subnet")
	}
	if !strings.Contains(got, "us-east") {
		t.Error("agent detail missing tags")
	}
	if !strings.Contains(got, "CPU") {
		t.Error("agent detail missing CPU bar")
	}

	// Container table - should only show worker-1's containers
	if !strings.Contains(got, "app-web-0") {
		t.Error("agent detail missing container app-web-0")
	}
	if !strings.Contains(got, "app-db-0") {
		t.Error("agent detail missing container app-db-0")
	}
	if strings.Contains(got, "other-0") {
		t.Error("agent detail should not show worker-2's containers")
	}
	if !strings.Contains(got, "Containers (2)") {
		t.Error("agent detail missing container count")
	}
}

func TestRenderContainerTable_Empty(t *testing.T) {
	got := renderContainerTable(nil, 100, "Containers (0)", true)
	if !strings.Contains(got, "No containers") {
		t.Error("empty container table should show no containers")
	}
}

func TestRenderContainerTable_WithData(t *testing.T) {
	containers := []ContainerData{
		{Name: "web-0", ServiceName: "web", AgentName: "agent-1", ContainerStatus: "running", Image: "nginx", Ports: []string{"8080:80"}},
	}

	got := renderContainerTable(containers, 120, "Test Containers", true)
	if !strings.Contains(got, "web-0") {
		t.Error("container table missing container name")
	}
	if !strings.Contains(got, "nginx") {
		t.Error("container table missing image")
	}
	if !strings.Contains(got, "Agent") {
		t.Error("container table with showAgent=true should have Agent column")
	}

	// Without agent column
	got = renderContainerTable(containers, 120, "Test", false)
	// Header should not have Agent column
	lines := strings.Split(got, "\n")
	if len(lines) > 1 && strings.Contains(lines[1], "Agent") {
		t.Error("container table with showAgent=false should not have Agent header column")
	}
}
