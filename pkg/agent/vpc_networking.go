package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

// Package-level function variables for testing VPC networking.
var (
	prerequisiteChecker    = checkVPCPrerequisites
	overlayDriverFactory   = defaultOverlayDriverFactory
	hostIPDetector         = detectHostIP
)

const (
	cniBinDir = "/opt/cni/bin"
)

func defaultOverlayDriverFactory() overlay.OverlayDriver {
	return overlay.NewVXLANDriver()
}

// initializeVPCNetworking sets up the VXLAN overlay network for this agent.
// Called once during Agent.Run() after registration.
func (a *Agent) initializeVPCNetworking(ctx context.Context, vpcConfig *VPCConfig) error {
	// 1. Check prerequisites (CNI plugins — no more flanneld check)
	if err := prerequisiteChecker(); err != nil {
		return fmt.Errorf("VPC prerequisites not met: %w", err)
	}

	// 2. Parse allocated subnet
	_, subnet, err := net.ParseCIDR(vpcConfig.AllocatedSubnet)
	if err != nil {
		return fmt.Errorf("invalid allocated subnet %q: %w", vpcConfig.AllocatedSubnet, err)
	}

	// 3. Detect host IP
	hostIP, err := hostIPDetector()
	if err != nil {
		return fmt.Errorf("failed to detect host IP: %w", err)
	}

	// 4. Create overlay driver and call Init()
	driver := overlayDriverFactory()
	if initErr := driver.Init(ctx, *subnet, hostIP); initErr != nil {
		return fmt.Errorf("overlay init failed: %w", initErr)
	}

	// 5. Write CNI config via driver
	if writeErr := driver.WriteCNIConfig(*subnet); writeErr != nil {
		return fmt.Errorf("failed to write CNI config: %w", writeErr)
	}

	// Store driver on Agent for later use
	a.overlayDriver = driver
	fmt.Printf("  Overlay networking initialized (subnet: %s, host: %s)\n", subnet.String(), hostIP.String())

	return nil
}

// reconcileVPCPeers converts VPCPeer from the heartbeat response to overlay.Peer
// and updates the overlay driver's forwarding/routing tables.
func (a *Agent) reconcileVPCPeers(ctx context.Context, peers []VPCPeer) error {
	if a.overlayDriver == nil {
		return nil
	}

	overlayPeers := make([]overlay.Peer, 0, len(peers))
	for _, p := range peers {
		peer, err := overlay.PeerFromSubnetAndHost(p.Subnet, p.HostIP, p.VTEPMAC)
		if err != nil {
			fmt.Printf("[Agent] WARNING: skipping invalid peer: %v\n", err)
			continue
		}
		overlayPeers = append(overlayPeers, peer)
	}

	if len(overlayPeers) == 0 {
		return nil
	}

	return a.overlayDriver.ReconcilePeers(ctx, overlayPeers)
}

// checkVPCPrerequisites verifies required CNI plugins exist.
// No longer checks for flanneld since we use built-in VXLAN.
func checkVPCPrerequisites() error {
	requiredPlugins := []string{"bridge", "host-local", "loopback", "portmap"}
	for _, plugin := range requiredPlugins {
		pluginPath := filepath.Join(cniBinDir, plugin)
		if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
			return fmt.Errorf("CNI plugin %q not found at %s: install CNI plugins (https://github.com/containernetworking/plugins)", plugin, pluginPath)
		}
	}

	return nil
}

// detectHostIP returns the host's non-loopback IPv4 address.
func detectHostIP() (net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, fmt.Errorf("failed to get interface addresses: %w", err)
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		if ip.IsLoopback() || ip.To4() == nil {
			continue
		}
		return ip, nil
	}
	return nil, fmt.Errorf("no non-loopback IPv4 address found")
}
