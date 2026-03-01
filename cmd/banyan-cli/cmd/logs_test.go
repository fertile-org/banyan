package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/fertile-org/banyan/pkg/types"
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

func TestRunLogs_ClientConnectFails(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")
	cfg := types.BanyanConfig{
		CLI: types.CLIConfig{
			EngineHost:  "127.0.0.1",
			EnginePort:  "50051",
			WGPublicKey: "test-pubkey",
		},
	}
	if err := types.SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	configPath = cfgPath

	err := runLogs(logsCmd, []string{"my-container"})
	if err == nil {
		t.Fatal("expected error when client connect fails")
	}
	if !strings.Contains(err.Error(), "failed to connect to engine") {
		t.Errorf("unexpected error: %v", err)
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
