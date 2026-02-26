package types

import (
	"testing"
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
