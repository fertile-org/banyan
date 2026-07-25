package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQemuCPUName(t *testing.T) {
	cases := map[string]string{
		"linux/arm64": "aarch64",
		"arm64":       "aarch64",
		"linux/amd64": "x86_64",
		"amd64":       "x86_64",
	}
	for platform, want := range cases {
		if got := qemuCPUName(platform); got != want {
			t.Errorf("qemuCPUName(%q) = %q, want %q", platform, got, want)
		}
	}
}

func TestBinfmtRegistered(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "qemu-aarch64"), []byte("enabled\ninterpreter /usr/bin/qemu-aarch64-static\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !binfmtRegistered(dir, "aarch64") {
		t.Error("expected aarch64 to be registered")
	}
	if binfmtRegistered(dir, "x86_64") {
		t.Error("expected x86_64 to be unregistered (no file)")
	}
}

func TestBinfmtRegistered_DisabledHandler(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "qemu-aarch64"), []byte("disabled\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if binfmtRegistered(dir, "aarch64") {
		t.Error("expected disabled handler to count as not registered")
	}
}

func TestCheckEmulationIn(t *testing.T) {
	host := hostCPU() // whatever arch the test runs on
	// A registered dir with only the host handler present.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "qemu-"+host), []byte("enabled\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The non-host CPU name (the one that will be missing from dir).
	foreign := "aarch64"
	if host == "aarch64" {
		foreign = "x86_64"
	}
	foreignArch := "arm64"
	if foreign == "x86_64" {
		foreignArch = "amd64"
	}

	t.Run("host-only platform passes", func(t *testing.T) {
		if err := checkEmulationIn(dir, []string{"linux/" + archOfCPU(host)}); err != nil {
			t.Errorf("expected nil (host arch needs no emulation), got %v", err)
		}
	})

	t.Run("comma-joined multi-arch detects missing foreign handler", func(t *testing.T) {
		// "<host>,<foreign>" — the foreign half must be caught even though the
		// host half (and, on some hosts, its trailing position) would be skipped.
		combo := "linux/" + archOfCPU(host) + ",linux/" + foreignArch
		err := checkEmulationIn(dir, []string{combo})
		if err == nil {
			t.Fatalf("expected error for missing %s emulation, got nil", foreign)
		}
		if !strings.Contains(err.Error(), foreign) {
			t.Errorf("error should name the missing CPU %q, got: %v", foreign, err)
		}
	})

	t.Run("registered foreign handler passes", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(dir, "qemu-"+foreign), []byte("enabled\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := checkEmulationIn(dir, []string{"linux/" + foreignArch}); err != nil {
			t.Errorf("expected nil once %s is registered, got %v", foreign, err)
		}
	})
}

// archOfCPU maps a QEMU CPU name back to the Go arch name (inverse of qemuCPUName).
func archOfCPU(cpu string) string {
	switch cpu {
	case "aarch64":
		return "arm64"
	case "x86_64":
		return "amd64"
	default:
		return cpu
	}
}
