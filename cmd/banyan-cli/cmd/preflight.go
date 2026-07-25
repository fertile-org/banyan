package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const binfmtDir = "/proc/sys/fs/binfmt_misc"

// qemuCPUName maps a platform string (e.g. "linux/arm64" or "arm64") to the
// QEMU user-static CPU name used in the binfmt_misc handler filename.
func qemuCPUName(platform string) string {
	arch := platform
	if i := strings.LastIndex(platform, "/"); i >= 0 {
		arch = platform[i+1:]
	}
	switch arch {
	case "arm64", "aarch64":
		return "aarch64"
	case "amd64", "x86_64":
		return "x86_64"
	default:
		return arch
	}
}

// binfmtRegistered reports whether an enabled qemu-<cpu> handler exists under dir.
func binfmtRegistered(dir, cpu string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "qemu-"+cpu))
	if err != nil {
		return false
	}
	// The handler file's first line is "enabled" or "disabled".
	return strings.HasPrefix(strings.TrimSpace(string(data)), "enabled")
}

// hostCPU returns the QEMU CPU name for the machine running the CLI.
func hostCPU() string {
	return qemuCPUName(runtime.GOARCH)
}

// checkEmulation verifies that every non-host platform in platforms has a
// registered QEMU binfmt handler. Returns an actionable error if not.
func checkEmulation(platforms []string) error {
	return checkEmulationIn(binfmtDir, platforms)
}

// checkEmulationIn is checkEmulation with an injectable binfmt dir (for tests).
// Each entry may itself be a comma-separated multi-platform string
// (e.g. "linux/amd64,linux/arm64"), so every sub-platform is checked.
func checkEmulationIn(dir string, platforms []string) error {
	host := hostCPU()
	var missing []string
	seen := map[string]bool{}
	for _, entry := range platforms {
		for _, p := range strings.Split(entry, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			cpu := qemuCPUName(p)
			if cpu == host || seen[cpu] {
				continue
			}
			seen[cpu] = true
			if !binfmtRegistered(dir, cpu) {
				missing = append(missing, cpu)
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"cross-arch build requires QEMU emulation for %s, but no binfmt handler is registered.\n"+
			"Install it with either:\n"+
			"  sudo bash install-deps.sh   # installs qemu-user-static + binfmt-support\n"+
			"or:\n"+
			"  sudo nerdctl run --privileged --rm tonistiigi/binfmt --install all",
		strings.Join(missing, ", "))
}
