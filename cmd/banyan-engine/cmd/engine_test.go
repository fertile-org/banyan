package cmd

import (
	"testing"
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
