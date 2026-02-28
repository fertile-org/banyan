package cmd

import (
	"testing"
	"time"

	"github.com/fertile-org/banyan/cmd/banyan-cli/dashboard"
)

func TestRunDeployment_NoConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	configPath = "/tmp/nonexistent-cli-config.yaml"

	err := runDeployment(deploymentCmd, nil)
	if err == nil {
		t.Fatal("expected error when no config")
	}
}

func TestRunDeployment_ListWithServer(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := runDeployment(deploymentCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunDeployment_DetailByName(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := runDeployment(deploymentCmd, []string{"my-app"})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunDeployment_DetailByID(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := runDeployment(deploymentCmd, []string{"dep-001"})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunDeployment_DetailNotFound(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := runDeployment(deploymentCmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent deployment")
	}
}

func TestRunDeployment_JSON(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = "json"

	err := runDeployment(deploymentCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestListDeployments_Empty(t *testing.T) {
	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := listDeployments(nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestShowDeploymentDetail(t *testing.T) {
	deps := []dashboard.DeploymentData{
		{
			ID: "dep-001", Name: "my-app", Status: "running",
			Healthy: 2, Total: 3, Services: 2, Tags: []string{"env:prod"},
			CreatedAt: time.Now().Add(-30 * time.Minute),
			UpdatedAt: time.Now().Add(-1 * time.Minute),
			Error:     "",
			ServiceDetails: []dashboard.ServiceData{
				{Name: "web", Image: "nginx:alpine", Replicas: 2, Ports: []string{"80:80"}},
				{Name: "api", Image: "myapp/api:v1", Replicas: 1, Ports: []string{"8080:8080"}, DependsOn: []string{"db"}},
			},
			Containers: []dashboard.ContainerData{
				{Name: "my-app-web-0", ServiceName: "web", AgentName: "worker-1", ContainerStatus: "running", Image: "nginx:alpine"},
				{Name: "my-app-web-1", ServiceName: "web", AgentName: "worker-2", ContainerStatus: "running", Image: "nginx:alpine"},
				{Name: "my-app-api-0", ServiceName: "api", AgentName: "worker-1", ContainerStatus: "stopped", Image: "myapp/api:v1"},
			},
		},
	}

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := showDeploymentDetail(deps, "my-app")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestShowDeploymentDetail_WithError(t *testing.T) {
	deps := []dashboard.DeploymentData{
		{
			ID: "dep-002", Name: "broken-app", Status: "failed",
			Healthy: 0, Total: 1,
			CreatedAt: time.Now().Add(-10 * time.Minute),
			UpdatedAt: time.Now().Add(-5 * time.Minute),
			Error:     "image pull failed: unauthorized",
		},
	}

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := showDeploymentDetail(deps, "broken-app")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}
