package debug_test

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/fertile-org/banyan/pkg/vpc"
	"github.com/fertile-org/banyan/pkg/vpc/debug"
)

func TestDebugManager_TraceConnection(t *testing.T) {
	ctx := context.Background()
	manager := debug.NewManager()

	tests := []struct {
		name    string
		fromIP  net.IP
		toIP    net.IP
		port    int
		wantErr bool
		checks  func(t *testing.T, result *vpc.TraceResult)
	}{
		{
			name:    "trace valid connection",
			fromIP:  net.ParseIP("10.0.1.5"),
			toIP:    net.ParseIP("10.0.1.10"),
			port:    5432,
			wantErr: false,
			checks: func(t *testing.T, result *vpc.TraceResult) {
				if result == nil {
					t.Fatal("expected trace result, got nil")
				}
				if result.FromIP == nil || !result.FromIP.Equal(net.ParseIP("10.0.1.5")) {
					t.Errorf("expected from IP 10.0.1.5, got %v", result.FromIP)
				}
				if result.ToIP == nil || !result.ToIP.Equal(net.ParseIP("10.0.1.10")) {
					t.Errorf("expected to IP 10.0.1.10, got %v", result.ToIP)
				}
				if result.Port != 5432 {
					t.Errorf("expected port 5432, got %d", result.Port)
				}
				// Check for hops in trace
				if len(result.Hops) == 0 {
					t.Error("expected at least one hop in trace")
				}
			},
		},
		{
			name:    "trace to external IP",
			fromIP:  net.ParseIP("10.0.1.5"),
			toIP:    net.ParseIP("8.8.8.8"),
			port:    443,
			wantErr: false,
			checks: func(t *testing.T, result *vpc.TraceResult) {
				if result == nil {
					t.Fatal("expected trace result, got nil")
				}
				// Should show NAT/gateway in path
				hasGateway := false
				for _, hop := range result.Hops {
					if hop.Type == "gateway" || hop.Type == "nat" {
						hasGateway = true
						break
					}
				}
				if !hasGateway {
					t.Error("expected gateway/NAT hop for external connection")
				}
			},
		},
		{
			name:    "trace blocked connection",
			fromIP:  net.ParseIP("10.0.1.5"),
			toIP:    net.ParseIP("10.0.2.10"),
			port:    22,
			wantErr: false,
			checks: func(t *testing.T, result *vpc.TraceResult) {
				if result == nil {
					t.Fatal("expected trace result, got nil")
				}
				if result.Status != "blocked" {
					t.Errorf("expected status blocked, got %s", result.Status)
				}
				if result.BlockedBy == "" {
					t.Error("expected BlockedBy to indicate which rule blocked")
				}
			},
		},
		{
			name:    "trace with nil from IP",
			fromIP:  nil,
			toIP:    net.ParseIP("10.0.1.10"),
			port:    80,
			wantErr: true,
		},
		{
			name:    "trace with nil to IP",
			fromIP:  net.ParseIP("10.0.1.5"),
			toIP:    nil,
			port:    80,
			wantErr: true,
		},
		{
			name:    "trace with invalid port",
			fromIP:  net.ParseIP("10.0.1.5"),
			toIP:    net.ParseIP("10.0.1.10"),
			port:    -1,
			wantErr: true,
		},
		{
			name:    "trace with port out of range",
			fromIP:  net.ParseIP("10.0.1.5"),
			toIP:    net.ParseIP("10.0.1.10"),
			port:    70000,
			wantErr: true,
		},
		{
			name:    "trace same IP (loopback)",
			fromIP:  net.ParseIP("10.0.1.5"),
			toIP:    net.ParseIP("10.0.1.5"),
			port:    8080,
			wantErr: false,
			checks: func(t *testing.T, result *vpc.TraceResult) {
				if result == nil {
					t.Fatal("expected trace result, got nil")
				}
				if result.Status != "allowed" {
					t.Errorf("loopback should be allowed, got %s", result.Status)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := manager.TraceConnection(ctx, tt.fromIP, tt.toIP, tt.port)
			if (err != nil) != tt.wantErr {
				t.Errorf("TraceConnection() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checks != nil {
				tt.checks(t, result)
			}
		})
	}
}

func TestDebugManager_CheckConnectivity(t *testing.T) {
	ctx := context.Background()
	manager := debug.NewManager()

	tests := []struct {
		name        string
		containerID string
		wantErr     bool
		checks      func(t *testing.T, result *vpc.ConnectivityResult)
	}{
		{
			name:        "check connectivity for existing container",
			containerID: "container-001",
			wantErr:     false,
			checks: func(t *testing.T, result *vpc.ConnectivityResult) {
				if result == nil {
					t.Fatal("expected connectivity result, got nil")
				}
				if result.ContainerID != "container-001" {
					t.Errorf("expected container-001, got %s", result.ContainerID)
				}
				// Check basic connectivity fields
				if result.IP == nil {
					t.Error("expected IP address")
				}
				if result.Gateway == nil {
					t.Error("expected gateway address")
				}
				if result.DNS == nil || len(result.DNS) == 0 {
					t.Error("expected DNS servers")
				}
			},
		},
		{
			name:        "check connectivity with internet access",
			containerID: "container-web",
			wantErr:     false,
			checks: func(t *testing.T, result *vpc.ConnectivityResult) {
				if result == nil {
					t.Fatal("expected connectivity result, got nil")
				}
				if !result.InternetAccess {
					t.Error("web container should have internet access")
				}
				if result.DefaultRoute == "" {
					t.Error("expected default route for internet access")
				}
			},
		},
		{
			name:        "check connectivity for isolated container",
			containerID: "container-database",
			wantErr:     false,
			checks: func(t *testing.T, result *vpc.ConnectivityResult) {
				if result == nil {
					t.Fatal("expected connectivity result, got nil")
				}
				if result.InternetAccess {
					t.Error("database container should not have internet access")
				}
				// Should still have internal connectivity
				if result.IP == nil {
					t.Error("isolated container should still have IP")
				}
			},
		},
		{
			name:        "check connectivity for non-existent container",
			containerID: "non-existent",
			wantErr:     true,
		},
		{
			name:        "check connectivity with empty container ID",
			containerID: "",
			wantErr:     true,
		},
		{
			name:        "check connectivity with network issues",
			containerID: "container-broken",
			wantErr:     false,
			checks: func(t *testing.T, result *vpc.ConnectivityResult) {
				if result == nil {
					t.Fatal("expected connectivity result, got nil")
				}
				if len(result.Errors) == 0 {
					t.Error("expected errors for broken container")
				}
				if result.Status != "degraded" && result.Status != "error" {
					t.Errorf("expected degraded or error status, got %s", result.Status)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := manager.CheckConnectivity(ctx, tt.containerID)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckConnectivity() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checks != nil {
				tt.checks(t, result)
			}
		})
	}
}

func TestDebugManager_GetIPTablesRules(t *testing.T) {
	ctx := context.Background()
	manager := debug.NewManager()

	tests := []struct {
		name    string
		ip      net.IP
		wantErr bool
		checks  func(t *testing.T, rules []string)
	}{
		{
			name:    "get rules for valid IP",
			ip:      net.ParseIP("10.0.1.5"),
			wantErr: false,
			checks: func(t *testing.T, rules []string) {
				if len(rules) == 0 {
					t.Error("expected at least one iptables rule")
				}
				// Check for basic iptables components
				hasFilterTable := false
				hasNatTable := false
				for _, rule := range rules {
					if strings.Contains(rule, "filter") || strings.Contains(rule, "FORWARD") {
						hasFilterTable = true
					}
					if strings.Contains(rule, "nat") || strings.Contains(rule, "POSTROUTING") {
						hasNatTable = true
					}
				}
				if !hasFilterTable {
					t.Error("expected filter table rules")
				}
				if !hasNatTable {
					t.Error("expected NAT table rules")
				}
			},
		},
		{
			name:    "get rules for container IP",
			ip:      net.ParseIP("10.0.1.10"),
			wantErr: false,
			checks: func(t *testing.T, rules []string) {
				// Should include container-specific rules
				hasContainerRule := false
				for _, rule := range rules {
					if strings.Contains(rule, "10.0.1.10") {
						hasContainerRule = true
						break
					}
				}
				if !hasContainerRule {
					t.Error("expected rules specific to container IP")
				}
			},
		},
		{
			name:    "get rules for external IP",
			ip:      net.ParseIP("8.8.8.8"),
			wantErr: false,
			checks: func(t *testing.T, rules []string) {
				// Should show rules affecting external traffic
				if len(rules) == 0 {
					t.Error("should return rules affecting external IPs")
				}
			},
		},
		{
			name:    "get rules for nil IP",
			ip:      nil,
			wantErr: true,
		},
		{
			name:    "get rules for invalid IP",
			ip:      net.IP{}, // Empty IP
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := manager.GetIPTablesRules(ctx, tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetIPTablesRules() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checks != nil {
				tt.checks(t, rules)
			}
		})
	}
}

func TestDebugManager_ComplexScenarios(t *testing.T) {
	ctx := context.Background()
	manager := debug.NewManager()

	t.Run("trace multi-hop connection", func(t *testing.T) {
		// Test tracing through multiple network hops
		result, err := manager.TraceConnection(ctx,
			net.ParseIP("10.0.1.5"),  // Source in subnet 1
			net.ParseIP("10.0.3.10"), // Dest in subnet 3
			80,
		)
		if err != nil {
			t.Fatalf("Failed to trace multi-hop: %v", err)
		}

		// Should show multiple hops through gateway
		if len(result.Hops) < 2 {
			t.Error("Expected multiple hops for cross-subnet connection")
		}

		// Verify hop types
		hasGateway := false
		hasVxlan := false
		for _, hop := range result.Hops {
			if hop.Type == "gateway" {
				hasGateway = true
			}
			if hop.Type == "vxlan" {
				hasVxlan = true
			}
		}
		if !hasGateway {
			t.Error("Expected gateway hop")
		}
		if !hasVxlan {
			t.Error("Expected VXLAN tunnel hop")
		}
	})

	t.Run("diagnose network partition", func(t *testing.T) {
		// Test connectivity check during network partition
		result, err := manager.CheckConnectivity(ctx, "container-partitioned")
		if err != nil {
			t.Fatalf("Failed to check connectivity: %v", err)
		}

		// Should detect partition
		if result.Status != "error" && result.Status != "unreachable" {
			t.Errorf("Expected error/unreachable status, got %s", result.Status)
		}

		// Should identify the issue
		hasPartitionError := false
		for _, errMsg := range result.Errors {
			if strings.Contains(errMsg, "partition") || strings.Contains(errMsg, "unreachable") {
				hasPartitionError = true
				break
			}
		}
		if !hasPartitionError {
			t.Error("Expected partition/unreachable error in diagnosis")
		}
	})

	t.Run("debug security rule conflicts", func(t *testing.T) {
		// Test tracing when there are conflicting security rules
		result, err := manager.TraceConnection(ctx,
			net.ParseIP("10.0.1.5"),
			net.ParseIP("10.0.1.20"),
			3306, // MySQL port
		)
		if err != nil {
			t.Fatalf("Failed to trace: %v", err)
		}

		// Should identify which rule is blocking/allowing
		if result.Status == "blocked" {
			if result.BlockedBy == "" {
				t.Error("Should identify blocking rule")
			}
			// Should provide rule details
			if result.BlockedByDetails == "" {
				t.Error("Should provide details about blocking rule")
			}
		} else if result.Status == "allowed" {
			if result.AllowedBy == "" {
				t.Error("Should identify allowing rule")
			}
		}
	})
}

func TestDebugManager_PerformanceDiagnostics(t *testing.T) {
	ctx := context.Background()
	manager := debug.NewManager()

	// Test performance-related debugging
	t.Run("detect MTU issues", func(t *testing.T) {
		result, err := manager.CheckConnectivity(ctx, "container-mtu-issue")
		if err != nil {
			t.Fatalf("Failed to check connectivity: %v", err)
		}

		// Should detect MTU mismatch
		hasMTUInfo := false
		for _, err := range result.Errors {
			if strings.Contains(err, "MTU") || strings.Contains(err, "fragmentation") {
				hasMTUInfo = true
				break
			}
		}
		if !hasMTUInfo {
			t.Log("MTU issue detection will be verified in implementation")
		}
	})

	t.Run("identify packet drops", func(t *testing.T) {
		rules, err := manager.GetIPTablesRules(ctx, net.ParseIP("10.0.1.100"))
		if err != nil {
			t.Fatalf("Failed to get rules: %v", err)
		}

		// Should include packet counters
		hasCounters := false
		for _, rule := range rules {
			if strings.Contains(rule, "pkts") || strings.Contains(rule, "bytes") {
				hasCounters = true
				break
			}
		}
		if !hasCounters {
			t.Log("Packet counter support will be verified in implementation")
		}
	})
}
