package cmd

import (
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fertile-org/banyan/pkg/types"
)

// mockTunnelOps replaces tunnel function variables with no-ops for testing.
// Returns a cleanup function to restore originals.
func mockTunnelOps(t *testing.T) {
	t.Helper()
	origSetup := setupControlTunnelFn
	origAdd := addControlPeerFn
	origCleanup := cleanupControlTunnelFn
	setupControlTunnelFn = func(string, string, net.IP, int) error { return nil }
	addControlPeerFn = func(string, string, string, net.IP) error { return nil }
	cleanupControlTunnelFn = func(string) error { return nil }
	t.Cleanup(func() {
		setupControlTunnelFn = origSetup
		addControlPeerFn = origAdd
		cleanupControlTunnelFn = origCleanup
	})
}

func TestCLIInit_NonInteractiveFlags(t *testing.T) {
	initCmd.ResetFlags()
	initCmd.Flags().Bool("non-interactive", false, "")
	initCmd.Flags().String("engine-host", "localhost", "")
	initCmd.Flags().String("engine-port", "50051", "")
	initCmd.Flags().String("cli-name", "", "")
	initCmd.Flags().String("engine-wg-pubkey", "", "")

	if initCmd.Flags().Lookup("non-interactive") == nil {
		t.Error("--non-interactive flag not registered")
	}
	if initCmd.Flags().Lookup("engine-wg-pubkey") == nil {
		t.Error("--engine-wg-pubkey flag not registered")
	}
}

func TestApplyCLIInit_Success(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })
	mockTunnelOps(t)

	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, "banyan.yaml")

	inputs := &cliInitInputs{
		EngineHost:     "192.168.1.10",
		EnginePort:     "50051",
		CLIName:        "cli-test",
		EngineWGPubKey: "dGVzdC1lbmdpbmUtd2cta2V5",
		PrivKey:        "dGVzdC1wcml2YXRlLWtleQ==",
		PubKey:         "dGVzdC1wdWJsaWMta2V5",
		KeysDir:        filepath.Join(tmpDir, "keys"),
	}

	if err := applyCLIInit(inputs); err != nil {
		t.Fatalf("applyCLIInit failed: %v", err)
	}

	// Verify config was saved correctly
	cfg, err := types.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.CLI.EngineHost != "192.168.1.10" {
		t.Errorf("expected engine host '192.168.1.10', got %q", cfg.CLI.EngineHost)
	}
	if cfg.CLI.EnginePort != "50051" {
		t.Errorf("expected engine port '50051', got %q", cfg.CLI.EnginePort)
	}
	if cfg.CLI.Name != "cli-test" {
		t.Errorf("expected CLI name 'cli-test', got %q", cfg.CLI.Name)
	}
	if cfg.CLI.EngineWGPublicKey != "dGVzdC1lbmdpbmUtd2cta2V5" {
		t.Errorf("expected engine WG public key, got %q", cfg.CLI.EngineWGPublicKey)
	}
	if cfg.CLI.WGPublicKey != "dGVzdC1wdWJsaWMta2V5" {
		t.Errorf("expected WG public key, got %q", cfg.CLI.WGPublicKey)
	}
	if cfg.CLI.WGPrivateKeyFile == "" {
		t.Error("expected non-empty WG private key file path")
	}
}

func TestApplyCLIInit_MissingEngineWGKey(t *testing.T) {
	inputs := &cliInitInputs{
		EngineHost:     "192.168.1.10",
		EnginePort:     "50051",
		CLIName:        "cli-test",
		EngineWGPubKey: "",
		PrivKey:        "dGVzdC1wcml2YXRlLWtleQ==",
		PubKey:         "dGVzdC1wdWJsaWMta2V5",
		KeysDir:        t.TempDir(),
	}

	err := applyCLIInit(inputs)
	if err == nil {
		t.Fatal("expected error for missing engine WG key")
	}
	if !strings.Contains(err.Error(), "engine WireGuard public key is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestApplyCLIInit_PreservesOtherConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })
	mockTunnelOps(t)

	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, "banyan.yaml")

	// Write a config with agent section
	existingCfg := types.BanyanConfig{
		Agent: types.AgentConfig{
			AgentName:  "worker-1",
			EngineHost: "10.0.0.1",
		},
	}
	if err := types.SaveConfig(configPath, &existingCfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	inputs := &cliInitInputs{
		EngineHost:     "192.168.1.10",
		EnginePort:     "50051",
		CLIName:        "cli-test",
		EngineWGPubKey: "dGVzdC1lbmdpbmUtd2cta2V5",
		PrivKey:        "dGVzdC1wcml2YXRlLWtleQ==",
		PubKey:         "dGVzdC1wdWJsaWMta2V5",
		KeysDir:        filepath.Join(tmpDir, "keys"),
	}

	if err := applyCLIInit(inputs); err != nil {
		t.Fatalf("applyCLIInit failed: %v", err)
	}

	// Verify agent section was preserved
	cfg, _ := types.LoadConfig(configPath)
	if cfg.Agent.AgentName != "worker-1" {
		t.Errorf("expected agent name 'worker-1' preserved, got %q", cfg.Agent.AgentName)
	}
}

func TestApplyCLIInit_WriteKeyFails(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, "banyan.yaml")

	inputs := &cliInitInputs{
		EngineHost:     "192.168.1.10",
		EnginePort:     "50051",
		CLIName:        "cli-test",
		EngineWGPubKey: "dGVzdC1lbmdpbmUtd2cta2V5",
		PrivKey:        "dGVzdC1wcml2YXRlLWtleQ==",
		PubKey:         "dGVzdC1wdWJsaWMta2V5",
		KeysDir:        "/nonexistent/path/keys", // unwritable directory
	}

	err := applyCLIInit(inputs)
	if err == nil {
		t.Fatal("expected error for unwritable keys directory")
	}
	if !strings.Contains(err.Error(), "failed to write private key") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSetupCLITunnel_Success(t *testing.T) {
	mockTunnelOps(t)

	inputs := &cliInitInputs{
		EngineHost:     "192.168.1.10",
		EngineWGPubKey: "dGVzdC1lbmdpbmUtd2cta2V5",
		PrivKey:        "dGVzdC1wcml2YXRlLWtleQ==",
		PubKey:         "dGVzdC1wdWJsaWMta2V5",
	}

	if err := setupCLITunnel(inputs); err != nil {
		t.Fatalf("setupCLITunnel failed: %v", err)
	}
}

func TestSetupCLITunnel_SetupFails(t *testing.T) {
	origSetup := setupControlTunnelFn
	origAdd := addControlPeerFn
	origCleanup := cleanupControlTunnelFn
	setupControlTunnelFn = func(string, string, net.IP, int) error {
		return net.UnknownNetworkError("wg module not loaded")
	}
	addControlPeerFn = func(string, string, string, net.IP) error { return nil }
	cleanupControlTunnelFn = func(string) error { return nil }
	t.Cleanup(func() {
		setupControlTunnelFn = origSetup
		addControlPeerFn = origAdd
		cleanupControlTunnelFn = origCleanup
	})

	inputs := &cliInitInputs{
		EngineHost:     "192.168.1.10",
		EngineWGPubKey: "dGVzdC1lbmdpbmUtd2cta2V5",
		PrivKey:        "dGVzdC1wcml2YXRlLWtleQ==",
		PubKey:         "dGVzdC1wdWJsaWMta2V5",
	}

	err := setupCLITunnel(inputs)
	if err == nil {
		t.Fatal("expected error when tunnel setup fails")
	}
	if !strings.Contains(err.Error(), "control tunnel setup failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSetupCLITunnel_AddPeerFails(t *testing.T) {
	origSetup := setupControlTunnelFn
	origAdd := addControlPeerFn
	origCleanup := cleanupControlTunnelFn
	cleanupCalled := false
	setupControlTunnelFn = func(string, string, net.IP, int) error { return nil }
	addControlPeerFn = func(string, string, string, net.IP) error {
		return net.UnknownNetworkError("peer add failed")
	}
	cleanupControlTunnelFn = func(string) error {
		cleanupCalled = true
		return nil
	}
	t.Cleanup(func() {
		setupControlTunnelFn = origSetup
		addControlPeerFn = origAdd
		cleanupControlTunnelFn = origCleanup
	})

	inputs := &cliInitInputs{
		EngineHost:     "192.168.1.10",
		EngineWGPubKey: "dGVzdC1lbmdpbmUtd2cta2V5",
		PrivKey:        "dGVzdC1wcml2YXRlLWtleQ==",
		PubKey:         "dGVzdC1wdWJsaWMta2V5",
	}

	err := setupCLITunnel(inputs)
	if err == nil {
		t.Fatal("expected error when add peer fails")
	}
	if !strings.Contains(err.Error(), "failed to add engine peer") {
		t.Errorf("unexpected error: %v", err)
	}
	if !cleanupCalled {
		t.Error("expected cleanup to be called when add peer fails")
	}
}
