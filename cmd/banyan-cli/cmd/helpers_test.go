package cmd

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/types"
)

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{5 * time.Second, "5s"},
		{45 * time.Second, "45s"},
		{3 * time.Minute, "3m"},
		{90 * time.Minute, "1h30m"},
		{2 * time.Hour, "2h"},
		{25 * time.Hour, "1d"},
		{72 * time.Hour, "3d"},
		{-10 * time.Second, "10s"}, // negative handled
	}
	for _, tt := range tests {
		got := humanDuration(tt.d)
		if got != tt.want {
			t.Errorf("humanDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatPercent(t *testing.T) {
	tests := []struct {
		ratio float64
		want  string
	}{
		{0.0, "0.0%"},
		{0.5, "50.0%"},
		{1.0, "100.0%"},
		{0.256, "25.6%"},
	}
	for _, tt := range tests {
		got := formatPercent(tt.ratio)
		if got != tt.want {
			t.Errorf("formatPercent(%v) = %q, want %q", tt.ratio, got, tt.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes uint64
		want  string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.0KB"},
		{1048576, "1.0MB"},
		{1073741824, "1.0GB"},
		{2147483648, "2.0GB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		s     string
		width int
		want  string
	}{
		{"hi", 5, "hi   "},
		{"hello", 3, "hello"},
		{"", 3, "   "},
	}
	for _, tt := range tests {
		got := padRight(tt.s, tt.width)
		if got != tt.want {
			t.Errorf("padRight(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
		}
	}
}

func TestPrintJSON(t *testing.T) {
	t.Run("valid value", func(t *testing.T) {
		err := printJSON(map[string]string{"key": "value"})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("unmarshalable value", func(t *testing.T) {
		err := printJSON(make(chan int))
		if err == nil {
			t.Fatal("expected error for unmarshalable value")
		}
	})
}

func TestFetchClusterData_NoConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	configPath = "/tmp/nonexistent-cli-config.yaml"

	_, err := fetchClusterData()
	if err == nil {
		t.Fatal("expected error when no config")
	}
}

func TestFetchClusterData_NoEngineWGKey(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")
	cfg := types.BanyanConfig{
		CLI: types.CLIConfig{
			EngineHost:  "127.0.0.1",
			EnginePort:  "50051",
			WGPublicKey: "test-pubkey",
			// No EngineWGPublicKey — should fail
		},
	}
	if err := types.SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	configPath = cfgPath

	_, err := fetchClusterData()
	if err == nil {
		t.Fatal("expected error when engine WG key missing")
	}
}

func TestFetchClusterData_WithServer(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	data, err := fetchClusterData()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
	if data.Engine.Status != "running" {
		t.Errorf("expected engine status 'running', got %q", data.Engine.Status)
	}
	if len(data.Agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(data.Agents))
	}
	if len(data.Deployments) != 1 {
		t.Errorf("expected 1 deployment, got %d", len(data.Deployments))
	}
}
