package dashboard

import (
	"strings"
	"testing"
)

func TestRenderContainerList_NilData(t *testing.T) {
	got := renderContainerList(nil, 120, 0, nil)
	if !strings.Contains(got, "Loading") {
		t.Error("nil data should show loading")
	}
}

func TestRenderContainerList_Empty(t *testing.T) {
	data := &DashboardData{}
	got := renderContainerList(data, 120, 0, nil)
	if !strings.Contains(got, "No containers") {
		t.Error("empty should show no containers")
	}
}

func TestRenderContainerList_WithData(t *testing.T) {
	data := &DashboardData{
		Containers: []ContainerData{
			{Name: "app-web-0", ServiceName: "web", AgentName: "worker-1", DeploymentName: "my-app", ContainerStatus: "running", Ports: []string{"8080:80"}},
			{Name: "app-db-0", ServiceName: "db", AgentName: "worker-1", DeploymentName: "my-app", ContainerStatus: "running", Ports: []string{"5432:5432"}},
			{Name: "api-web-0", ServiceName: "web", AgentName: "worker-2", DeploymentName: "api", ContainerStatus: "stopped"},
		},
	}

	got := renderContainerList(data, 140, 0, nil)

	if !strings.Contains(got, "All Containers (3)") {
		t.Error("container list missing count")
	}
	if !strings.Contains(got, "app-web-0") {
		t.Error("container list missing app-web-0")
	}
	if !strings.Contains(got, "app-db-0") {
		t.Error("container list missing app-db-0")
	}
	if !strings.Contains(got, "api-web-0") {
		t.Error("container list missing api-web-0")
	}
	if !strings.Contains(got, "my-app") {
		t.Error("container list missing deployment name")
	}
	if !strings.Contains(got, "worker-1") {
		t.Error("container list missing agent name")
	}
}

func TestRenderContainerList_CursorPosition(t *testing.T) {
	data := &DashboardData{
		Containers: []ContainerData{
			{Name: "container-a", ContainerStatus: "running"},
			{Name: "container-b", ContainerStatus: "running"},
		},
	}

	got := renderContainerList(data, 120, 1, nil)
	lines := strings.Split(got, "\n")

	foundCursorOnB := false
	for _, l := range lines {
		if strings.Contains(l, "container-b") && strings.Contains(l, ">") {
			foundCursorOnB = true
		}
	}
	if !foundCursorOnB {
		t.Error("cursor should be on container-b when cursor=1")
	}
}

func TestRenderContainerList_FallbackStatus(t *testing.T) {
	data := &DashboardData{
		Containers: []ContainerData{
			{Name: "pending-task", Status: "pending", ContainerStatus: ""},
		},
	}

	got := renderContainerList(data, 120, 0, nil)
	if !strings.Contains(got, "pending") {
		t.Error("should fall back to task status when container status is empty")
	}
}
