package dns_test

import (
	"context"
	"net"
	"testing"

	"github.com/fertile/banyan/pkg/vpc/dns"
)

func TestDNSManager_RegisterHost(t *testing.T) {
	ctx := context.Background()
	manager := dns.NewManager()

	tests := []struct {
		name     string
		hostname string
		ip       net.IP
		wantErr  bool
	}{
		{
			name:     "register valid hostname with IP",
			hostname: "web-001.internal",
			ip:       net.ParseIP("10.0.1.5"),
			wantErr:  false,
		},
		{
			name:     "register another hostname",
			hostname: "database-001.internal",
			ip:       net.ParseIP("10.0.1.10"),
			wantErr:  false,
		},
		{
			name:     "register hostname with IPv6",
			hostname: "web-002.internal",
			ip:       net.ParseIP("2001:db8::1"),
			wantErr:  false,
		},
		{
			name:     "register duplicate hostname (should update)",
			hostname: "web-001.internal",
			ip:       net.ParseIP("10.0.1.6"),
			wantErr:  false,
		},
		{
			name:     "register with empty hostname",
			hostname: "",
			ip:       net.ParseIP("10.0.1.7"),
			wantErr:  true,
		},
		{
			name:     "register with nil IP",
			hostname: "web-003.internal",
			ip:       nil,
			wantErr:  true,
		},
		{
			name:     "register with invalid hostname",
			hostname: "invalid hostname with spaces",
			ip:       net.ParseIP("10.0.1.8"),
			wantErr:  true,
		},
		{
			name:     "register with special characters in hostname",
			hostname: "web_001.internal", // Underscore not allowed in DNS
			ip:       net.ParseIP("10.0.1.9"),
			wantErr:  true,
		},
		{
			name:     "register service name without suffix",
			hostname: "web",
			ip:       net.ParseIP("10.0.1.11"),
			wantErr:  false,
		},
		{
			name:     "register FQDN",
			hostname: "web.prod.internal",
			ip:       net.ParseIP("10.0.1.12"),
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.RegisterHost(ctx, tt.hostname, tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterHost() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDNSManager_UnregisterHost(t *testing.T) {
	ctx := context.Background()
	manager := dns.NewManager()

	// First register some hosts
	manager.RegisterHost(ctx, "unregister-test-001.internal", net.ParseIP("10.0.1.20"))
	manager.RegisterHost(ctx, "unregister-test-002.internal", net.ParseIP("10.0.1.21"))

	tests := []struct {
		name     string
		hostname string
		wantErr  bool
	}{
		{
			name:     "unregister existing host",
			hostname: "unregister-test-001.internal",
			wantErr:  false,
		},
		{
			name:     "unregister non-existent host",
			hostname: "non-existent.internal",
			wantErr:  true,
		},
		{
			name:     "unregister with empty hostname",
			hostname: "",
			wantErr:  true,
		},
		{
			name:     "unregister already unregistered host",
			hostname: "unregister-test-001.internal", // Already unregistered above
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.UnregisterHost(ctx, tt.hostname)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnregisterHost() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDNSManager_LookupHost(t *testing.T) {
	ctx := context.Background()
	manager := dns.NewManager()

	// Setup test data
	manager.RegisterHost(ctx, "web.internal", net.ParseIP("10.0.1.30"))
	manager.RegisterHost(ctx, "api.internal", net.ParseIP("10.0.1.31"))

	// Register multiple IPs for same hostname (load balancing scenario)
	manager.RegisterHost(ctx, "cluster.internal", net.ParseIP("10.0.1.40"))
	manager.RegisterHost(ctx, "cluster.internal", net.ParseIP("10.0.1.41"))
	manager.RegisterHost(ctx, "cluster.internal", net.ParseIP("10.0.1.42"))

	tests := []struct {
		name      string
		hostname  string
		wantCount int
		wantErr   bool
		checkIPs  func(t *testing.T, ips []net.IP)
	}{
		{
			name:      "lookup existing single host",
			hostname:  "web.internal",
			wantCount: 1,
			wantErr:   false,
			checkIPs: func(t *testing.T, ips []net.IP) {
				if len(ips) != 1 {
					t.Errorf("expected 1 IP, got %d", len(ips))
					return
				}
				if !ips[0].Equal(net.ParseIP("10.0.1.30")) {
					t.Errorf("expected 10.0.1.30, got %s", ips[0])
				}
			},
		},
		{
			name:      "lookup host with multiple IPs",
			hostname:  "cluster.internal",
			wantCount: 3,
			wantErr:   false,
			checkIPs: func(t *testing.T, ips []net.IP) {
				if len(ips) != 3 {
					t.Errorf("expected 3 IPs, got %d", len(ips))
					return
				}
				// Check that all expected IPs are present
				expectedIPs := map[string]bool{
					"10.0.1.40": false,
					"10.0.1.41": false,
					"10.0.1.42": false,
				}
				for _, ip := range ips {
					expectedIPs[ip.String()] = true
				}
				for ip, found := range expectedIPs {
					if !found {
						t.Errorf("expected IP %s not found", ip)
					}
				}
			},
		},
		{
			name:      "lookup non-existent host",
			hostname:  "non-existent.internal",
			wantCount: 0,
			wantErr:   true,
		},
		{
			name:     "lookup with empty hostname",
			hostname: "",
			wantErr:  true,
		},
		{
			name:      "lookup service name without suffix",
			hostname:  "api",
			wantCount: 0,
			wantErr:   true, // Should require full hostname or auto-append suffix
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ips, err := manager.LookupHost(ctx, tt.hostname)
			if (err != nil) != tt.wantErr {
				t.Errorf("LookupHost() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checkIPs != nil {
				tt.checkIPs(t, ips)
			}
		})
	}
}

func TestDNSManager_UpdateHealth(t *testing.T) {
	ctx := context.Background()
	manager := dns.NewManager()

	// Setup hosts for health checking
	manager.RegisterHost(ctx, "healthy.internal", net.ParseIP("10.0.1.50"))
	manager.RegisterHost(ctx, "unhealthy.internal", net.ParseIP("10.0.1.51"))

	tests := []struct {
		name     string
		hostname string
		healthy  bool
		wantErr  bool
		verify   func(t *testing.T)
	}{
		{
			name:     "mark host as unhealthy",
			hostname: "unhealthy.internal",
			healthy:  false,
			wantErr:  false,
			verify: func(t *testing.T) {
				// Lookup should not return unhealthy hosts
				ips, err := manager.LookupHost(ctx, "unhealthy.internal")
				if err == nil && len(ips) > 0 {
					t.Error("unhealthy host should not be returned in lookup")
				}
			},
		},
		{
			name:     "mark host as healthy",
			hostname: "healthy.internal",
			healthy:  true,
			wantErr:  false,
			verify: func(t *testing.T) {
				// Lookup should return healthy hosts
				ips, err := manager.LookupHost(ctx, "healthy.internal")
				if err != nil || len(ips) == 0 {
					t.Error("healthy host should be returned in lookup")
				}
			},
		},
		{
			name:     "update health for non-existent host",
			hostname: "non-existent.internal",
			healthy:  true,
			wantErr:  true,
		},
		{
			name:     "update health with empty hostname",
			hostname: "",
			healthy:  true,
			wantErr:  true,
		},
		{
			name:     "mark previously unhealthy host as healthy",
			hostname: "unhealthy.internal",
			healthy:  true,
			wantErr:  false,
			verify: func(t *testing.T) {
				// Now it should be returned in lookup
				ips, err := manager.LookupHost(ctx, "unhealthy.internal")
				if err != nil || len(ips) == 0 {
					t.Error("re-enabled host should be returned in lookup")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.UpdateHealth(ctx, tt.hostname, tt.healthy)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateHealth() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.verify != nil {
				tt.verify(t)
			}
		})
	}
}

func TestDNSManager_ServiceDiscovery(t *testing.T) {
	ctx := context.Background()
	manager := dns.NewManager()

	// Scenario: Multiple instances of a service for load balancing
	// Register multiple web service instances
	manager.RegisterHost(ctx, "web-001.prod.internal", net.ParseIP("10.0.1.60"))
	manager.RegisterHost(ctx, "web-002.prod.internal", net.ParseIP("10.0.1.61"))
	manager.RegisterHost(ctx, "web-003.prod.internal", net.ParseIP("10.0.1.62"))

	// Also register them under a service name for discovery
	manager.RegisterHost(ctx, "web.service.internal", net.ParseIP("10.0.1.60"))
	manager.RegisterHost(ctx, "web.service.internal", net.ParseIP("10.0.1.61"))
	manager.RegisterHost(ctx, "web.service.internal", net.ParseIP("10.0.1.62"))

	// Test service discovery
	ips, err := manager.LookupHost(ctx, "web.service.internal")
	if err != nil {
		t.Fatalf("Failed to lookup service: %v", err)
	}

	if len(ips) != 3 {
		t.Errorf("Expected 3 instances for web service, got %d", len(ips))
	}

	// Test health-aware discovery
	// Mark one instance as unhealthy
	manager.UpdateHealth(ctx, "web-002.prod.internal", false)

	// Service lookup should now return only healthy instances
	// This test assumes the implementation links individual hosts to service records
	t.Log("Health-aware service discovery will be verified in implementation")
}

func TestDNSManager_ConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	manager := dns.NewManager()

	// Test concurrent registration and lookup
	done := make(chan bool, 20)

	// Concurrent registrations
	for i := 0; i < 10; i++ {
		go func(index int) {
			hostname := "concurrent-host-" + string(rune('a'+index)) + ".internal"
			ip := net.ParseIP("10.0.2." + string(rune('1'+index)))
			err := manager.RegisterHost(ctx, hostname, ip)
			if err != nil {
				t.Errorf("concurrent registration failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Concurrent lookups (on existing host)
	manager.RegisterHost(ctx, "lookup-test.internal", net.ParseIP("10.0.2.100"))
	for i := 0; i < 10; i++ {
		go func() {
			_, err := manager.LookupHost(ctx, "lookup-test.internal")
			if err != nil {
				t.Errorf("concurrent lookup failed: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all operations
	for i := 0; i < 20; i++ {
		<-done
	}

	t.Log("Concurrent DNS operations test completed")
}

func TestDNSManager_TTL(t *testing.T) {
	// This test would verify TTL handling for DNS records
	// In a real implementation, we'd need to mock time or have a way to set TTL
	t.Skip("TTL test requires time manipulation - implement after review")
}

func TestDNSManager_ReverseLooku(t *testing.T) {
	ctx := context.Background()
	manager := dns.NewManager()

	// Register hosts
	manager.RegisterHost(ctx, "reverse-test.internal", net.ParseIP("10.0.1.70"))

	// This test would verify reverse DNS lookup (IP to hostname)
	// Would be implemented if reverse lookup is needed
	t.Skip("Reverse lookup test - implement if feature is needed")
}
