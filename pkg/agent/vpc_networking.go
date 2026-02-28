package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fertile-org/banyan/pkg/logging"
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
	reclaimDNSPort = defaultReclaimDNSPort
)

// Paths used by findUDPListenerPID — variables for testability.
var (
	procNetUDP = "/proc/net/udp"
	procDir    = "/proc"
)

const (
	cniBinDir = "/opt/cni/bin"
)

func defaultOverlayDriverFactory(overlayType, wgPrivateKey, wgPublicKey string) overlay.OverlayDriver {
	if overlayType == "wireguard" && wgPrivateKey != "" && wgPublicKey != "" {
		return overlay.NewWireGuardDriver(wgPrivateKey, wgPublicKey)
	}
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
		a.logger().Warn("Failed to enable IP forwarding", "error", writeErr)
	}

	// 5. Create overlay driver and call Init()
	driver := overlayDriverFactory(vpcConfig.OverlayType, a.opts.WGPrivateKey, a.opts.WGPublicKey)
	if initErr := driver.Init(ctx, *subnet, hostIP); initErr != nil {
		return fmt.Errorf("overlay init failed: %w", initErr)
	}

	// 6. Write CNI config via driver
	if writeErr := driver.WriteCNIConfig(*subnet); writeErr != nil {
		return fmt.Errorf("failed to write CNI config: %w", writeErr)
	}

	// Store driver on Agent for later use
	a.overlayDriver = driver
	a.logger().Info("Overlay networking initialized", "subnet", subnet.String(), "host", hostIP.String())

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
		var peer overlay.Peer
		var err error
		if p.PublicKey != "" {
			peer, err = overlay.PeerFromSubnetAndHostWG(p.Subnet, p.HostIP, p.PublicKey)
		} else {
			peer, err = overlay.PeerFromSubnetAndHost(p.Subnet, p.HostIP, p.VTEPMAC)
		}
		if err != nil {
			a.logger().Warn("Skipping invalid peer", "error", err)
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

// defaultReclaimDNSPort finds and kills the process holding a UDP port on a specific IP.
// This handles the case where a previous agent was killed without cleanup (e.g., terminal crash).
func defaultReclaimDNSPort(ip string, port int) error {
	pid, err := findUDPListenerPID(ip, port)
	if err != nil {
		return err
	}
	if pid <= 1 {
		return fmt.Errorf("refusing to kill PID %d", pid)
	}

	logging.Info("Killing stale process holding port", "pid", pid, "ip", ip, "port", port)
	if killErr := syscall.Kill(pid, syscall.SIGKILL); killErr != nil {
		return fmt.Errorf("kill PID %d: %w", pid, killErr)
	}
	time.Sleep(200 * time.Millisecond)
	return nil
}

// findUDPListenerPID finds the PID of the process bound to a specific IP:port on UDP.
// Parses /proc/net/udp for the socket inode, then walks /proc/[pid]/fd/ to find the owner.
func findUDPListenerPID(ip string, port int) (int, error) {
	parsedIP := net.ParseIP(ip).To4()
	if parsedIP == nil {
		return 0, fmt.Errorf("invalid IPv4: %s", ip)
	}

	// /proc/net/udp stores IPs as 32-bit little-endian hex
	hexAddr := fmt.Sprintf("%02X%02X%02X%02X:%04X",
		parsedIP[3], parsedIP[2], parsedIP[1], parsedIP[0], port)

	data, err := os.ReadFile(procNetUDP)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", procNetUDP, err)
	}

	var inode string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 10 && fields[1] == hexAddr {
			inode = fields[9]
			break
		}
	}
	if inode == "" || inode == "0" {
		return 0, fmt.Errorf("no UDP socket found for %s:%d", ip, port)
	}

	// Walk /proc/[pid]/fd/ to find which PID owns the socket inode
	target := "socket:[" + inode + "]"
	entries, readErr := os.ReadDir(procDir)
	if readErr != nil {
		return 0, fmt.Errorf("read %s: %w", procDir, readErr)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, parseErr := strconv.Atoi(entry.Name())
		if parseErr != nil {
			continue
		}
		fdPath := filepath.Join(procDir, entry.Name(), "fd")
		fds, fdErr := os.ReadDir(fdPath)
		if fdErr != nil {
			continue // permission denied for other users' processes
		}
		for _, fd := range fds {
			link, linkErr := os.Readlink(filepath.Join(fdPath, fd.Name()))
			if linkErr != nil {
				continue
			}
			if link == target {
				return pid, nil
			}
		}
	}

	return 0, fmt.Errorf("no process found for socket inode %s", inode)
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
	bindAddr := gatewayIP.String() + ":53"

	manager := dnsManagerFactory()
	server := dnsServerFactory(manager, dns.ServerConfig{
		BindAddr:     bindAddr,
		InternalZone: "internal",
		UpstreamDNS:  "8.8.8.8:53",
	})

	if startErr := server.Start(); startErr != nil {
		// If a stale agent process is holding the port, reclaim and retry
		if strings.Contains(startErr.Error(), "address already in use") {
			a.logger().Info("DNS port in use by stale process, reclaiming", "bind", bindAddr)
			if reclaimErr := reclaimDNSPort(gatewayIP.String(), 53); reclaimErr != nil {
				a.logger().Warn("Could not reclaim DNS port", "error", reclaimErr)
			}
			// Create fresh server and retry
			server = dnsServerFactory(manager, dns.ServerConfig{
				BindAddr:     bindAddr,
				InternalZone: "internal",
				UpstreamDNS:  "8.8.8.8:53",
			})
			if retryErr := server.Start(); retryErr != nil {
				return fmt.Errorf("DNS server failed to start after reclaim: %w", retryErr)
			}
		} else {
			return fmt.Errorf("DNS server failed to start: %w", startErr)
		}
	}

	a.dnsManager = manager
	a.dnsServer = server
	a.gatewayIP = gatewayIP.String()
	a.logger().Info("DNS server started", "bind", bindAddr)

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
