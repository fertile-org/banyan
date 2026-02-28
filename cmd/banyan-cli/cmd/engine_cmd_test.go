package cmd

import (
	"testing"
)

func TestRunEngine_NoConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	configPath = "/tmp/nonexistent-cli-config.yaml"

	err := runEngine(engineCmd, nil)
	if err == nil {
		t.Fatal("expected error when no config")
	}
}

func TestRunEngine_WithServer(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := runEngine(engineCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunEngine_JSON(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = "json"

	err := runEngine(engineCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}
