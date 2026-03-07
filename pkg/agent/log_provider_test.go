package agent

import (
	"context"
	"os/exec"
	"testing"

	"github.com/fertile-org/banyan/pkg/types"
)

func TestNerdctlLogProvider_StreamLogs_NoNerdctl(t *testing.T) {
	// When nerdctl is not available, StreamLogs should return an error
	// from cmd.Start (exec: not found or similar).
	_, err := (&NerdctlLogProvider{}).StreamLogs(
		context.Background(),
		"nonexistent-container",
		types.LogOptions{},
	)
	// On most CI systems nerdctl isn't installed — verify graceful failure
	if err == nil {
		// nerdctl is actually available; skip this test
		t.Skip("nerdctl is available on this machine")
	}
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestNerdctlLogProvider_StreamLogs_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := (&NerdctlLogProvider{}).StreamLogs(
		ctx,
		"test-container",
		types.LogOptions{Follow: true, Tail: 100},
	)
	// Should fail because context is cancelled
	if err == nil {
		t.Skip("nerdctl is available and started despite cancelled context")
	}
}

func TestCmdReadCloser_Close(t *testing.T) {
	// Use a simple command that produces output
	cmd := exec.Command("echo", "hello")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	rc := &cmdReadCloser{cmd: cmd, reader: stdout}

	// Read some data
	buf := make([]byte, 100)
	n, _ := rc.Read(buf)
	if n == 0 {
		t.Error("expected to read some data")
	}
	got := string(buf[:n])
	if got != "hello\n" {
		t.Errorf("Read() = %q, want %q", got, "hello\n")
	}

	// Close should not error
	if err := rc.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestCmdReadCloser_Close_KillsRunningProcess(t *testing.T) {
	// Use a long-running command
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "sleep", "60")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	rc := &cmdReadCloser{cmd: cmd, reader: stdout}

	// Close should kill the process and not hang
	if err := rc.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify process exited
	if cmd.ProcessState == nil {
		t.Error("expected process to have exited after Close()")
	}
}

func TestCmdReadCloser_Close_ProcessAlreadyExited(t *testing.T) {
	// Test Close when the process has already finished
	cmd := exec.Command("true") // Exits immediately
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Wait for it to exit naturally
	_ = cmd.Wait()

	rc := &cmdReadCloser{cmd: cmd, reader: stdout}

	// Close should handle already-exited process gracefully
	if err := rc.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
