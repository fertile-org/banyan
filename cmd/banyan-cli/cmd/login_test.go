package cmd

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fertile-org/banyan/pkg/types"
)

func TestRunLogin_Success(t *testing.T) {
	origConfig := configPath
	origTunnelExists := controlTunnelExistsFn
	t.Cleanup(func() {
		configPath = origConfig
		controlTunnelExistsFn = origTunnelExists
	})
	mockTunnelOps(t)

	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, "banyan.yaml")

	// Write a private key file
	keysDir := filepath.Join(tmpDir, "keys")
	keyPath, err := types.WritePrivateKeyFile(keysDir, "cli", "dGVzdC1wcml2YXRlLWtleQ==")
	if err != nil {
		t.Fatalf("WritePrivateKeyFile failed: %v", err)
	}

	// Write a valid config
	cfg := types.BanyanConfig{
		CLI: types.CLIConfig{
			EngineHost:        "192.168.1.10",
			EnginePort:        "50051",
			WGPrivateKeyFile:  keyPath,
			WGPublicKey:       "dGVzdC1wdWJsaWMta2V5",
			EngineWGPublicKey: "dGVzdC1lbmdpbmUtd2cta2V5",
		},
	}
	if err := types.SaveConfig(configPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Tunnel doesn't exist yet
	controlTunnelExistsFn = func(string) bool { return false }

	if err := runLogin(nil, nil); err != nil {
		t.Fatalf("runLogin failed: %v", err)
	}
}

func TestRunLogin_TunnelAlreadyActive(t *testing.T) {
	origConfig := configPath
	origTunnelExists := controlTunnelExistsFn
	t.Cleanup(func() {
		configPath = origConfig
		controlTunnelExistsFn = origTunnelExists
	})

	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, "banyan.yaml")

	cfg := types.BanyanConfig{
		CLI: types.CLIConfig{
			EngineHost:        "192.168.1.10",
			EnginePort:        "50051",
			WGPrivateKeyFile:  "/some/key",
			WGPublicKey:       "dGVzdC1wdWJsaWMta2V5",
			EngineWGPublicKey: "dGVzdC1lbmdpbmUtd2cta2V5",
		},
	}
	if err := types.SaveConfig(configPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Tunnel already exists
	controlTunnelExistsFn = func(string) bool { return true }

	if err := runLogin(nil, nil); err != nil {
		t.Fatalf("runLogin should succeed when tunnel already active: %v", err)
	}
}

func TestRunLogin_IncompleteConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, "banyan.yaml")

	// Write config missing WGPublicKey
	cfg := types.BanyanConfig{
		CLI: types.CLIConfig{
			EngineHost: "192.168.1.10",
		},
	}
	if err := types.SaveConfig(configPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	err := runLogin(nil, nil)
	if err == nil {
		t.Fatal("expected error for incomplete config")
	}
	if !strings.Contains(err.Error(), "incomplete CLI configuration") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunLogin_NoConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	configPath = filepath.Join(t.TempDir(), "nonexistent.yaml")

	err := runLogin(nil, nil)
	if err == nil {
		t.Fatal("expected error when config doesn't exist")
	}
	if !strings.Contains(err.Error(), "incomplete CLI configuration") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunLogin_BadPrivateKeyFile(t *testing.T) {
	origConfig := configPath
	origTunnelExists := controlTunnelExistsFn
	t.Cleanup(func() {
		configPath = origConfig
		controlTunnelExistsFn = origTunnelExists
	})

	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, "banyan.yaml")

	cfg := types.BanyanConfig{
		CLI: types.CLIConfig{
			EngineHost:        "192.168.1.10",
			EnginePort:        "50051",
			WGPrivateKeyFile:  "/nonexistent/key.pem",
			WGPublicKey:       "dGVzdC1wdWJsaWMta2V5",
			EngineWGPublicKey: "dGVzdC1lbmdpbmUtd2cta2V5",
		},
	}
	if err := types.SaveConfig(configPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	controlTunnelExistsFn = func(string) bool { return false }

	err := runLogin(nil, nil)
	if err == nil {
		t.Fatal("expected error for missing private key file")
	}
	if !strings.Contains(err.Error(), "failed to read private key") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunLogin_EmptyPrivateKeyFile(t *testing.T) {
	origConfig := configPath
	origTunnelExists := controlTunnelExistsFn
	t.Cleanup(func() {
		configPath = origConfig
		controlTunnelExistsFn = origTunnelExists
	})

	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, "banyan.yaml")

	// Create an empty key file
	keyPath := filepath.Join(tmpDir, "empty.key")
	if err := os.WriteFile(keyPath, []byte(""), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg := types.BanyanConfig{
		CLI: types.CLIConfig{
			EngineHost:        "192.168.1.10",
			EnginePort:        "50051",
			WGPrivateKeyFile:  keyPath,
			WGPublicKey:       "dGVzdC1wdWJsaWMta2V5",
			EngineWGPublicKey: "dGVzdC1lbmdpbmUtd2cta2V5",
		},
	}
	if err := types.SaveConfig(configPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	controlTunnelExistsFn = func(string) bool { return false }

	err := runLogin(nil, nil)
	if err == nil {
		t.Fatal("expected error for empty private key file")
	}
	if !strings.Contains(err.Error(), "failed to read private key") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoginSetupTunnel_Success(t *testing.T) {
	mockTunnelOps(t)

	cfg := types.BanyanConfig{
		CLI: types.CLIConfig{
			EngineHost:        "192.168.1.10",
			WGPublicKey:       "dGVzdC1wdWJsaWMta2V5",
			EngineWGPublicKey: "dGVzdC1lbmdpbmUtd2cta2V5",
		},
	}

	if err := loginSetupTunnel(&cfg, "dGVzdC1wcml2YXRlLWtleQ=="); err != nil {
		t.Fatalf("loginSetupTunnel failed: %v", err)
	}
}

func TestLoginSetupTunnel_SetupFails(t *testing.T) {
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

	cfg := types.BanyanConfig{
		CLI: types.CLIConfig{
			EngineHost:        "192.168.1.10",
			WGPublicKey:       "dGVzdC1wdWJsaWMta2V5",
			EngineWGPublicKey: "dGVzdC1lbmdpbmUtd2cta2V5",
		},
	}

	err := loginSetupTunnel(&cfg, "dGVzdC1wcml2YXRlLWtleQ==")
	if err == nil {
		t.Fatal("expected error when tunnel setup fails")
	}
	if !strings.Contains(err.Error(), "control tunnel setup failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoginSetupTunnel_AddPeerFails(t *testing.T) {
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

	cfg := types.BanyanConfig{
		CLI: types.CLIConfig{
			EngineHost:        "192.168.1.10",
			WGPublicKey:       "dGVzdC1wdWJsaWMta2V5",
			EngineWGPublicKey: "dGVzdC1lbmdpbmUtd2cta2V5",
		},
	}

	err := loginSetupTunnel(&cfg, "dGVzdC1wcml2YXRlLWtleQ==")
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
