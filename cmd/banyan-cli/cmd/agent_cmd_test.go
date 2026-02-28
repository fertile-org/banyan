package cmd

import (
	"testing"
	"time"

	"github.com/fertile-org/banyan/cmd/banyan-cli/dashboard"
)

func TestRunAgent_NoConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	configPath = "/tmp/nonexistent-cli-config.yaml"

	err := runAgent(agentCmd, nil)
	if err == nil {
		t.Fatal("expected error when no config")
	}
}

func TestRunAgent_ListWithServer(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := runAgent(agentCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunAgent_DetailWithServer(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := runAgent(agentCmd, []string{"worker-1"})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunAgent_DetailNotFound(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := runAgent(agentCmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestRunAgent_JSON(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = "json"

	err := runAgent(agentCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunAgent_DetailJSON(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = "json"

	err := runAgent(agentCmd, []string{"worker-1"})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestListAgents_Empty(t *testing.T) {
	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := listAgents(nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestShowAgentDetail(t *testing.T) {
	agents := []dashboard.AgentData{
		{
			Name: "worker-1", Status: "connected", APIAddress: "10.0.1.10:50052",
			VPCSubnet: "10.0.1.0/24", Tags: []string{"zone:us-east"},
			ContainerCount: 3, CPU: 0.45, CPUCores: 8,
			MemUsed: 2147483648, MemTotal: 8589934592,
			DiskUsed: 21474836480, DiskTotal: 107374182400,
			LastSeen: time.Now().Add(-5 * time.Second), CreatedAt: time.Now().Add(-2 * time.Hour),
		},
	}

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := showAgentDetail(agents, "worker-1")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}
