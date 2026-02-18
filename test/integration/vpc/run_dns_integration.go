//go:build ignore

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	mdns "github.com/miekg/dns"

	"github.com/fertile-org/banyan/pkg/vpc/dns"
	"github.com/fertile-org/banyan/test/integration/helpers"
)

const (
	vpcCLIPath    = "/tmp/banyan-cli"
	dnsServerPort = 15359 // High port for testing without root
	dnsBindAddr   = "127.0.0.1:15359"
	internalZone  = "internal"
	upstreamDNS   = "8.8.8.8:53"
)

func main() {
	ctx := context.Background()
	p := helpers.NewPrinter()

	// Run test
	exitCode := runTest(ctx, p)

	if exitCode == 0 {
		p.Result(true, "DNS Integration Test completed successfully!")
	} else {
		p.Result(false, fmt.Sprintf("DNS Integration Test failed with exit code %d", exitCode))
	}

	os.Exit(exitCode)
}

func runTest(ctx context.Context, p *helpers.Printer) int {
	p.Title("Testing DNS Service Discovery Integration")

	// Step 1: Build banyan-cli
	p.Step("Building banyan-cli")

	projectRoot := getProjectRoot()
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", vpcCLIPath, "./cmd/banyan-cli/")
	buildCmd.Dir = projectRoot

	if output, err := buildCmd.CombinedOutput(); err != nil {
		p.Error(fmt.Sprintf("Failed to build banyan-cli: %v", err))
		p.Code(string(output))
		return 1
	}
	p.Success("Built banyan-cli successfully")

	// Step 2: Verify DNS server command exists
	p.Step("Verifying DNS CLI commands")

	helpCmd := exec.CommandContext(ctx, vpcCLIPath, "dns", "--help")
	if output, err := helpCmd.CombinedOutput(); err != nil {
		p.Error(fmt.Sprintf("DNS command not available: %v", err))
		p.Code(string(output))
		return 1
	}
	p.Success("DNS CLI commands available")

	// Step 3: Create DNS Manager and Server programmatically for testing
	// (CLI server command blocks, so we test programmatically here)
	p.Step("Starting DNS server programmatically")

	manager := dns.NewManager()
	config := dns.ServerConfig{
		BindAddr:     dnsBindAddr,
		InternalZone: internalZone,
		UpstreamDNS:  upstreamDNS,
	}

	server := dns.NewServer(manager, config)
	if err := server.Start(); err != nil {
		p.Error(fmt.Sprintf("Failed to start DNS server: %v", err))
		return 1
	}
	defer func() {
		p.Info("Stopping DNS server...")
		server.Stop()
	}()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	if !server.IsRunning() {
		p.Error("DNS server is not running")
		return 1
	}
	p.Success(fmt.Sprintf("DNS server started on %s", server.BindAddress()))
	p.Info(fmt.Sprintf("Internal zone: %s", server.InternalZone()))
	p.Info(fmt.Sprintf("Upstream DNS: %s", server.UpstreamDNS()))

	// Step 4: Register hosts using the DNS Manager
	p.Step("Registering hosts for service discovery")

	testHosts := []struct {
		hostname string
		ip       string
	}{
		{"web.internal", "10.0.1.10"},
		{"api.internal", "10.0.2.20"},
		{"db.internal", "10.0.3.30"},
		{"cache.internal", "10.0.4.40"},
	}

	for _, host := range testHosts {
		if err := manager.RegisterHost(ctx, host.hostname, net.ParseIP(host.ip)); err != nil {
			p.Error(fmt.Sprintf("Failed to register %s: %v", host.hostname, err))
			return 1
		}
		p.Success(fmt.Sprintf("Registered %s -> %s", host.hostname, host.ip))
	}

	// Step 5: Register multiple IPs for load balancing
	p.Step("Registering multiple IPs for load balancing (cluster.internal)")

	clusterIPs := []string{"10.0.5.10", "10.0.5.11", "10.0.5.12"}
	for _, ip := range clusterIPs {
		if err := manager.RegisterHost(ctx, "cluster.internal", net.ParseIP(ip)); err != nil {
			p.Error(fmt.Sprintf("Failed to register cluster IP %s: %v", ip, err))
			return 1
		}
	}
	p.Success(fmt.Sprintf("Registered cluster.internal with %d IPs", len(clusterIPs)))

	// Step 6: Test DNS resolution using DNS client
	p.Step("Testing DNS resolution via DNS protocol")

	client := &mdns.Client{
		Net:     "udp",
		Timeout: 5 * time.Second,
	}

	for _, host := range testHosts {
		msg := new(mdns.Msg)
		msg.SetQuestion(host.hostname+".", mdns.TypeA)

		response, _, err := client.Exchange(msg, dnsBindAddr)
		if err != nil {
			p.Error(fmt.Sprintf("DNS query for %s failed: %v", host.hostname, err))
			return 1
		}

		if len(response.Answer) == 0 {
			p.Error(fmt.Sprintf("No answer for %s", host.hostname))
			return 1
		}

		aRecord, ok := response.Answer[0].(*mdns.A)
		if !ok {
			p.Error(fmt.Sprintf("Expected A record for %s", host.hostname))
			return 1
		}

		if aRecord.A.String() != host.ip {
			p.Error(fmt.Sprintf("Expected %s for %s, got %s", host.ip, host.hostname, aRecord.A.String()))
			return 1
		}

		p.Success(fmt.Sprintf("DNS lookup %s -> %s (TTL: %ds)", host.hostname, aRecord.A.String(), aRecord.Hdr.Ttl))
	}

	// Step 7: Test cluster resolution (multiple A records)
	p.Step("Testing cluster DNS resolution (multiple A records)")

	msg := new(mdns.Msg)
	msg.SetQuestion("cluster.internal.", mdns.TypeA)

	response, _, err := client.Exchange(msg, dnsBindAddr)
	if err != nil {
		p.Error(fmt.Sprintf("DNS query for cluster.internal failed: %v", err))
		return 1
	}

	if len(response.Answer) != 3 {
		p.Error(fmt.Sprintf("Expected 3 A records for cluster.internal, got %d", len(response.Answer)))
		return 1
	}

	returnedIPs := make(map[string]bool)
	for _, answer := range response.Answer {
		if aRecord, ok := answer.(*mdns.A); ok {
			returnedIPs[aRecord.A.String()] = true
			p.Info(fmt.Sprintf("  cluster.internal -> %s", aRecord.A.String()))
		}
	}

	for _, ip := range clusterIPs {
		if !returnedIPs[ip] {
			p.Error(fmt.Sprintf("Expected IP %s not found in cluster response", ip))
			return 1
		}
	}
	p.Success("All cluster IPs returned correctly")

	// Step 8: Test non-existent host (NXDOMAIN)
	p.Step("Testing non-existent host resolution (NXDOMAIN)")

	msg = new(mdns.Msg)
	msg.SetQuestion("nonexistent.internal.", mdns.TypeA)

	response, _, err = client.Exchange(msg, dnsBindAddr)
	if err != nil {
		p.Error(fmt.Sprintf("DNS query failed: %v", err))
		return 1
	}

	if response.Rcode != mdns.RcodeNameError {
		p.Error(fmt.Sprintf("Expected NXDOMAIN for nonexistent.internal, got rcode %d", response.Rcode))
		return 1
	}
	p.Success("Non-existent host correctly returns NXDOMAIN")

	// Step 9: Test health-aware resolution
	p.Step("Testing health-aware DNS resolution")

	// Mark api.internal as unhealthy
	healthErr := manager.UpdateHealth(ctx, "api.internal", false)
	if healthErr != nil {
		p.Error(fmt.Sprintf("Failed to mark api.internal unhealthy: %v", healthErr))
		return 1
	}
	p.Info("Marked api.internal as unhealthy")

	msg = new(mdns.Msg)
	msg.SetQuestion("api.internal.", mdns.TypeA)

	response, _, err = client.Exchange(msg, dnsBindAddr)
	if err != nil {
		p.Error(fmt.Sprintf("DNS query failed: %v", err))
		return 1
	}

	if response.Rcode != mdns.RcodeNameError {
		p.Error(fmt.Sprintf("Expected NXDOMAIN for unhealthy api.internal, got rcode %d with %d answers", response.Rcode, len(response.Answer)))
		return 1
	}
	p.Success("Unhealthy host correctly excluded from DNS resolution")

	// Re-enable health
	healthErr = manager.UpdateHealth(ctx, "api.internal", true)
	if healthErr != nil {
		p.Error(fmt.Sprintf("Failed to re-enable api.internal: %v", healthErr))
		return 1
	}
	p.Info("Re-enabled api.internal health")

	msg = new(mdns.Msg)
	msg.SetQuestion("api.internal.", mdns.TypeA)

	response, _, err = client.Exchange(msg, dnsBindAddr)
	if err != nil {
		p.Error(fmt.Sprintf("DNS query failed: %v", err))
		return 1
	}

	if len(response.Answer) == 0 {
		p.Error("Expected api.internal to be resolvable after health re-enabled")
		return 1
	}
	p.Success("Re-enabled host correctly returned in DNS resolution")

	// Step 10: Test external domain forwarding (if network available)
	p.Step("Testing external domain forwarding (google.com)")
	p.Info("Note: This test requires network connectivity to 8.8.8.8")

	msg = new(mdns.Msg)
	msg.SetQuestion("google.com.", mdns.TypeA)

	response, _, err = client.Exchange(msg, dnsBindAddr)
	switch {
	case err != nil:
		p.Warning(fmt.Sprintf("External DNS forwarding failed (may be expected in isolated environments): %v", err))
	case len(response.Answer) > 0:
		p.Success("External domain forwarding works")
		for _, answer := range response.Answer {
			if aRecord, ok := answer.(*mdns.A); ok {
				p.Info(fmt.Sprintf("  google.com -> %s", aRecord.A.String()))
				break // Just show first IP
			}
		}
	default:
		p.Warning("External DNS returned empty response (may be expected)")
	}

	// Step 11: Test TCP resolution
	p.Step("Testing TCP DNS resolution")

	tcpClient := &mdns.Client{
		Net:     "tcp",
		Timeout: 5 * time.Second,
	}

	msg = new(mdns.Msg)
	msg.SetQuestion("web.internal.", mdns.TypeA)

	response, _, err = tcpClient.Exchange(msg, dnsBindAddr)
	if err != nil {
		p.Error(fmt.Sprintf("TCP DNS query failed: %v", err))
		return 1
	}

	if len(response.Answer) == 0 {
		p.Error("TCP DNS query returned no answers")
		return 1
	}
	p.Success("TCP DNS resolution works correctly")

	// Step 12: Test using system dig command (if available)
	p.Step("Testing DNS resolution with dig command")

	digCmd := exec.CommandContext(ctx, "dig", "@127.0.0.1", "-p", fmt.Sprintf("%d", dnsServerPort), "web.internal", "+short")
	digOutput, digErr := digCmd.CombinedOutput()
	if digErr != nil {
		p.Warning(fmt.Sprintf("dig command failed (may not be installed): %v", digErr))
	} else {
		result := strings.TrimSpace(string(digOutput))
		switch {
		case result == "10.0.1.10":
			p.Success(fmt.Sprintf("dig resolved web.internal to %s", result))
		case result != "":
			p.Warning(fmt.Sprintf("dig returned unexpected result: %s", result))
		default:
			p.Warning("dig returned empty result")
		}
	}

	// Step 13: Unregister a host
	p.Step("Testing host unregistration")

	unregErr := manager.UnregisterHost(ctx, "cache.internal")
	if unregErr != nil {
		p.Error(fmt.Sprintf("Failed to unregister cache.internal: %v", unregErr))
		return 1
	}
	p.Info("Unregistered cache.internal")

	msg = new(mdns.Msg)
	msg.SetQuestion("cache.internal.", mdns.TypeA)

	response, _, err = client.Exchange(msg, dnsBindAddr)
	if err != nil {
		p.Error(fmt.Sprintf("DNS query failed: %v", err))
		return 1
	}

	if response.Rcode != mdns.RcodeNameError {
		p.Error(fmt.Sprintf("Expected NXDOMAIN for unregistered cache.internal, got rcode %d", response.Rcode))
		return 1
	}
	p.Success("Unregistered host correctly returns NXDOMAIN")

	return 0
}

// getProjectRoot returns the project root directory
func getProjectRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "../../.."
	}
	// If we're in test/integration/vpc, go up 3 levels
	if filepath.Base(wd) == "vpc" {
		return filepath.Join(wd, "..", "..", "..")
	}
	// Otherwise assume we're at project root
	return wd
}
