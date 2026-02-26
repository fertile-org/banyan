package overlay

import (
	"fmt"
	"os"
	"strings"
)

// ExecWireGuardOps implements WireGuardOps using ip and wg CLI commands.
type ExecWireGuardOps struct{}

func (e *ExecWireGuardOps) CreateInterface(name string) error {
	return runCmd("ip", "link", "add", name, "type", "wireguard")
}

func (e *ExecWireGuardOps) ConfigureInterface(name, privateKeyB64 string, port int) error {
	// Write private key to a temp file (wg set reads from file, not stdin)
	tmpFile, err := os.CreateTemp("", "wg-key-*")
	if err != nil {
		return fmt.Errorf("create temp key file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(privateKeyB64); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write private key to temp file: %w", err)
	}
	tmpFile.Close()

	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("chmod temp key file: %w", err)
	}

	return runCmd("wg", "set", name,
		"private-key", tmpPath,
		"listen-port", fmt.Sprintf("%d", port),
	)
}

func (e *ExecWireGuardOps) AddPeer(iface, pubKey, endpoint string, allowedIPs []string, keepalive int) error {
	args := []string{"set", iface, "peer", pubKey}
	if endpoint != "" {
		args = append(args, "endpoint", endpoint)
	}
	if len(allowedIPs) > 0 {
		args = append(args, "allowed-ips", strings.Join(allowedIPs, ","))
	}
	if keepalive > 0 {
		args = append(args, "persistent-keepalive", fmt.Sprintf("%d", keepalive))
	}
	return runCmd("wg", args...)
}

func (e *ExecWireGuardOps) RemovePeer(iface, pubKey string) error {
	return runCmd("wg", "set", iface, "peer", pubKey, "remove")
}
