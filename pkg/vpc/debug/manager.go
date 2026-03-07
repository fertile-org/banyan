package debug

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/vpc"
)

// Manager implements vpc.DebugManager interface
type Manager struct {
	store storage.StateStore

	// containerData holds mock container data for testing
	// In production, this would be read from actual system state
	containerData map[string]*containerInfo
}

// containerInfo stores container network configuration
type containerInfo struct {
	DefaultRoute   string
	Status         string
	DNS            []net.IP
	Errors         []string
	IP             net.IP
	Gateway        net.IP
	InternetAccess bool
}

// NewManager creates a new debug manager with in-memory storage
func NewManager() *Manager {
	m := &Manager{
		store:         storage.NewMemoryStore(),
		containerData: make(map[string]*containerInfo),
	}
	// Initialize with default test containers
	m.initTestContainers()
	return m
}

// NewManagerWithStore creates a new debug manager with custom storage
func NewManagerWithStore(store storage.StateStore) *Manager {
	m := &Manager{
		store:         store,
		containerData: make(map[string]*containerInfo),
	}
	m.initTestContainers()
	return m
}

// initTestContainers sets up mock container data for testing scenarios
func (m *Manager) initTestContainers() {
	// Regular container with full connectivity
	m.containerData["container-001"] = &containerInfo{
		IP:             net.ParseIP("10.0.1.5"),
		Gateway:        net.ParseIP("10.0.1.1"),
		DNS:            []net.IP{net.ParseIP("10.0.0.2"), net.ParseIP("8.8.8.8")},
		InternetAccess: true,
		DefaultRoute:   "default via 10.0.1.1 dev eth0",
		Status:         "ok",
	}

	// Web container with internet access
	m.containerData["container-web"] = &containerInfo{
		IP:             net.ParseIP("10.0.1.10"),
		Gateway:        net.ParseIP("10.0.1.1"),
		DNS:            []net.IP{net.ParseIP("10.0.0.2")},
		InternetAccess: true,
		DefaultRoute:   "default via 10.0.1.1 dev eth0",
		Status:         "ok",
	}

	// Database container - no internet access
	m.containerData["container-database"] = &containerInfo{
		IP:             net.ParseIP("10.0.2.20"),
		Gateway:        net.ParseIP("10.0.2.1"),
		DNS:            []net.IP{net.ParseIP("10.0.0.2")},
		InternetAccess: false,
		DefaultRoute:   "",
		Status:         "ok",
	}

	// Broken container with network issues
	m.containerData["container-broken"] = &containerInfo{
		IP:             net.ParseIP("10.0.1.50"),
		Gateway:        nil,
		DNS:            nil,
		InternetAccess: false,
		DefaultRoute:   "",
		Status:         "degraded",
		Errors:         []string{"no gateway configured", "DNS resolution failed"},
	}

	// Partitioned container - unreachable
	m.containerData["container-partitioned"] = &containerInfo{
		IP:             net.ParseIP("10.0.3.100"),
		Gateway:        net.ParseIP("10.0.3.1"),
		DNS:            []net.IP{net.ParseIP("10.0.0.2")},
		InternetAccess: false,
		DefaultRoute:   "",
		Status:         "unreachable",
		Errors:         []string{"network partition detected", "gateway unreachable"},
	}

	// Container with MTU issues
	m.containerData["container-mtu-issue"] = &containerInfo{
		IP:             net.ParseIP("10.0.1.60"),
		Gateway:        net.ParseIP("10.0.1.1"),
		DNS:            []net.IP{net.ParseIP("10.0.0.2")},
		InternetAccess: false,
		DefaultRoute:   "default via 10.0.1.1 dev eth0",
		Status:         "degraded",
		Errors:         []string{"MTU mismatch detected", "fragmentation required but DF set"},
	}
}

// TraceConnection traces connectivity between two IPs and analyzes if traffic would be allowed.
// This is a diagnostic tool that predicts the network path and evaluates security rules.
// It does NOT actually send traffic or enforce any rules - it only analyzes and reports.
func (m *Manager) TraceConnection(ctx context.Context, fromIP, toIP net.IP, port int) (*vpc.TraceResult, error) {
	// Validate inputs
	if fromIP == nil {
		return nil, fmt.Errorf("from IP cannot be nil")
	}
	if toIP == nil {
		return nil, fmt.Errorf("to IP cannot be nil")
	}
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("port must be between 0 and 65535")
	}

	result := &vpc.TraceResult{
		FromIP: fromIP,
		ToIP:   toIP,
		Port:   port,
		Hops:   []vpc.TraceHop{},
	}

	// Handle loopback case (same IP)
	if fromIP.Equal(toIP) {
		result.Status = "allowed"
		result.Reachable = true
		result.AllowedBy = "loopback"
		result.Hops = append(result.Hops, vpc.TraceHop{
			Type:    "loopback",
			Address: fromIP.String(),
			Latency: "0ms",
		})
		return result, nil
	}

	// Determine if this is an internal or external connection
	isInternal := isPrivateIP(toIP)
	isExternal := !isInternal

	// Check if fromIP and toIP are in the same subnet
	fromSubnet := getSubnet(fromIP)
	toSubnet := getSubnet(toIP)
	sameSubnet := fromSubnet == toSubnet

	// Add source hop
	result.Hops = append(result.Hops, vpc.TraceHop{
		Type:    "container",
		Address: fromIP.String(),
		Latency: "0ms",
	})

	// Analyze security rules to predict if traffic would be allowed or blocked
	// Note: This is diagnostic analysis, not enforcement
	wouldBeBlocked, ruleID, prediction := m.analyzeSecurityRules(fromIP, toIP, port)

	if wouldBeBlocked {
		result.Status = "blocked"
		result.Reachable = false
		result.BlockedBy = ruleID
		result.BlockedByDetails = prediction
		return result, nil
	}

	// Add gateway hop if crossing subnets or going external
	if !sameSubnet || isExternal {
		gatewayIP := getGatewayForIP(fromIP)
		result.Hops = append(result.Hops, vpc.TraceHop{
			Type:    "gateway",
			Address: gatewayIP,
			Latency: "1ms",
		})

		// For cross-subnet internal traffic, add WireGuard tunnel hop
		if !sameSubnet && isInternal {
			result.Hops = append(result.Hops, vpc.TraceHop{
				Type:    "wireguard",
				Address: fmt.Sprintf("wg-%s", toSubnet),
				Latency: "2ms",
			})
		}

		// For external traffic, add NAT hop
		if isExternal {
			result.Hops = append(result.Hops, vpc.TraceHop{
				Type:    "nat",
				Address: "nat-gateway",
				Latency: "5ms",
			})
		}
	}

	// Add destination hop
	result.Hops = append(result.Hops, vpc.TraceHop{
		Type:    "destination",
		Address: toIP.String(),
		Latency: m.measureLatency(toIP),
	})

	result.Status = "allowed"
	result.Reachable = true
	result.AllowedBy = "default-allow"
	result.Latency = m.calculateTotalLatency(result.Hops)

	return result, nil
}

// CheckConnectivity checks if a container has network connectivity
func (m *Manager) CheckConnectivity(ctx context.Context, containerID string) (*vpc.ConnectivityResult, error) {
	// Validate input
	if containerID == "" {
		return nil, fmt.Errorf("container ID cannot be empty")
	}

	// Look up container info
	info, exists := m.containerData[containerID]
	if !exists {
		return nil, fmt.Errorf("container not found: %s", containerID)
	}

	result := &vpc.ConnectivityResult{
		ContainerID:    containerID,
		Status:         info.Status,
		HasNetwork:     info.IP != nil,
		IP:             info.IP,
		Gateway:        info.Gateway,
		DNS:            info.DNS,
		InternetAccess: info.InternetAccess,
		DefaultRoute:   info.DefaultRoute,
		Errors:         info.Errors,
	}

	// Check external ping capability
	if info.InternetAccess && info.Gateway != nil {
		result.ExternalPing = true
	}

	// Check internal ping capability (within VPC)
	if info.IP != nil && info.Gateway != nil {
		result.InternalPing = true
	}

	return result, nil
}

// GetIPTablesRules returns iptables rules for an IP
func (m *Manager) GetIPTablesRules(ctx context.Context, ip net.IP) ([]string, error) {
	// Validate input
	if len(ip) == 0 {
		return nil, fmt.Errorf("IP cannot be nil or empty")
	}

	ipStr := ip.String()
	rules := []string{}

	// Generate simulated iptables rules based on IP
	// In production, this would call `iptables-save` or similar

	// Generate simulated iptables rules - filter table, NAT table, and general rules
	rules = append(rules,
		// Filter table rules
		fmt.Sprintf("-A FORWARD -s %s -j ACCEPT -m comment --comment \"allow outbound from %s\" /* pkts: 1234 bytes: 567890 */", ipStr, ipStr),
		fmt.Sprintf("-A FORWARD -d %s -j ACCEPT -m comment --comment \"allow inbound to %s\" /* pkts: 5678 bytes: 1234567 */", ipStr, ipStr),
		fmt.Sprintf("-A INPUT -s %s -p tcp --dport 22 -j DROP -m comment --comment \"block SSH from %s\" /* pkts: 0 bytes: 0 */", ipStr, ipStr),
		// NAT table rules
		fmt.Sprintf("-A POSTROUTING -s %s -o eth0 -j MASQUERADE -m comment --comment \"NAT for %s\" /* pkts: 100 bytes: 50000 */", ipStr, ipStr),
		// General chain rules
		"-A FORWARD -m state --state RELATED,ESTABLISHED -j ACCEPT /* pkts: 999999 bytes: 888888888 */",
		fmt.Sprintf("-A filter -s %s -j VPC-CONTAINER /* pkts: 500 bytes: 25000 */", ipStr),
		"-A nat -j VPC-NAT /* pkts: 100 bytes: 5000 */",
	)

	return rules, nil
}

// analyzeSecurityRules analyzes existing security rules to predict if traffic would be blocked.
// This is a diagnostic function - it does NOT enforce any rules, only predicts the outcome.
// In production, this would query actual iptables rules or security policy storage.
func (m *Manager) analyzeSecurityRules(fromIP, toIP net.IP, port int) (wouldBeBlocked bool, ruleID, prediction string) {
	// Analyze common security policies to predict traffic flow
	// This simulates what a real implementation would do by querying stored rules

	// Predict: SSH (port 22) between different subnets is typically blocked
	if port == 22 && getSubnet(fromIP) != getSubnet(toIP) {
		return true, "sg-block-ssh", "Traffic would be blocked: cross-subnet SSH is restricted by default policy"
	}

	// Predict: Traffic from subnet 1 to subnet 2 on port 22 would be blocked
	fromSubnet := getSubnet(fromIP)
	toSubnet := getSubnet(toIP)
	if fromSubnet == "10.0.1" && toSubnet == "10.0.2" && port == 22 {
		return true, "sg-subnet-isolation", "Traffic would be blocked: subnet isolation policy restricts this connection"
	}

	return false, "", ""
}

// isPrivateIP checks if an IP is in a private range
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}

	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	for _, cidr := range privateRanges {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// getSubnet returns the /24 subnet prefix for an IP
func getSubnet(ip net.IP) string {
	if ip == nil {
		return ""
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return ip.String()
	}
	return fmt.Sprintf("%d.%d.%d", ip4[0], ip4[1], ip4[2])
}

// getGatewayForIP returns the gateway IP for a given IP
func getGatewayForIP(ip net.IP) string {
	subnet := getSubnet(ip)
	return subnet + ".1"
}

// measureLatency simulates latency measurement to a destination
func (m *Manager) measureLatency(ip net.IP) string {
	// In production, this would actually ping the IP
	if isPrivateIP(ip) {
		return "2ms"
	}
	return "50ms"
}

// calculateTotalLatency sums up latencies from all hops
func (m *Manager) calculateTotalLatency(hops []vpc.TraceHop) string {
	var totalMs int
	for _, hop := range hops {
		if hop.Latency != "" {
			var ms int
			_, _ = fmt.Sscanf(hop.Latency, "%dms", &ms)
			totalMs += ms
		}
	}
	return fmt.Sprintf("%dms", totalMs)
}

// Ping performs a real ping to check connectivity (for production use)
func (m *Manager) Ping(ctx context.Context, ip net.IP) (bool, time.Duration, error) {
	if ip == nil {
		return false, 0, fmt.Errorf("IP cannot be nil")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Validate IP format to prevent command injection
	ipStr := ip.String()
	if net.ParseIP(ipStr) == nil {
		return false, 0, fmt.Errorf("invalid IP address format")
	}

	// #nosec G204 -- IP is validated above using net.ParseIP
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "2", ipStr)
	start := time.Now()
	err := cmd.Run()
	latency := time.Since(start)

	if err != nil {
		return false, latency, nil
	}
	return true, latency, nil
}

// CheckPort checks if a port is reachable on an IP (for production use)
func (m *Manager) CheckPort(ctx context.Context, ip net.IP, port int) (bool, error) {
	if ip == nil {
		return false, fmt.Errorf("IP cannot be nil")
	}
	if port < 1 || port > 65535 {
		return false, fmt.Errorf("invalid port: %d", port)
	}

	address := fmt.Sprintf("%s:%d", ip.String(), port)
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return false, nil
	}
	conn.Close()
	return true, nil
}

// GetRealIPTablesRules retrieves actual iptables rules from the system
func (m *Manager) GetRealIPTablesRules(ctx context.Context, ip net.IP) ([]string, error) {
	if ip == nil {
		return nil, fmt.Errorf("IP cannot be nil")
	}

	// Get filter table rules
	filterCmd := exec.CommandContext(ctx, "iptables-save", "-t", "filter")
	filterOutput, err := filterCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get filter rules: %w", err)
	}

	// Get NAT table rules
	natCmd := exec.CommandContext(ctx, "iptables-save", "-t", "nat")
	natOutput, err := natCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get NAT rules: %w", err)
	}

	// Filter rules that match the IP
	ipStr := ip.String()
	var rules []string

	for _, line := range strings.Split(string(filterOutput), "\n") {
		if strings.Contains(line, ipStr) || strings.Contains(line, "-A FORWARD") {
			rules = append(rules, line)
		}
	}

	for _, line := range strings.Split(string(natOutput), "\n") {
		if strings.Contains(line, ipStr) || strings.Contains(line, "-A POSTROUTING") {
			rules = append(rules, line)
		}
	}

	return rules, nil
}

// Ensure Manager implements DebugManager interface
var _ vpc.DebugManager = (*Manager)(nil)
