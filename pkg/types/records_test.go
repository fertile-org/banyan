package types

import (
	"testing"
	"time"
)

func TestKeyConstants(t *testing.T) {
	t.Run("etcd key prefixes", func(t *testing.T) {
		if KeyDeployments != "deployments/" {
			t.Errorf("expected deployments/, got %s", KeyDeployments)
		}
		if KeyNodes != "nodes/" {
			t.Errorf("expected nodes/, got %s", KeyNodes)
		}
		if KeyTasks != "tasks/" {
			t.Errorf("expected tasks/, got %s", KeyTasks)
		}
		if KeyRegistry != "config/registry" {
			t.Errorf("expected config/registry, got %s", KeyRegistry)
		}
		if KeyTokens != "tokens/" {
			t.Errorf("expected tokens/, got %s", KeyTokens)
		}
		if KeyTokenIndex != "token-index/" {
			t.Errorf("expected token-index/, got %s", KeyTokenIndex)
		}
	})

	t.Run("deployment statuses", func(t *testing.T) {
		if StatusPending != "pending" {
			t.Errorf("expected pending, got %s", StatusPending)
		}
		if StatusDeploying != "deploying" {
			t.Errorf("expected deploying, got %s", StatusDeploying)
		}
		if StatusRunning != "running" {
			t.Errorf("expected running, got %s", StatusRunning)
		}
		if StatusFailed != "failed" {
			t.Errorf("expected failed, got %s", StatusFailed)
		}
		if StatusCompleted != "completed" {
			t.Errorf("expected completed, got %s", StatusCompleted)
		}
		if StatusStopping != "stopping" {
			t.Errorf("expected stopping, got %s", StatusStopping)
		}
		if StatusStopped != "stopped" {
			t.Errorf("expected stopped, got %s", StatusStopped)
		}
	})

	t.Run("task types", func(t *testing.T) {
		if TaskTypeCreateAndStart != "create_and_start" {
			t.Errorf("expected create_and_start, got %s", TaskTypeCreateAndStart)
		}
		if TaskTypeStopAndRemove != "stop_and_remove" {
			t.Errorf("expected stop_and_remove, got %s", TaskTypeStopAndRemove)
		}
	})
}

func TestDefaultCLITokenTTL(t *testing.T) {
	expected := 30 * 24 * time.Hour
	if DefaultCLITokenTTL != expected {
		t.Errorf("expected %v, got %v", expected, DefaultCLITokenTTL)
	}
}

func TestTokenRecord_IsExpired(t *testing.T) {
	t.Run("zero time is not expired", func(t *testing.T) {
		record := TokenRecord{Name: "test", Role: "cli"}
		if record.IsExpired() {
			t.Error("zero ExpiresAt should not be expired")
		}
	})

	t.Run("future time is not expired", func(t *testing.T) {
		record := TokenRecord{
			Name:      "test",
			Role:      "cli",
			ExpiresAt: time.Now().Add(time.Hour),
		}
		if record.IsExpired() {
			t.Error("future ExpiresAt should not be expired")
		}
	})

	t.Run("past time is expired", func(t *testing.T) {
		record := TokenRecord{
			Name:      "test",
			Role:      "cli",
			ExpiresAt: time.Now().Add(-time.Hour),
		}
		if !record.IsExpired() {
			t.Error("past ExpiresAt should be expired")
		}
	})
}
