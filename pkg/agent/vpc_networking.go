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

	"github.com/coreos/go-iptables/iptables"
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
	readSysctl     = defaultReadSysctl
	writeSysctl    = defaultWriteSysctl
)

// Paths used by findUDPListenerPID — variables for testability.
var (
	procNetUDP = "/proc/net/udp"
	procDir    = "/proc"
)

const (
	cniBinDir = "/opt/cni/bin"
)

// iptablesHandle is the subset of go-iptables methods used for overlay forwarding
// and deployment network isolation.
type iptablesHandle interface {
	Exists(table, chain string, rulespec ...string) (bool, error)
	Insert(table, chain string, pos int, rulespec ...string) error
	Delete(table, chain string, rulespec ...string) error
	Append(table, chain string, rulespec ...string) error
	NewChain(table, chain string) error
	ClearChain(table, chain string) error
	DeleteChain(table, chain string) error
	ListChains(table string) ([]string, error)
}

// iptablesFactory creates an iptables handle. Extracted as a variable for test mocking.
var iptablesFactory = func() (iptablesHandle, error) {
	return iptables.New()
}

// isolationChainName is the main iptables chain for deployment network isolation.
const isolationChainName = "BANYAN-ISOLATION"

// depChainPrefix is the prefix for per-deployment iptables chains.
const depChainPrefix = "BN-DEP-"

// setupOverlayForwarding creates the BANYAN-ISOLATION chain and adds jump rules
// from the FORWARD chain for cross-host overlay traffic. The actual per-deployment
// isolation rules are populated by reconcileNetworkIsolation().
// Extracted as a variable for test mocking.
var setupOverlayForwarding = defaultSetupOverlayForwarding

func defaultSetupOverlayForwarding() error {
	ipt, err := iptablesFactory()
	if err != nil {
		return fmt.Errorf("failed to init iptables: %w", err)
	}

	// Create the isolation chain (idempotent)
	if chainErr := ipt.NewChain("filter", isolationChainName); chainErr != nil {
		// NewChain returns error if chain already exists — that's fine
		chains, listErr := ipt.ListChains("filter")
		if listErr != nil {
			return fmt.Errorf("failed to list chains: %w", listErr)
		}
		found := false
		for _, c := range chains {
			if c == isolationChainName {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("failed to create chain %s: %w", isolationChainName, chainErr)
		}
	}

	// Add jump rules from FORWARD to BANYAN-ISOLATION for overlay traffic
	jumpRules := [][]string{
		{"-i", "banyan0", "-o", "banyan-wg", "-j", isolationChainName},
		{"-i", "banyan-wg", "-o", "banyan0", "-j", isolationChainName},
	}
	for _, rule := range jumpRules {
		exists, _ := ipt.Exists("filter", "FORWARD", rule...)
		if !exists {
			if insertErr := ipt.Insert("filter", "FORWARD", 1, rule...); insertErr != nil {
				return fmt.Errorf("failed to add FORWARD jump rule: %w", insertErr)
			}
		}
	}
	return nil
}

// depChainName returns the iptables chain name for a deployment.
// Truncates to fit the 28-character iptables chain name limit.
func depChainName(deploymentName string) string {
	maxLen := 28 - len(depChainPrefix) // 21 chars for deployment name
	name := deploymentName
	if len(name) > maxLen {
		name = name[:maxLen]
	}
	return depChainPrefix + name
}

// reconcileNetworkIsolation rebuilds iptables rules for deployment-level
// network isolation. Containers in the same deployment can communicate;
// containers in different deployments are blocked on the overlay network.
// Called from the heartbeat loop with the complete set of service backends.
func (a *Agent) reconcileNetworkIsolation(ctx context.Context, backends []ServiceBackend) {
	ipt, err := iptablesFactory()
	if err != nil {
		a.logger().Warn("Failed to init iptables for network isolation", "error", err)
		return
	}

	// Group container IPs by deployment name
	deploymentIPs := map[string][]string{} // deploymentName → []containerIP
	for _, b := range backends {
		if b.DeploymentName == "" || b.ContainerIP == "" {
			continue
		}
		deploymentIPs[b.DeploymentName] = append(deploymentIPs[b.DeploymentName], b.ContainerIP)
	}

	// 1. Flush the isolation chain
	if flushErr := ipt.ClearChain("filter", isolationChainName); flushErr != nil {
		a.logger().Warn("Failed to flush isolation chain", "error", flushErr)
		return
	}

	// 2. Delete old per-deployment chains
	chains, listErr := ipt.ListChains("filter")
	if listErr != nil {
		a.logger().Warn("Failed to list chains for cleanup", "error", listErr)
		return
	}
	for _, chain := range chains {
		if strings.HasPrefix(chain, depChainPrefix) {
			_ = ipt.ClearChain("filter", chain)
			_ = ipt.DeleteChain("filter", chain)
		}
	}

	// 3. Add conntrack rule for established connections
	if appendErr := ipt.Append("filter", isolationChainName,
		"-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"); appendErr != nil {
		a.logger().Warn("Failed to add conntrack rule", "error", appendErr)
		return
	}

	// 4. Allow DNS to gateway IP
	if a.gatewayIP != "" {
		for _, proto := range []string{"udp", "tcp"} {
			_ = ipt.Append("filter", isolationChainName,
				"-d", a.gatewayIP, "-p", proto, "--dport", "53", "-j", "ACCEPT")
		}
	}

	// 5. Create per-deployment chains and populate
	for depName, ips := range deploymentIPs {
		chainName := depChainName(depName)

		// Create the deployment chain
		if chainErr := ipt.NewChain("filter", chainName); chainErr != nil {
			a.logger().Warn("Failed to create deployment chain", "chain", chainName, "error", chainErr)
			continue
		}

		// Allow traffic to all IPs in the same deployment
		for _, ip := range ips {
			_ = ipt.Append("filter", chainName, "-d", ip, "-j", "ACCEPT")
		}

		// Default deny at end of deployment chain
		_ = ipt.Append("filter", chainName, "-j", "DROP")

		// Add source-based jump rules in the isolation chain
		for _, ip := range ips {
			_ = ipt.Append("filter", isolationChainName, "-s", ip, "-j", chainName)
		}
	}

	// 6. Default deny at end of isolation chain (unknown sources)
	_ = ipt.Append("filter", isolationChainName, "-j", "DROP")

	a.logger().Debug("Network isolation reconciled", "deployments", len(deploymentIPs))
}

// addContainerToIsolation adds a newly created container's IP to the iptables
// isolation rules immediately, without waiting for the next heartbeat cycle.
// If the deployment chain already exists, the IP is appended. Otherwise, a new
// deployment chain is created. The next reconcileNetworkIsolation call will
// rebuild everything cleanly.
func (a *Agent) addContainerToIsolation(ctx context.Context, containerIP, deploymentName string) {
	ipt, err := iptablesFactory()
	if err != nil {
		a.logger().Warn("Failed to init iptables for immediate isolation", "error", err)
		return
	}

	chainName := depChainName(deploymentName)

	// Check if the deployment chain exists
	chains, listErr := ipt.ListChains("filter")
	if listErr != nil {
		a.logger().Warn("Failed to list chains", "error", listErr)
		return
	}

	chainExists := false
	for _, c := range chains {
		if c == chainName {
			chainExists = true
			break
		}
	}

	if chainExists {
		// Insert the allow rule before the final DROP (position matters — use Insert at pos 1)
		_ = ipt.Insert("filter", chainName, 1, "-d", containerIP, "-j", "ACCEPT")
	} else {
		// Create a new deployment chain
		if chainErr := ipt.NewChain("filter", chainName); chainErr != nil {
			a.logger().Warn("Failed to create deployment chain", "chain", chainName, "error", chainErr)
			return
		}
		_ = ipt.Append("filter", chainName, "-d", containerIP, "-j", "ACCEPT")
		_ = ipt.Append("filter", chainName, "-j", "DROP")
	}

	// Add source rule in isolation chain (insert before final DROP)
	_ = ipt.Insert("filter", isolationChainName, 1, "-s", containerIP, "-j", chainName)

	a.logger().Debug("Container added to isolation immediately", "ip", containerIP, "deployment", deploymentName)
}

func defaultOverlayDriverFactory(wgPrivateKey, wgPublicKey string) overlay.OverlayDriver {
	return overlay.NewWireGuardDriver(wgPrivateKey, wgPublicKey)
}

// initializeVPCNetworking sets up the WireGuard overlay network for this agent.
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

	// 4. Ensure IP forwarding is enabled (required for routing between bridge and WireGuard)
	// If init was run with sudo, this is already set via sysctl.d. We check first, then attempt
	// to write if needed (may succeed with CAP_NET_ADMIN), and warn clearly on failure.
	if err := ensureSysctl("/proc/sys/net/ipv4/ip_forward", "1"); err != nil {
		a.logger().Warn("IP forwarding is not enabled — run 'sudo banyan-agent init' first", "error", err)
	}

	// 5. Create overlay driver and call Init()
	driver := overlayDriverFactory(a.opts.WGPrivateKey, a.opts.WGPublicKey)
	if initErr := driver.Init(ctx, *subnet, hostIP); initErr != nil {
		return fmt.Errorf("overlay init failed: %w", initErr)
	}

	// 6. Write CNI config via driver
	if writeErr := driver.WriteCNIConfig(*subnet); writeErr != nil {
		return fmt.Errorf("failed to write CNI config: %w", writeErr)
	}

	// 7. Add iptables FORWARD rules for cross-host overlay traffic
	if fwdErr := setupOverlayForwarding(); fwdErr != nil {
		a.logger().Warn("Failed to set up overlay forwarding rules", "error", fwdErr)
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
		peer, err := overlay.PeerFromProto(p.Subnet, p.HostIP, p.PublicKey)
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

// controlTunnelIfacePrefix is the naming prefix for WireGuard control tunnel
// interfaces. These must be skipped when detecting the host's "real" IP.
const controlTunnelIfacePrefix = "wg-ctl-"

// detectHostIP returns the host's non-loopback IPv4 address, skipping
// WireGuard control tunnel interfaces (wg-ctl-*) whose IPs live in
// 10.200.0.0/16 and must not be advertised as host endpoints.
func detectHostIP() (net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	for _, iface := range ifaces {
		// Skip control tunnel interfaces
		if strings.HasPrefix(iface.Name, controlTunnelIfacePrefix) {
			continue
		}

		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
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

	// Wait briefly for the kernel to release the UDP socket after SIGKILL.
	// Poll /proc/<pid> existence instead of fixed sleep.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return nil // Process is gone
		}
		time.Sleep(50 * time.Millisecond) //nolint:mnd // polling interval for process exit
	}
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
// DNS entries are scoped by deployment: each service registers as
// <service>.<deployment>.internal (fully qualified). The short name
// <service>.internal only resolves within the same deployment to prevent
// cross-deployment service name collisions.
func (a *Agent) reconcileDNS(ctx context.Context, backends []ServiceBackend) {
	if a.dnsManager == nil {
		return
	}

	// Build desired: hostname → set of IPs
	// Register both deployment-scoped and short names
	desired := map[string]map[string]bool{}
	for _, b := range backends {
		if b.ServiceName == "" || b.ContainerIP == "" {
			continue
		}

		// Always register the deployment-scoped name: <service>.<deployment>.internal
		if b.DeploymentName != "" {
			fqdn := b.ServiceName + "." + b.DeploymentName + ".internal"
			if desired[fqdn] == nil {
				desired[fqdn] = map[string]bool{}
			}
			desired[fqdn][b.ContainerIP] = true
		}

		// Register the short name <service>.internal only if there's no conflict
		// (i.e., all backends with this service name belong to the same deployment)
		shortName := b.ServiceName + ".internal"
		if desired[shortName] == nil {
			desired[shortName] = map[string]bool{}
		}
		desired[shortName][b.ContainerIP] = true
	}

	// Check for short name conflicts: if multiple deployments have the same
	// service name, remove the short name entry (only FQDN should resolve)
	shortNameDeployment := map[string]string{} // serviceName → deploymentName
	for _, b := range backends {
		if b.ServiceName == "" || b.DeploymentName == "" {
			continue
		}
		if prev, ok := shortNameDeployment[b.ServiceName]; ok && prev != b.DeploymentName {
			// Conflict: multiple deployments use the same service name
			delete(desired, b.ServiceName+".internal")
		} else {
			shortNameDeployment[b.ServiceName] = b.DeploymentName
		}
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

// defaultReadSysctl reads a sysctl value from /proc/sys.
func defaultReadSysctl(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// defaultWriteSysctl writes a sysctl value to /proc/sys.
func defaultWriteSysctl(path, value string) error {
	return os.WriteFile(path, []byte(value), 0o644) //nolint:gosec // sysctl pseudo-files in /proc/sys require world-readable permissions
}

// ensureSysctl checks that a sysctl value is set correctly.
// If the value is already correct, it does nothing. Otherwise, it attempts
// to write the value (may succeed with CAP_NET_ADMIN). Returns an error
// only if the value is wrong and cannot be corrected.
func ensureSysctl(path, expected string) error {
	current, err := readSysctl(path)
	if err == nil && current == expected {
		return nil // already set correctly (e.g. by 'sudo banyan-agent init')
	}

	// Value is wrong or unreadable — attempt to write
	if writeErr := writeSysctl(path, expected); writeErr != nil {
		return fmt.Errorf("%s is %q, expected %q, and write failed: %w", path, current, expected, writeErr)
	}
	return nil
}
