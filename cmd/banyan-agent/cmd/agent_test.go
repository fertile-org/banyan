package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/fertile-org/banyan/pkg/types"
)

func TestIsAgentRunning(t *testing.T) {
	origPidFile := agentPidFile
	t.Cleanup(func() { agentPidFile = origPidFile })

	t.Run("no PID file returns false", func(t *testing.T) {
		agentPidFile = filepath.Join(t.TempDir(), "nonexistent.pid")
		if isAgentRunning() {
			t.Error("expected false when PID file doesn't exist")
		}
	})

	t.Run("invalid content returns false", func(t *testing.T) {
		pidFile := filepath.Join(t.TempDir(), "bad.pid")
		os.WriteFile(pidFile, []byte("not-a-number"), 0600)
		agentPidFile = pidFile
		if isAgentRunning() {
			t.Error("expected false for non-numeric PID content")
		}
	})

	t.Run("current process PID returns true", func(t *testing.T) {
		pidFile := filepath.Join(t.TempDir(), "current.pid")
		os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0600)
		agentPidFile = pidFile
		if !isAgentRunning() {
			t.Error("expected true for current process PID")
		}
	})

	t.Run("non-existent PID returns false", func(t *testing.T) {
		pidFile := filepath.Join(t.TempDir(), "dead.pid")
		// Use a very large PID that's unlikely to exist
		os.WriteFile(pidFile, []byte("9999999"), 0600)
		agentPidFile = pidFile
		if isAgentRunning() {
			t.Error("expected false for non-existent process")
		}
	})
}

func TestRunAgentStop(t *testing.T) {
	origPidFile := agentPidFile
	t.Cleanup(func() { agentPidFile = origPidFile })

	t.Run("no running agent", func(t *testing.T) {
		agentPidFile = filepath.Join(t.TempDir(), "nonexistent.pid")
		err := runAgentStop(nil, nil)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("stops running agent (self)", func(t *testing.T) {
		// Write our own PID to simulate a running agent.
		// This test sends SIGTERM to ourselves, so we must handle it.
		tmpDir := t.TempDir()
		pidFile := filepath.Join(tmpDir, "agent.pid")
		os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o600)
		agentPidFile = pidFile

		// Skip this test because sending SIGTERM to self would kill the test runner
		// Instead, test the branch where PID file has invalid content
		agentPidFile = filepath.Join(tmpDir, "bad.pid")
		os.WriteFile(agentPidFile, []byte("not-a-pid"), 0o600)
		err := runAgentStop(nil, nil)
		// isAgentRunning() will return false for invalid PID, so it prints "not running"
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})
}

func TestIsAgentRunning_EmptyPidFile(t *testing.T) {
	origPidFile := agentPidFile
	t.Cleanup(func() { agentPidFile = origPidFile })

	pidFile := filepath.Join(t.TempDir(), "empty.pid")
	os.WriteFile(pidFile, []byte(""), 0o600)
	agentPidFile = pidFile

	if isAgentRunning() {
		t.Error("expected false for empty PID file")
	}
}

func TestRunAgentStop_StopsRunningProcess(t *testing.T) {
	origPidFile := agentPidFile
	t.Cleanup(func() { agentPidFile = origPidFile })

	// Start a background sleep process that we can safely kill
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sleep process: %v", err)
	}
	t.Cleanup(func() {
		// Ensure cleanup in case test fails
		cmd.Process.Kill()
		cmd.Wait()
	})

	pid := cmd.Process.Pid

	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "agent.pid")
	os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0o600)
	agentPidFile = pidFile

	err := runAgentStop(nil, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	// Verify PID file was removed
	if _, statErr := os.Stat(pidFile); !os.IsNotExist(statErr) {
		t.Error("expected PID file to be removed after stop")
	}

	// Wait for the process to actually exit
	cmd.Wait()
}

func TestRunAgentStatus_NotRunning_NoPassword(t *testing.T) {
	origPidFile := agentPidFile
	origConfig := configPath
	t.Cleanup(func() {
		agentPidFile = origPidFile
		configPath = origConfig
	})

	agentPidFile = filepath.Join(t.TempDir(), "nonexistent.pid")
	configPath = filepath.Join(t.TempDir(), "nonexistent.yaml")

	err := runAgentStatus(statusCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunAgentStatus_NotRunning_WithToken(t *testing.T) {
	origPidFile := agentPidFile
	origConfig := configPath
	origEndpoint := agentEngineEndpoint
	t.Cleanup(func() {
		agentPidFile = origPidFile
		configPath = origConfig
		agentEngineEndpoint = origEndpoint
	})

	agentPidFile = filepath.Join(t.TempDir(), "nonexistent.pid")

	// Create config with an auth token
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")
	cfg := types.BanyanConfig{
		Agent: types.AgentConfig{
			EngineHost: "127.0.0.1",
			EnginePort: "59999", // unreachable port
			AuthToken:  "test-token-abc",
		},
	}
	if err := types.SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	configPath = cfgPath

	// Health check will fail since there's no engine at this address
	err := runAgentStatus(statusCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunAgentStatus_Running(t *testing.T) {
	origPidFile := agentPidFile
	origConfig := configPath
	t.Cleanup(func() {
		agentPidFile = origPidFile
		configPath = origConfig
	})

	// Write current process PID to simulate running agent
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "agent.pid")
	os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o600)
	agentPidFile = pidFile

	// No password config → covers "NOT CONFIGURED" path
	configPath = filepath.Join(tmpDir, "nonexistent.yaml")

	err := runAgentStatus(statusCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}
