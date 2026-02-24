package overlay

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// ExecLinkOps implements LinkOperations using exec.Command calls to ip/bridge.
type ExecLinkOps struct{}

func (e *ExecLinkOps) CreateVXLAN(name string, vni int, port int, srcIP net.IP) error {
	return runCmd("ip", "link", "add", name,
		"type", "vxlan",
		"id", fmt.Sprintf("%d", vni),
		"dstport", fmt.Sprintf("%d", port),
		"local", srcIP.String(),
		"nolearning",
	)
}

func (e *ExecLinkOps) CreateBridge(name string) error {
	return runCmd("ip", "link", "add", name, "type", "bridge")
}

func (e *ExecLinkOps) SetLinkMaster(slave, master string) error {
	return runCmd("ip", "link", "set", slave, "master", master)
}

func (e *ExecLinkOps) SetLinkUp(name string) error {
	return runCmd("ip", "link", "set", name, "up")
}

func (e *ExecLinkOps) AddAddress(name string, addr *net.IPNet) error {
	return runCmd("ip", "addr", "add", addr.String(), "dev", name)
}

func (e *ExecLinkOps) AddRoute(dst net.IPNet, gw net.IP, dev string) error {
	return runCmd("ip", "route", "replace", dst.String(), "via", gw.String(), "dev", dev)
}

func (e *ExecLinkOps) AddFDBEntry(mac net.HardwareAddr, ip net.IP, dev string) error {
	return runCmd("bridge", "fdb", "append", mac.String(), "dev", dev, "dst", ip.String())
}

func (e *ExecLinkOps) AddARPEntry(ip net.IP, mac net.HardwareAddr, dev string) error {
	return runCmd("ip", "neigh", "replace", ip.String(), "lladdr", mac.String(), "dev", dev, "nud", "permanent")
}

func (e *ExecLinkOps) DeleteLink(name string) error {
	return runCmd("ip", "link", "del", name)
}

func (e *ExecLinkOps) LinkExists(name string) (bool, error) {
	err := runCmd("ip", "link", "show", name)
	if err != nil {
		// If the error message contains "does not exist", the link doesn't exist
		if strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// runCmd executes a command and returns an error with stderr on failure.
func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...) //nolint:gosec // args are constructed internally
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), errMsg)
		}
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
