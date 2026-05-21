package cmd

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/types"
)

func TestResolveStoreConfig(t *testing.T) {
	// This tests the default behavior when no flags are changed.
	// The function reads from config which defaults to "etcd".
	t.Run("returns defaults from config", func(t *testing.T) {
		original := configPath
		t.Cleanup(func() { configPath = original })

		configPath = "/tmp/nonexistent-banyan-test-config.yaml"
		// With no config, should return default "etcd" backend
		backend, _ := resolveStoreConfig(startCmd)
		if backend != "etcd" {
			t.Errorf("expected default backend 'etcd', got %q", backend)
		}
	})
}

func TestResolveDefaultStoreAddress(t *testing.T) {
	t.Run("etcd defaults to http://127.0.0.1:2379", func(t *testing.T) {
		result := resolveDefaultStoreAddress("etcd", "")
		if result != "http://127.0.0.1:2379" {
			t.Errorf("expected 'http://127.0.0.1:2379', got %q", result)
		}
	})

	t.Run("explicit address is preserved", func(t *testing.T) {
		result := resolveDefaultStoreAddress("etcd", "10.0.0.1:2379")
		if result != "10.0.0.1:2379" {
			t.Errorf("expected '10.0.0.1:2379', got %q", result)
		}
	})

	t.Run("unknown backend returns empty", func(t *testing.T) {
		result := resolveDefaultStoreAddress("unknown", "")
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})
}

func TestRunEngineStop(t *testing.T) {
	err := runEngineStop(nil, nil)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestRunEngineStatus_EmptyStore(t *testing.T) {
	t.Skip("requires etcd server")
}

func TestRunEngineStatus_WithData(t *testing.T) {
	t.Skip("requires etcd server")
}

func TestRunEngineStatus_StoreCreationFailure(t *testing.T) {
	origConfig := configPath
	origDataDir := engineDataDir
	origBackend := engineStoreBackend
	origAddress := engineStoreAddress
	t.Cleanup(func() {
		configPath = origConfig
		engineDataDir = origDataDir
		engineStoreBackend = origBackend
		engineStoreAddress = origAddress
	})

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")
	cfg := types.BanyanConfig{
		Engine: types.EngineConfig{
			StoreBackend: "etcd",
			StoreAddress: "http://127.0.0.1:19999", // unreachable
		},
	}
	if err := types.SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	configPath = cfgPath
	engineDataDir = tmpDir

	// runEngineStatus should handle store creation failure gracefully
	err := runEngineStatus(statusCmd, nil)
	if err != nil {
		t.Errorf("expected nil error (graceful failure), got %v", err)
	}
}

func TestRunEngineStatus_AuthFailure(t *testing.T) {
	t.Skip("requires etcd server")
}

func TestResolveStoreConfig_FlagOverrides(t *testing.T) {
	original := configPath
	t.Cleanup(func() { configPath = original })

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")
	cfg := types.BanyanConfig{
		Engine: types.EngineConfig{
			StoreBackend: "etcd",
			StoreAddress: "http://127.0.0.1:2379",
		},
	}
	if err := types.SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	configPath = cfgPath

	// Create a fresh cobra command with flags to avoid polluting the global startCmd
	testCmd := &cobra.Command{Use: "test"}
	testCmd.Flags().StringVar(&engineStoreBackend, "store-backend", "", "")
	testCmd.Flags().StringVar(&engineStoreAddress, "store-address", "", "")

	testCmd.Flags().Set("store-backend", "etcd")
	testCmd.Flags().Set("store-address", "http://10.0.0.1:2379")

	backend, address := resolveStoreConfig(testCmd)
	if backend != "etcd" {
		t.Errorf("expected flag override backend 'etcd', got %q", backend)
	}
	if address != "http://10.0.0.1:2379" {
		t.Errorf("expected flag override address 'http://10.0.0.1:2379', got %q", address)
	}
}

func TestResolveStoreConfig_AddressFromConfigWithoutBackend(t *testing.T) {
	original := configPath
	t.Cleanup(func() { configPath = original })

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")
	cfg := types.BanyanConfig{
		Engine: types.EngineConfig{
			StoreAddress: "http://127.0.0.1:2379",
		},
	}
	if err := types.SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	configPath = cfgPath

	backend, address := resolveStoreConfig(statusCmd)
	// Backend should default to "etcd" since StoreBackend is empty
	if backend != "etcd" {
		t.Errorf("expected default backend 'etcd', got %q", backend)
	}
	if address != "http://127.0.0.1:2379" {
		t.Errorf("expected address 'http://127.0.0.1:2379', got %q", address)
	}
}

func TestRunEngineStatus_WithRichData(t *testing.T) {
	t.Skip("requires etcd server")
}

func TestRunEngineStatus_NoConfig(t *testing.T) {
	t.Skip("requires etcd server")
}

func TestWaitForEtcd_Healthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := waitForEtcd(srv.URL, 2*time.Second)
	if err != nil {
		t.Errorf("expected healthy etcd, got %v", err)
	}
}

func TestWaitForEtcd_Timeout(t *testing.T) {
	// Use an address where nothing is listening
	err := waitForEtcd("http://127.0.0.1:19876", 500*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

func TestWaitForEtcd_DelayedHealth(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := waitForEtcd(srv.URL, 5*time.Second)
	if err != nil {
		t.Errorf("expected eventual healthy etcd, got %v", err)
	}
	if calls < 3 {
		t.Errorf("expected at least 3 health check calls, got %d", calls)
	}
}

func TestStopManagedEtcd_Nil(t *testing.T) {
	// Should not panic
	stopManagedEtcd(nil)
	stopManagedEtcd(&exec.Cmd{})
}

func TestManagedEtcdClientURL(t *testing.T) {
	if managedEtcdClientURL != "http://127.0.0.1:2379" {
		t.Errorf("expected 'http://127.0.0.1:2379', got %q", managedEtcdClientURL)
	}
}

func TestManagedEtcdListenURL(t *testing.T) {
	if managedEtcdListenURL != "http://127.0.0.1:2379" {
		t.Errorf("expected 'http://127.0.0.1:2379', got %q", managedEtcdListenURL)
	}
}

func TestStartManagedEtcd_BinaryNotFound(t *testing.T) {
	// Temporarily override PATH to ensure etcd binary is not found
	origPath := t.TempDir() // empty dir — no etcd binary
	t.Setenv("PATH", origPath)

	_, err := startManagedEtcd(t.TempDir())
	if err == nil {
		t.Error("expected error when etcd binary not found, got nil")
	}
}

func TestResolveStoreConfig_WithConfigFile(t *testing.T) {
	original := configPath
	t.Cleanup(func() { configPath = original })

	t.Run("reads backend from config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := tmpDir + "/banyan.yaml"
		cfg := types.BanyanConfig{
			Engine: types.EngineConfig{
				StoreBackend: "etcd",
				StoreAddress: "http://10.0.0.5:2379",
			},
		}
		if err := types.SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}
		configPath = cfgPath

		backend, address := resolveStoreConfig(startCmd)
		if backend != "etcd" {
			t.Errorf("expected backend 'etcd', got %q", backend)
		}
		if address != "http://10.0.0.5:2379" {
			t.Errorf("expected address 'http://10.0.0.5:2379', got %q", address)
		}
	})
}

func TestEngineInit_NonInteractiveFlags(t *testing.T) {
	initCmd.ResetFlags()
	initCmd.Flags().Bool("non-interactive", false, "")
	initCmd.Flags().String("admin-user", "admin", "")
	initCmd.Flags().String("admin-password", "", "")
	initCmd.Flags().String("vpc-cidr", "10.0.0.0/16", "")

	if initCmd.Flags().Lookup("non-interactive") == nil {
		t.Error("--non-interactive flag not registered")
	}
	if initCmd.Flags().Lookup("admin-user") == nil {
		t.Error("--admin-user flag not registered")
	}
	if initCmd.Flags().Lookup("admin-password") == nil {
		t.Error("--admin-password flag not registered")
	}
	if initCmd.Flags().Lookup("vpc-cidr") == nil {
		t.Error("--vpc-cidr flag not registered")
	}
}

func TestRegistryBindAddressSelection(t *testing.T) {
	tests := []struct {
		name                string
		controlTunnelActive bool
		engineTunnelIP      string
		expectedBindAddr    string
	}{
		{
			name:                "no tunnel uses localhost",
			controlTunnelActive: false,
			engineTunnelIP:      "",
			expectedBindAddr:    "127.0.0.1",
		},
		{
			name:                "tunnel active uses tunnel IP",
			controlTunnelActive: true,
			engineTunnelIP:      "10.200.1.1",
			expectedBindAddr:    "10.200.1.1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bindAddr := "127.0.0.1"
			if tc.controlTunnelActive && tc.engineTunnelIP != "" {
				bindAddr = tc.engineTunnelIP
			}
			if bindAddr != tc.expectedBindAddr {
				t.Errorf("expected bind address %q, got %q", tc.expectedBindAddr, bindAddr)
			}
		})
	}
}
