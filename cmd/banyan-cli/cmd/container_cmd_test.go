package cmd

import (
	"testing"
	"time"

	"github.com/fertile-org/banyan/cmd/banyan-cli/dashboard"
)

func TestRunContainer_NoConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	configPath = "/tmp/nonexistent-cli-config.yaml"

	err := runContainer(containerCmd, nil)
	if err == nil {
		t.Fatal("expected error when no config")
	}
}

func TestRunContainer_ListWithServer(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := runContainer(containerCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunContainer_DetailWithServer(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := runContainer(containerCmd, []string{"my-app-web-0"})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunContainer_DetailNotFound(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := runContainer(containerCmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent container")
	}
}

func TestRunContainer_JSON(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = "json"

	err := runContainer(containerCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestListContainers_Empty(t *testing.T) {
	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := listContainers(nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestShowContainerDetail(t *testing.T) {
	containers := []dashboard.ContainerData{
		{
			Name: "my-app-web-0", ServiceName: "web", AgentName: "worker-1",
			DeploymentName: "my-app", DeploymentID: "dep-001",
			Status: "completed", ContainerStatus: "running",
			Image: "nginx:alpine", Ports: []string{"80:80"},
			ReplicaIndex: 0,
			CreatedAt:    time.Now().Add(-30 * time.Minute),
			UpdatedAt:    time.Now().Add(-28 * time.Minute),
		},
	}

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := showContainerDetail(containers, "my-app-web-0")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestShowContainerDetail_EmptyContainerStatus(t *testing.T) {
	containers := []dashboard.ContainerData{
		{
			Name: "my-app-web-0", ServiceName: "web", AgentName: "worker-1",
			Status: "pending", ContainerStatus: "",
		},
	}

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := showContainerDetail(containers, "my-app-web-0")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}
