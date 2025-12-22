package dns_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/vpc/dns"
	mdns "github.com/miekg/dns"
)

func TestServer_StartStop(t *testing.T) {
	manager := dns.NewManager()
	config := dns.ServerConfig{
		BindAddr:     "127.0.0.1:15353", // High port for testing without root
		InternalZone: "internal",
		UpstreamDNS:  "8.8.8.8:53",
	}

	server := dns.NewServer(manager, config)

	// Test start
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	if !server.IsRunning() {
		t.Error("Server should be running after Start()")
	}

	// Test double start (should error)
	if err := server.Start(); err == nil {
		t.Error("Double start should return error")
	}

	// Test stop
	if err := server.Stop(); err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}

	if server.IsRunning() {
		t.Error("Server should not be running after Stop()")
	}

	// Test double stop (should be safe)
	if err := server.Stop(); err != nil {
		t.Errorf("Double stop should not error: %v", err)
	}
}

func TestServer_InternalDomainResolution(t *testing.T) {
	ctx := context.Background()
	manager := dns.NewManager()

	// Register some hosts
	manager.RegisterHost(ctx, "web.internal", net.ParseIP("10.0.1.5"))
	manager.RegisterHost(ctx, "api.internal", net.ParseIP("10.0.2.10"))
	manager.RegisterHost(ctx, "db.internal", net.ParseIP("10.0.3.20"))

	config := dns.ServerConfig{
		BindAddr:     "127.0.0.1:15354",
		InternalZone: "internal",
		UpstreamDNS:  "8.8.8.8:53",
	}

	server := dns.NewServer(manager, config)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Test DNS resolution
	tests := []struct {
		name       string
		hostname   string
		expectedIP string
		shouldFail bool
	}{
		{
			name:       "resolve web.internal",
			hostname:   "web.internal.",
			expectedIP: "10.0.1.5",
			shouldFail: false,
		},
		{
			name:       "resolve api.internal",
			hostname:   "api.internal.",
			expectedIP: "10.0.2.10",
			shouldFail: false,
		},
		{
			name:       "resolve non-existent host",
			hostname:   "nonexistent.internal.",
			shouldFail: true,
		},
	}

	client := &mdns.Client{
		Net:     "udp",
		Timeout: 2 * time.Second,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := new(mdns.Msg)
			msg.SetQuestion(tt.hostname, mdns.TypeA)

			response, _, err := client.Exchange(msg, "127.0.0.1:15354")
			if err != nil {
				t.Fatalf("DNS query failed: %v", err)
			}

			if tt.shouldFail {
				if response.Rcode != mdns.RcodeNameError {
					t.Errorf("Expected NXDOMAIN, got rcode %d", response.Rcode)
				}
				return
			}

			if len(response.Answer) == 0 {
				t.Fatal("Expected at least one answer")
			}

			// Check the IP
			if aRecord, ok := response.Answer[0].(*mdns.A); ok {
				if aRecord.A.String() != tt.expectedIP {
					t.Errorf("Expected IP %s, got %s", tt.expectedIP, aRecord.A.String())
				}
			} else {
				t.Error("Expected A record in response")
			}
		})
	}
}

func TestServer_MultipleIPs(t *testing.T) {
	ctx := context.Background()
	manager := dns.NewManager()

	// Register multiple IPs for load balancing
	manager.RegisterHost(ctx, "cluster.internal", net.ParseIP("10.0.1.10"))
	manager.RegisterHost(ctx, "cluster.internal", net.ParseIP("10.0.1.11"))
	manager.RegisterHost(ctx, "cluster.internal", net.ParseIP("10.0.1.12"))

	config := dns.ServerConfig{
		BindAddr:     "127.0.0.1:15355",
		InternalZone: "internal",
		UpstreamDNS:  "8.8.8.8:53",
	}

	server := dns.NewServer(manager, config)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(50 * time.Millisecond)

	client := &mdns.Client{
		Net:     "udp",
		Timeout: 2 * time.Second,
	}

	msg := new(mdns.Msg)
	msg.SetQuestion("cluster.internal.", mdns.TypeA)

	response, _, err := client.Exchange(msg, "127.0.0.1:15355")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if len(response.Answer) != 3 {
		t.Errorf("Expected 3 A records, got %d", len(response.Answer))
	}

	// Collect returned IPs
	returnedIPs := make(map[string]bool)
	for _, answer := range response.Answer {
		if aRecord, ok := answer.(*mdns.A); ok {
			returnedIPs[aRecord.A.String()] = true
		}
	}

	// Verify all IPs are returned
	expectedIPs := []string{"10.0.1.10", "10.0.1.11", "10.0.1.12"}
	for _, ip := range expectedIPs {
		if !returnedIPs[ip] {
			t.Errorf("Expected IP %s not found in response", ip)
		}
	}
}

func TestServer_HealthAwareResolution(t *testing.T) {
	ctx := context.Background()
	manager := dns.NewManager()

	// Register hosts
	manager.RegisterHost(ctx, "service.internal", net.ParseIP("10.0.1.100"))
	manager.RegisterHost(ctx, "service.internal", net.ParseIP("10.0.1.101"))

	config := dns.ServerConfig{
		BindAddr:     "127.0.0.1:15356",
		InternalZone: "internal",
		UpstreamDNS:  "8.8.8.8:53",
	}

	server := dns.NewServer(manager, config)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(50 * time.Millisecond)

	client := &mdns.Client{
		Net:     "udp",
		Timeout: 2 * time.Second,
	}

	// Initially both should be returned
	msg := new(mdns.Msg)
	msg.SetQuestion("service.internal.", mdns.TypeA)

	response, _, err := client.Exchange(msg, "127.0.0.1:15356")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if len(response.Answer) != 2 {
		t.Errorf("Expected 2 A records initially, got %d", len(response.Answer))
	}

	// Mark all as unhealthy
	manager.UpdateHealth(ctx, "service.internal", false)

	// Now query should return NXDOMAIN (no healthy hosts)
	response, _, err = client.Exchange(msg, "127.0.0.1:15356")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if response.Rcode != mdns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN when all hosts unhealthy, got rcode %d", response.Rcode)
	}

	// Re-enable health
	manager.UpdateHealth(ctx, "service.internal", true)

	// Should work again
	response, _, err = client.Exchange(msg, "127.0.0.1:15356")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if len(response.Answer) != 2 {
		t.Errorf("Expected 2 A records after re-enabling, got %d", len(response.Answer))
	}
}

func TestServer_IPv6Resolution(t *testing.T) {
	ctx := context.Background()
	manager := dns.NewManager()

	// Register an IPv6 address
	manager.RegisterHost(ctx, "ipv6host.internal", net.ParseIP("2001:db8::1"))

	config := dns.ServerConfig{
		BindAddr:     "127.0.0.1:15357",
		InternalZone: "internal",
		UpstreamDNS:  "8.8.8.8:53",
	}

	server := dns.NewServer(manager, config)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(50 * time.Millisecond)

	client := &mdns.Client{
		Net:     "udp",
		Timeout: 2 * time.Second,
	}

	// Query for AAAA record
	msg := new(mdns.Msg)
	msg.SetQuestion("ipv6host.internal.", mdns.TypeAAAA)

	response, _, err := client.Exchange(msg, "127.0.0.1:15357")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if len(response.Answer) == 0 {
		t.Fatal("Expected AAAA record in response")
	}

	if aaaaRecord, ok := response.Answer[0].(*mdns.AAAA); ok {
		if aaaaRecord.AAAA.String() != "2001:db8::1" {
			t.Errorf("Expected IP 2001:db8::1, got %s", aaaaRecord.AAAA.String())
		}
	} else {
		t.Error("Expected AAAA record type")
	}

	// Query for A record should return empty (IPv6 only)
	// Note: This returns success with empty answer, not NXDOMAIN,
	// because the host exists but has no IPv4 addresses
	msg = new(mdns.Msg)
	msg.SetQuestion("ipv6host.internal.", mdns.TypeA)

	response, _, err = client.Exchange(msg, "127.0.0.1:15357")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	// Should return NXDOMAIN since no healthy IPv4 addresses exist
	// (The DNS Manager LookupHost filters by IP type implicitly through addARecords)
	if len(response.Answer) != 0 {
		t.Errorf("Expected no A records for IPv6-only host, got %d", len(response.Answer))
	}
}

func TestServer_ConfigMethods(t *testing.T) {
	manager := dns.NewManager()
	config := dns.ServerConfig{
		BindAddr:     "10.0.0.1:53",
		InternalZone: "myzone",
		UpstreamDNS:  "1.1.1.1",
	}

	server := dns.NewServer(manager, config)

	if server.BindAddress() != "10.0.0.1:53" {
		t.Errorf("Expected bind address 10.0.0.1:53, got %s", server.BindAddress())
	}

	if server.InternalZone() != "myzone" {
		t.Errorf("Expected internal zone myzone, got %s", server.InternalZone())
	}

	if server.UpstreamDNS() != "1.1.1.1:53" {
		t.Errorf("Expected upstream DNS 1.1.1.1:53, got %s", server.UpstreamDNS())
	}
}

func TestServer_DefaultConfig(t *testing.T) {
	config := dns.DefaultServerConfig()

	if config.BindAddr != "0.0.0.0:53" {
		t.Errorf("Expected default bind addr 0.0.0.0:53, got %s", config.BindAddr)
	}

	if config.InternalZone != "internal" {
		t.Errorf("Expected default internal zone 'internal', got %s", config.InternalZone)
	}

	if config.UpstreamDNS != "8.8.8.8:53" {
		t.Errorf("Expected default upstream DNS 8.8.8.8:53, got %s", config.UpstreamDNS)
	}
}
