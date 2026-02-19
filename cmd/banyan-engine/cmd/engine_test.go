package cmd

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

func TestResolveStoreConfig(t *testing.T) {
	// This tests the default behavior when no flags are changed.
	// The function reads from config which defaults to "badger".
	t.Run("returns defaults from config", func(t *testing.T) {
		original := configPath
		t.Cleanup(func() { configPath = original })

		configPath = "/tmp/nonexistent-banyan-test-config.yaml"
		// With no config, should return default "badger" backend
		backend, _ := resolveStoreConfig(startCmd)
		if backend != "badger" {
			t.Errorf("expected default backend 'badger', got %q", backend)
		}
	})
}

func TestResolveDefaultStoreAddress(t *testing.T) {
	t.Run("badger defaults to datadir/store", func(t *testing.T) {
		result := resolveDefaultStoreAddress("badger", "", "/var/lib/banyan")
		if result != "/var/lib/banyan/store" {
			t.Errorf("expected '/var/lib/banyan/store', got %q", result)
		}
	})

	t.Run("redis defaults to localhost:6379", func(t *testing.T) {
		result := resolveDefaultStoreAddress("redis", "", "/var/lib/banyan")
		if result != "localhost:6379" {
			t.Errorf("expected 'localhost:6379', got %q", result)
		}
	})

	t.Run("etcd defaults to http://localhost:2379", func(t *testing.T) {
		result := resolveDefaultStoreAddress("etcd", "", "/var/lib/banyan")
		if result != "http://localhost:2379" {
			t.Errorf("expected 'http://localhost:2379', got %q", result)
		}
	})

	t.Run("explicit address is preserved", func(t *testing.T) {
		result := resolveDefaultStoreAddress("redis", "10.0.0.1:6379", "/var/lib/banyan")
		if result != "10.0.0.1:6379" {
			t.Errorf("expected '10.0.0.1:6379', got %q", result)
		}
	})

	t.Run("unknown backend returns empty", func(t *testing.T) {
		result := resolveDefaultStoreAddress("unknown", "", "/var/lib/banyan")
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

// setupTestConfigAndStore creates a config file with a password and a badger store with the auth hash.
func setupTestConfigAndStore(t *testing.T) (tmpDir string) {
	t.Helper()
	tmpDir = t.TempDir()

	// Create config with a password
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")
	cfg := types.BanyanConfig{
		Security: types.SecurityConfig{
			AuthType: "password",
			Password: "test-pass",
		},
	}
	if err := types.SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	configPath = cfgPath

	// Create badger store with the auth hash
	storeDir := filepath.Join(tmpDir, "store")
	store, err := storage.NewStore("badger", storeDir, "/banyan")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	store.Save(context.Background(), types.KeyAuthHash, types.HashPassword("test-pass"))
	store.Close()

	return tmpDir
}

func TestRunEngineStatus_EmptyStore(t *testing.T) {
	origConfig := configPath
	origDataDir := engineDataDir
	t.Cleanup(func() {
		configPath = origConfig
		engineDataDir = origDataDir
	})

	tmpDir := setupTestConfigAndStore(t)
	engineDataDir = tmpDir

	err := runEngineStatus(statusCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunEngineStatus_WithData(t *testing.T) {
	origConfig := configPath
	origDataDir := engineDataDir
	t.Cleanup(func() {
		configPath = origConfig
		engineDataDir = origDataDir
	})

	tmpDir := setupTestConfigAndStore(t)
	engineDataDir = tmpDir

	// Add more data to the store
	storeDir := filepath.Join(tmpDir, "store")
	store, err := storage.NewStore("badger", storeDir, "/banyan")
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	ctx := context.Background()
	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name: "agent-1", Status: "ready",
	})
	store.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
		ID: "deploy-1", Name: "my-app", Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"web": {Image: "nginx", Replicas: 1},
		},
	})
	store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID: "task-1", DeploymentID: "deploy-1", AgentID: "agent-1",
		ServiceName: "web", ContainerName: "my-app-web-0",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerStatus: types.StatusRunning,
	})
	store.Close()

	err = runEngineStatus(statusCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
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
			StoreBackend: "redis",
			StoreAddress: "localhost:19999", // unreachable
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
	origConfig := configPath
	origDataDir := engineDataDir
	t.Cleanup(func() {
		configPath = origConfig
		engineDataDir = origDataDir
	})

	tmpDir := t.TempDir()

	// Create config with wrong password
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")
	cfg := types.BanyanConfig{
		Security: types.SecurityConfig{
			AuthType: "password",
			Password: "wrong-pass",
		},
	}
	if err := types.SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	configPath = cfgPath
	engineDataDir = tmpDir

	// Create badger store with a different auth hash
	storeDir := filepath.Join(tmpDir, "store")
	store, err := storage.NewStore("badger", storeDir, "/banyan")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	store.Save(context.Background(), types.KeyAuthHash, types.HashPassword("correct-pass"))
	store.Close()

	// runEngineStatus should return auth error
	err = runEngineStatus(statusCmd, nil)
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestResolveStoreConfig_FlagOverrides(t *testing.T) {
	original := configPath
	t.Cleanup(func() { configPath = original })

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")
	cfg := types.BanyanConfig{
		Engine: types.EngineConfig{
			StoreBackend: "badger",
			StoreAddress: "/var/lib/banyan/store",
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
			StoreAddress: "localhost:6379",
		},
	}
	if err := types.SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	configPath = cfgPath

	backend, address := resolveStoreConfig(statusCmd)
	// Backend should default to "badger" since StoreBackend is empty
	if backend != "badger" {
		t.Errorf("expected default backend 'badger', got %q", backend)
	}
	if address != "localhost:6379" {
		t.Errorf("expected address 'localhost:6379', got %q", address)
	}
}

func TestRunEngineStatus_WithRichData(t *testing.T) {
	origConfig := configPath
	origDataDir := engineDataDir
	t.Cleanup(func() {
		configPath = origConfig
		engineDataDir = origDataDir
	})

	tmpDir := setupTestConfigAndStore(t)
	engineDataDir = tmpDir

	// Add rich data: agents, deployments with services, tasks with container statuses
	storeDir := filepath.Join(tmpDir, "store")
	store, err := storage.NewStore("badger", storeDir, "/banyan")
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	ctx := context.Background()

	// Add agents
	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name: "agent-1", Status: "ready", APIAddress: "agent-1:50052",
		LastSeen: time.Now().Add(-5 * time.Second), CreatedAt: time.Now(),
	})
	store.Save(ctx, types.KeyNodes+"agent-2", &types.NodeRecord{
		Name: "agent-2", Status: "ready", APIAddress: "agent-2:50052",
		LastSeen: time.Now().Add(-30 * time.Second), CreatedAt: time.Now(),
	})

	// Add deployment with multiple services
	store.Save(ctx, types.KeyDeployments+"deploy-rich", &types.DeploymentRecord{
		ID:     "deploy-rich",
		Name:   "rich-app",
		Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"web": {Image: "nginx", Replicas: 2},
			"api": {Image: "node", Replicas: 1},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	// Tasks for web service - one running, one pending
	store.Save(ctx, types.KeyTasks+"agent-1/task-web-0", &types.TaskRecord{
		ID: "task-web-0", DeploymentID: "deploy-rich", AgentID: "agent-1",
		ServiceName: "web", ReplicaIndex: 0,
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "rich-app-web-0", ContainerStatus: types.StatusRunning,
		ContainerCheckedAt: time.Now().Add(-2 * time.Second),
	})
	store.Save(ctx, types.KeyTasks+"agent-2/task-web-1", &types.TaskRecord{
		ID: "task-web-1", DeploymentID: "deploy-rich", AgentID: "agent-2",
		ServiceName: "web", ReplicaIndex: 1,
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "rich-app-web-1", ContainerStatus: "", // empty → "pending" fallback
	})

	// Task for api service - running with checked timestamp
	store.Save(ctx, types.KeyTasks+"agent-1/task-api-0", &types.TaskRecord{
		ID: "task-api-0", DeploymentID: "deploy-rich", AgentID: "agent-1",
		ServiceName: "api", ReplicaIndex: 0,
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "rich-app-api-0", ContainerStatus: types.StatusRunning,
		ContainerCheckedAt: time.Now().Add(-10 * time.Second),
	})

	store.Close()

	err = runEngineStatus(statusCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunEngineStatus_NoConfig(t *testing.T) {
	origConfig := configPath
	origDataDir := engineDataDir
	t.Cleanup(func() {
		configPath = origConfig
		engineDataDir = origDataDir
	})

	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, "nonexistent.yaml")
	engineDataDir = tmpDir

	// With no config file, LoadConfig returns empty config (password=""),
	// so VerifyAuth returns "authentication required".
	err := runEngineStatus(statusCmd, nil)
	if err == nil {
		t.Fatal("expected auth error, got nil")
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
				StoreBackend: "redis",
				StoreAddress: "10.0.0.5:6379",
			},
		}
		if err := types.SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}
		configPath = cfgPath

		backend, address := resolveStoreConfig(startCmd)
		if backend != "redis" {
			t.Errorf("expected backend 'redis', got %q", backend)
		}
		if address != "10.0.0.5:6379" {
			t.Errorf("expected address '10.0.0.5:6379', got %q", address)
		}
	})
}
