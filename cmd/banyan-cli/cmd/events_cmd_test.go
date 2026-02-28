package cmd

import (
	"testing"
	"time"

	"github.com/fertile-org/banyan/cmd/banyan-cli/dashboard"
)

func TestRunEvents_NoConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	configPath = "/tmp/nonexistent-cli-config.yaml"

	err := runEvents(eventsCmd, nil)
	if err == nil {
		t.Fatal("expected error when no config")
	}
}

func TestRunEvents_WithServer(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	origTail := eventsTail
	t.Cleanup(func() {
		outputFormat = origFmt
		eventsTail = origTail
	})
	outputFormat = ""
	eventsTail = 50

	err := runEvents(eventsCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunEvents_WithTail(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	origTail := eventsTail
	t.Cleanup(func() {
		outputFormat = origFmt
		eventsTail = origTail
	})
	outputFormat = ""
	eventsTail = 1

	err := runEvents(eventsCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunEvents_JSON(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origFmt := outputFormat
	origTail := eventsTail
	t.Cleanup(func() {
		outputFormat = origFmt
		eventsTail = origTail
	})
	outputFormat = "json"
	eventsTail = 50

	err := runEvents(eventsCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestListEvents_Empty(t *testing.T) {
	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	err := listEvents(nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestListEvents_WithData(t *testing.T) {
	origFmt := outputFormat
	t.Cleanup(func() { outputFormat = origFmt })
	outputFormat = ""

	events := []dashboard.EventData{
		{
			Timestamp: time.Now().Add(-1 * time.Minute),
			Type:      "deployment.updated",
			Message:   "Deployment my-app updated",
			Severity:  "info",
		},
		{
			Timestamp: time.Now().Add(-5 * time.Minute),
			Type:      "container.stopped",
			Message:   "Container my-app-api-0 stopped",
			Severity:  "warning",
		},
	}

	err := listEvents(events)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}
