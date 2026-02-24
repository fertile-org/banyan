package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/fertile-org/banyan/pkg/vpc/dns"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

// Package-level function variables for testing VPC networking.
var (
	prerequisiteChecker  = checkVPCPrerequisites
	overlayDriverFactory = defaultOverlayDriverFactory
	hostIPDetector       = detectHostIP
	dnsManagerFactory    = func() *dns.Manager { return dns.NewManager() }
	dnsServerFactory     = func(m *dns.Manager, c dns.ServerConfig) *dns.Server {
		return dns.NewServer(m, c)
	}
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

	// 4. Enable IP forwarding (required for routing between bridge and VXLAN)
	if writeErr := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0o644); writeErr != nil {
		fmt.Printf("  WARNING: failed to enable IP forwarding: %v\n", writeErr)
	}

	// 5. Create overlay driver and call Init()
	driver := overlayDriverFactory()
	if initErr := driver.Init(ctx, *subnet, hostIP); initErr != nil {
		return fmt.Errorf("overlay init failed: %w", initErr)
	}

	// 6. Write CNI config via driver
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

// initializeDNS creates a DNS manager and server, binding to the bridge gateway IP.
// Called once during Agent.Run() after VPC networking is initialized.
func (a *Agent) initializeDNS(ctx context.Context, allocatedSubnet string) error {
	_, ipnet, err := net.ParseCIDR(allocatedSubnet)
	if err != nil {
		return fmt.Errorf("invalid subnet %q: %w", allocatedSubnet, err)
	}

	// Gateway IP is the first usable IP in the subnet (VTEP IP)
	gatewayIP := overlay.VTEPIP(*ipnet)

	manager := dnsManagerFactory()
	server := dnsServerFactory(manager, dns.ServerConfig{
		BindAddr:     gatewayIP.String() + ":53",
		InternalZone: "internal",
		UpstreamDNS:  "8.8.8.8:53",
	})

	if startErr := server.Start(); startErr != nil {
		return fmt.Errorf("DNS server failed to start: %w", startErr)
	}

	a.dnsManager = manager
	a.dnsServer = server
	a.gatewayIP = gatewayIP.String()
	fmt.Printf("  DNS server started (bind: %s:53)\n", a.gatewayIP)

	return nil
}

// reconcileDNS updates the DNS manager with the current set of service backends.
// It removes stale hostnames and rebuilds all desired entries for clean state.
func (a *Agent) reconcileDNS(ctx context.Context, backends []ServiceBackend) {
	if a.dnsManager == nil {
		return
	}

	// Build desired: hostname → set of IPs
	desired := map[string]map[string]bool{}
	for _, b := range backends {
		if b.ServiceName == "" || b.ContainerIP == "" {
			continue
		}
		hostname := b.ServiceName + ".internal"
		if desired[hostname] == nil {
			desired[hostname] = map[string]bool{}
		}
		desired[hostname][b.ContainerIP] = true
	}

	// Remove stale hostnames (in registeredDNS but not in desired)
	for hostname := range a.registeredDNS {
		if _, ok := desired[hostname]; !ok {
			a.dnsManager.UnregisterHost(ctx, hostname) //nolint:errcheck // best-effort cleanup
		}
	}

	// Rebuild all desired entries (unregister + re-register for clean state)
	for hostname, ips := range desired {
		a.dnsManager.UnregisterHost(ctx, hostname) //nolint:errcheck // may not exist yet
		for ipStr := range ips {
			a.dnsManager.RegisterHost(ctx, hostname, net.ParseIP(ipStr)) //nolint:errcheck // best-effort
		}
	}

	// Track what we registered
	a.registeredDNS = make(map[string]bool, len(desired))
	for hostname := range desired {
		a.registeredDNS[hostname] = true
	}
}
