package cmd

import (
	"testing"
)

func TestRunLogs_NoConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	configPath = "/tmp/nonexistent-cli-config.yaml"

	err := runLogs(logsCmd, []string{"my-container"})
	if err == nil {
		t.Fatal("expected error when no config")
	}
}

func TestRunLogs_WithServer(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	err := runLogs(logsCmd, []string{"my-container"})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}
