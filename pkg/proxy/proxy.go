package proxy

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/coreos/go-iptables/iptables"
)

// sysctlWriter and sysctlReader allow tests to override sysctl access.
var (
	sysctlWriter = defaultSysctlWriter
	sysctlReader = defaultSysctlReader
)

// SetSysctlWriter overrides the sysctl writer for testing from external packages.
func SetSysctlWriter(fn func(string, string) error) func() {
	orig := sysctlWriter
	sysctlWriter = fn
	return func() { sysctlWriter = orig }
}

func defaultSysctlWriter(path, value string) error {
	return os.WriteFile(path, []byte(value), 0o644)
}

func defaultSysctlReader(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// ensureSysctl checks that a sysctl value is set correctly.
// If already correct, it does nothing. Otherwise, it attempts to write (may succeed
// with CAP_NET_ADMIN). Returns an error only if the value is wrong and cannot be corrected.
func ensureSysctl(path, expected string) error {
	current, err := sysctlReader(path)
	if err == nil && current == expected {
		return nil // already set correctly (e.g. by 'setup' command)
	}

	// Value is wrong or unreadable — attempt to write
	if writeErr := sysctlWriter(path, expected); writeErr != nil {
		return fmt.Errorf("%s is %q, expected %q, and write failed: %w", path, current, expected, writeErr)
	}
	return nil
}

// Chain name constants for iptables rules.
// Uses "BANYAN-P-" prefix to avoid collision with VPC security chains
// which use "BANYAN-<networkID>" in the filter table.
const (
	chainServices    = "BANYAN-P-SERVICES"
	chainForward     = "BANYAN-P-FWD"
	chainPostrouting = "BANYAN-P-POSTROUTING"
	chainSVCPrefix   = "BANYAN-P-SVC-"
	proxyChainPrefix = "BANYAN-P-"
)

// Proxy manages iptables DNAT rules for forwarding traffic to container backends.
// It uses the same approach as kube-proxy in iptables mode: the agent writes rules
// but never touches actual traffic — the Linux kernel handles packet forwarding.
type Proxy struct {
	ipt   IPTables
	ports map[int]*portState // host port -> state
	mu    sync.Mutex
}

// portState tracks backends for a single host port.
type portState struct {
	hostPort      int
	containerPort int
	chainName     string // e.g. "BANYAN-SVC-80"
	backends      []backendInfo
}

// backendInfo represents a container backend.
type backendInfo struct {
	containerName string
	containerIP   string
}

// New creates a new Proxy using real iptables.
// Requires root/CAP_NET_ADMIN privileges.
func New() (*Proxy, error) {
	ipt, err := iptables.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize iptables: %w", err)
	}
	return NewWithIPTables(ipt)
}

// NewWithIPTables creates a new Proxy with a provided IPTables implementation.
// This is used for testing with mock/noop iptables.
func NewWithIPTables(ipt IPTables) (*Proxy, error) {
	p := &Proxy{
		ipt:   ipt,
		ports: make(map[int]*portState),
	}
	if err := p.init(); err != nil {
		return nil, err
	}
	return p, nil
}

// init sets up the base iptables chain hierarchy and hooks.
func (p *Proxy) init() error {
	// Enable route_localnet so that iptables DNAT from localhost (127.0.0.1)
	// can route to container IPs on bridge interfaces. Without this,
	// "curl localhost:<port>" would be silently dropped by the kernel.
	// This is the same approach used by kube-proxy in iptables mode.
	// If init was run with sudo, these are already set. We check first, attempt write if needed.
	if err := ensureSysctl("/proc/sys/net/ipv4/conf/all/route_localnet", "1"); err != nil {
		return fmt.Errorf("failed to enable route_localnet (run 'sudo banyan-agent init' first): %w", err)
	}

	// Ensure IP forwarding is enabled (usually set by init, but verify)
	if err := ensureSysctl("/proc/sys/net/ipv4/ip_forward", "1"); err != nil {
		return fmt.Errorf("failed to enable ip_forward (run 'sudo banyan-agent init' first): %w", err)
	}

	// Clean up any stale chains from a previous crash
	if err := p.cleanupStaleChains(); err != nil {
		return fmt.Errorf("failed to clean stale chains: %w", err)
	}

	// Create base chains
	for _, chain := range []struct {
		table string
		name  string
	}{
		{"nat", chainServices},
		{"filter", chainForward},
		{"nat", chainPostrouting},
	} {
		if err := p.ipt.NewChain(chain.table, chain.name); err != nil {
			return fmt.Errorf("failed to create chain %s in %s: %w", chain.name, chain.table, err)
		}
	}

	// Hook into built-in chains (idempotent — skip if already exists)
	hooks := []struct {
		table    string
		chain    string
		rulespec []string
		insert   bool // insert at top instead of append (ensures priority over containerd/CNI rules)
	}{
		{"nat", "PREROUTING", []string{"-j", chainServices}, false},
		{"nat", "OUTPUT", []string{"-j", chainServices}, false},
		{"filter", "FORWARD", []string{"-j", chainForward}, true},
		{"nat", "POSTROUTING", []string{"-j", chainPostrouting}, false},
	}
	for _, h := range hooks {
		exists, err := p.ipt.Exists(h.table, h.chain, h.rulespec...)
		if err != nil {
			return fmt.Errorf("failed to check hook %s/%s: %w", h.table, h.chain, err)
		}
		if !exists {
			if h.insert {
				if err := p.ipt.Insert(h.table, h.chain, 1, h.rulespec...); err != nil {
					return fmt.Errorf("failed to insert hook %s/%s: %w", h.table, h.chain, err)
				}
			} else {
				if err := p.ipt.Append(h.table, h.chain, h.rulespec...); err != nil {
					return fmt.Errorf("failed to add hook %s/%s: %w", h.table, h.chain, err)
				}
			}
		}
	}

	// Add MASQUERADE rule for DNAT'd traffic
	masqRule := []string{"-m", "conntrack", "--ctstate", "DNAT", "-j", "MASQUERADE"}
	exists, err := p.ipt.Exists("nat", chainPostrouting, masqRule...)
	if err != nil {
		return fmt.Errorf("failed to check masquerade rule: %w", err)
	}
	if !exists {
		if err := p.ipt.Append("nat", chainPostrouting, masqRule...); err != nil {
			return fmt.Errorf("failed to add masquerade rule: %w", err)
		}
	}

	return nil
}

// cleanupStaleChains removes proxy-specific chains from a previous run.
// Only targets chains with the "BANYAN-P-" prefix to avoid destroying
// VPC security chains which use "BANYAN-<networkID>".
func (p *Proxy) cleanupStaleChains() error {
	// Remove hooks from built-in chains first
	hooks := []struct {
		table    string
		chain    string
		rulespec []string
	}{
		{"nat", "PREROUTING", []string{"-j", chainServices}},
		{"nat", "OUTPUT", []string{"-j", chainServices}},
		{"filter", "FORWARD", []string{"-j", chainForward}},
		{"nat", "POSTROUTING", []string{"-j", chainPostrouting}},
	}
	for _, h := range hooks {
		exists, err := p.ipt.Exists(h.table, h.chain, h.rulespec...)
		if err != nil {
			continue // Chain might not exist
		}
		if exists {
			p.ipt.Delete(h.table, h.chain, h.rulespec...) //nolint:errcheck // best-effort cleanup
		}
	}

	// Find and remove only proxy chains (BANYAN-P-* prefix)
	for _, table := range []string{"nat", "filter"} {
		chains, err := p.ipt.ListChains(table)
		if err != nil {
			continue
		}
		for _, chain := range chains {
			if strings.HasPrefix(chain, proxyChainPrefix) {
				clearAndDeleteChain(p.ipt, table, chain) //nolint:errcheck // best-effort cleanup
			}
		}
	}

	return nil
}

// AddBackend registers a container as a backend for a host port.
// Creates iptables DNAT rules for traffic forwarding.
func (p *Proxy) AddBackend(hostPort, containerPort int, containerName, containerIP string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	ps, exists := p.ports[hostPort]
	if !exists {
		// Create new per-port service chain
		chainName := fmt.Sprintf("%s%d", chainSVCPrefix, hostPort)
		if err := p.ipt.NewChain("nat", chainName); err != nil {
			return fmt.Errorf("failed to create chain %s: %w", chainName, err)
		}

		// Add jump from BANYAN-SERVICES to the per-port chain.
		// --dst-type LOCAL ensures only host-bound traffic is DNAT'd;
		// container-to-container traffic on the bridge is excluded
		// (container IPs are not LOCAL addresses on the host).
		jumpRule := []string{"-m", "addrtype", "--dst-type", "LOCAL", "-p", "tcp", "--dport", fmt.Sprintf("%d", hostPort), "-j", chainName}
		if err := p.ipt.Append("nat", chainServices, jumpRule...); err != nil {
			return fmt.Errorf("failed to add jump to %s: %w", chainName, err)
		}

		ps = &portState{
			hostPort:      hostPort,
			containerPort: containerPort,
			chainName:     chainName,
		}
		p.ports[hostPort] = ps
	}

	// Add backend to tracking
	ps.backends = append(ps.backends, backendInfo{
		containerName: containerName,
		containerIP:   containerIP,
	})

	// Rebuild DNAT rules with updated probabilities
	if err := p.rebuildDNATRules(ps); err != nil {
		return err
	}

	// Add FORWARD accept rule
	fwdRule := []string{"-d", containerIP, "-p", "tcp", "--dport", fmt.Sprintf("%d", containerPort), "-j", "ACCEPT"}
	if err := p.ipt.Append("filter", chainForward, fwdRule...); err != nil {
		return fmt.Errorf("failed to add forward rule: %w", err)
	}

	return nil
}

// RemoveBackend removes a container from all port listeners.
// Cleans up iptables rules and chains when no backends remain.
func (p *Proxy) RemoveBackend(containerName string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for port, ps := range p.ports {
		var remaining []backendInfo
		var removed []backendInfo
		for _, b := range ps.backends {
			if b.containerName != containerName {
				remaining = append(remaining, b)
			} else {
				removed = append(removed, b)
			}
		}

		if len(removed) == 0 {
			continue
		}

		// Remove FORWARD accept rules for removed backends
		for _, b := range removed {
			fwdRule := []string{"-d", b.containerIP, "-p", "tcp", "--dport", fmt.Sprintf("%d", ps.containerPort), "-j", "ACCEPT"}
			p.ipt.Delete("filter", chainForward, fwdRule...) //nolint:errcheck // best-effort cleanup
		}

		if len(remaining) == 0 {
			// No backends left — remove the service chain
			jumpRule := []string{"-m", "addrtype", "--dst-type", "LOCAL", "-p", "tcp", "--dport", fmt.Sprintf("%d", ps.hostPort), "-j", ps.chainName}
			p.ipt.Delete("nat", chainServices, jumpRule...) //nolint:errcheck // best-effort cleanup
			clearAndDeleteChain(p.ipt, "nat", ps.chainName) //nolint:errcheck // best-effort cleanup
			delete(p.ports, port)
		} else {
			ps.backends = remaining
			if err := p.rebuildDNATRules(ps); err != nil {
				return err
			}
		}
	}

	return nil
}

// rebuildDNATRules flushes and rebuilds DNAT rules in a service chain
// with probability-based load balancing.
func (p *Proxy) rebuildDNATRules(ps *portState) error {
	// Flush existing rules
	if err := p.ipt.ClearChain("nat", ps.chainName); err != nil {
		return fmt.Errorf("failed to flush chain %s: %w", ps.chainName, err)
	}

	n := len(ps.backends)
	for i, b := range ps.backends {
		dest := fmt.Sprintf("%s:%d", b.containerIP, ps.containerPort)

		if i == n-1 {
			// Last backend — no probability match needed, gets all remaining traffic
			rule := []string{"-p", "tcp", "-j", "DNAT", "--to-destination", dest}
			if err := p.ipt.Append("nat", ps.chainName, rule...); err != nil {
				return fmt.Errorf("failed to add DNAT rule: %w", err)
			}
		} else {
			// probability = 1.0 / (N - i)
			prob := 1.0 / float64(n-i)
			rule := []string{
				"-p", "tcp",
				"-m", "statistic", "--mode", "random", "--probability", fmt.Sprintf("%.5f", prob),
				"-j", "DNAT", "--to-destination", dest,
			}
			if err := p.ipt.Append("nat", ps.chainName, rule...); err != nil {
				return fmt.Errorf("failed to add DNAT rule with probability: %w", err)
			}
		}
	}

	return nil
}

// Close removes all BANYAN-* iptables chains and hooks.
func (p *Proxy) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Remove hooks from built-in chains
	hooks := []struct {
		table    string
		chain    string
		rulespec []string
	}{
		{"nat", "PREROUTING", []string{"-j", chainServices}},
		{"nat", "OUTPUT", []string{"-j", chainServices}},
		{"filter", "FORWARD", []string{"-j", chainForward}},
		{"nat", "POSTROUTING", []string{"-j", chainPostrouting}},
	}
	for _, h := range hooks {
		p.ipt.Delete(h.table, h.chain, h.rulespec...) //nolint:errcheck // best-effort cleanup
	}

	// Delete all per-port service chains
	for _, ps := range p.ports {
		clearAndDeleteChain(p.ipt, "nat", ps.chainName) //nolint:errcheck // best-effort cleanup
	}

	// Delete base chains
	for _, chain := range []struct {
		table string
		name  string
	}{
		{"nat", chainServices},
		{"filter", chainForward},
		{"nat", chainPostrouting},
	} {
		clearAndDeleteChain(p.ipt, chain.table, chain.name) //nolint:errcheck // best-effort cleanup
	}

	p.ports = make(map[int]*portState)
	return nil
}

// ListenerCount returns the number of host ports with active backends.
func (p *Proxy) ListenerCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.ports)
}

// BackendCount returns the number of backends for a given host port.
func (p *Proxy) BackendCount(hostPort int) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	ps, exists := p.ports[hostPort]
	if !exists {
		return 0
	}
	return len(ps.backends)
}
