package dashboard

import (
	"strings"
	"testing"
	"time"
)

func TestRenderDeploymentList_NilData(t *testing.T) {
	got := renderDeploymentList(nil, 120, 0)
	if !strings.Contains(got, "Loading") {
		t.Error("nil data should show loading")
	}
}

func TestRenderDeploymentList_Empty(t *testing.T) {
	data := &DashboardData{}
	got := renderDeploymentList(data, 120, 0)
	if !strings.Contains(got, "No deployments") {
		t.Error("empty deployments should show no deployments message")
	}
}

func TestRenderDeploymentList_WithData(t *testing.T) {
	data := &DashboardData{
		Deployments: []DeploymentData{
			{Name: "web-app", Status: "running", Healthy: 2, Total: 2, Services: 2, Tags: []string{"prod"}, CreatedAt: time.Now().Add(-time.Hour)},
			{Name: "api", Status: "stopped", Healthy: 0, Total: 1, Services: 1, CreatedAt: time.Now().Add(-2 * time.Hour)},
		},
	}

	got := renderDeploymentList(data, 120, 0)
	if !strings.Contains(got, "web-app") {
		t.Error("deployment list missing web-app")
	}
	if !strings.Contains(got, "api") {
		t.Error("deployment list missing api")
	}
	if !strings.Contains(got, ">") {
		t.Error("deployment list missing cursor")
	}
}

func TestRenderDeploymentList_CursorPosition(t *testing.T) {
	data := &DashboardData{
		Deployments: []DeploymentData{
			{Name: "app-a", Status: "running", CreatedAt: time.Now()},
			{Name: "app-b", Status: "stopped", CreatedAt: time.Now().Add(-time.Hour)},
		},
	}

	got := renderDeploymentList(data, 120, 1)
	lines := strings.Split(got, "\n")

	foundCursorOnB := false
	for _, l := range lines {
		if strings.Contains(l, "app-b") && strings.Contains(l, ">") {
			foundCursorOnB = true
		}
	}
	if !foundCursorOnB {
		t.Error("cursor should be on app-b when listCursor=1")
	}
}

func TestRenderDeploymentDetail_NilData(t *testing.T) {
	got := renderDeploymentDetail(nil, "dep-001", 100)
	if !strings.Contains(got, "Loading") {
		t.Error("nil data should show loading")
	}
}

func TestRenderDeploymentDetail_NotFound(t *testing.T) {
	data := &DashboardData{}
	got := renderDeploymentDetail(data, "nonexistent", 100)
	if !strings.Contains(got, "not found") {
		t.Error("missing deployment should show not found")
	}
}

func TestRenderDeploymentDetail_WithData(t *testing.T) {
	data := &DashboardData{
		Deployments: []DeploymentData{
			{
				ID:      "dep-001",
				Name:    "my-app",
				Status:  "running",
				Healthy: 3,
				Total:   3,
				Tags:    []string{"production"},
				Error:   "",
				ServiceDetails: []ServiceData{
					{Name: "web", Image: "nginx:latest", Replicas: 2, Ports: []string{"8080:80"}},
					{Name: "db", Image: "postgres:15", Replicas: 1, Ports: []string{"5432:5432"}, DependsOn: []string{"web"}},
				},
				Containers: []ContainerData{
					{Name: "app-web-0", ServiceName: "web", AgentName: "worker-1", ContainerStatus: "running", Image: "nginx:latest"},
					{Name: "app-web-1", ServiceName: "web", AgentName: "worker-2", ContainerStatus: "running", Image: "nginx:latest"},
					{Name: "app-db-0", ServiceName: "db", AgentName: "worker-1", ContainerStatus: "running", Image: "postgres:15"},
				},
				CreatedAt: time.Now().Add(-time.Hour),
				UpdatedAt: time.Now().Add(-5 * time.Minute),
			},
		},
	}

	got := renderDeploymentDetail(data, "dep-001", 120)

	// Info card
	if !strings.Contains(got, "my-app") {
		t.Error("deployment detail missing name")
	}
	if !strings.Contains(got, "dep-001") {
		t.Error("deployment detail missing ID")
	}
	if !strings.Contains(got, "production") {
		t.Error("deployment detail missing tags")
	}

	// Services card
	if !strings.Contains(got, "web") {
		t.Error("deployment detail missing web service")
	}
	if !strings.Contains(got, "db") {
		t.Error("deployment detail missing db service")
	}
	if !strings.Contains(got, "nginx") {
		t.Error("deployment detail missing nginx image")
	}
	if !strings.Contains(got, "postgres") {
		t.Error("deployment detail missing postgres image")
	}

	// Containers card
	if !strings.Contains(got, "app-web-0") {
		t.Error("deployment detail missing container app-web-0")
	}
	if !strings.Contains(got, "Containers (3)") {
		t.Error("deployment detail missing container count")
	}
}

func TestRenderDeploymentDetail_WithError(t *testing.T) {
	data := &DashboardData{
		Deployments: []DeploymentData{
			{
				ID:        "dep-fail",
				Name:      "broken-app",
				Status:    "failed",
				Error:     "container crashed on startup",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}

	got := renderDeploymentDetail(data, "dep-fail", 100)
	if !strings.Contains(got, "container crashed") {
		t.Error("deployment detail should show error message")
	}
}

func TestRenderDeploymentDetail_NoServiceDetails(t *testing.T) {
	data := &DashboardData{
		Deployments: []DeploymentData{
			{
				ID:        "dep-minimal",
				Name:      "minimal-app",
				Status:    "running",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}

	got := renderDeploymentDetail(data, "dep-minimal", 100)
	if !strings.Contains(got, "No service details") {
		t.Error("deployment with no service details should show message")
	}
}
