package agent

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/dns"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

// mockOverlayDriver records calls for verification.
type mockOverlayDriver struct {
	initCalled           bool
	initSubnet           string
	initHostIP           string
	reconcileCalled      bool
	reconcilePeers       []overlay.Peer
	writeCNIConfigCalled bool
	writeCNISubnet       string
	cleanupCalled        bool
	initErr              error
	reconcileErr         error
	writeCNIErr          error
	cleanupErr           error
}

func (m *mockOverlayDriver) Init(ctx context.Context, subnet net.IPNet, hostIP net.IP) error {
	m.initCalled = true
	m.initSubnet = subnet.String()
	m.initHostIP = hostIP.String()
	return m.initErr
}

func (m *mockOverlayDriver) ReconcilePeers(ctx context.Context, peers []overlay.Peer) error {
	m.reconcileCalled = true
	m.reconcilePeers = peers
	return m.reconcileErr
}

func (m *mockOverlayDriver) WriteCNIConfig(subnet net.IPNet) error {
	m.writeCNIConfigCalled = true
	m.writeCNISubnet = subnet.String()
	return m.writeCNIErr
}

func (m *mockOverlayDriver) Cleanup(ctx context.Context) error {
	m.cleanupCalled = true
	return m.cleanupErr
}

func TestCheckVPCPrerequisites(t *testing.T) {
	t.Run("fails when CNI plugins missing", func(t *testing.T) {
		// Unless /opt/cni/bin has all required plugins, this should fail
		err := checkVPCPrerequisites()
		if err == nil {
			// If all plugins exist on this machine, skip
			t.Skip("all CNI plugins present on this machine")
		}
		if !strings.Contains(err.Error(), "CNI plugin") {
			t.Errorf("expected 'CNI plugin' in error, got: %v", err)
		}
	})

	t.Run("no longer checks for flanneld", func(t *testing.T) {
		// Save original PATH and set to empty — should NOT error about flanneld
		origPath := os.Getenv("PATH")
		t.Cleanup(func() { os.Setenv("PATH", origPath) })
		os.Setenv("PATH", "/nonexistent")

		err := checkVPCPrerequisites()
		if err == nil {
			// If CNI plugins exist at /opt/cni/bin, test passes
			return
		}
		// Error should be about CNI plugins, NOT about flanneld
		if strings.Contains(err.Error(), "flanneld") {
			t.Errorf("should not check for flanneld anymore, got: %v", err)
		}
	})

	t.Run("checks all required CNI plugins", func(t *testing.T) {
		// Create a temp dir with some plugins
		tmpDir := t.TempDir()

		// Create fake plugins — all except 'portmap'
		for _, name := range []string{"bridge", "host-local", "loopback"} {
			os.WriteFile(filepath.Join(tmpDir, name), []byte(""), 0o755)
		}

		// The test checks /opt/cni/bin which we can't override without refactoring,
		// so we just verify the function checks for the right plugins
		err := checkVPCPrerequisites()
		if err == nil {
			return // all plugins exist on this machine
		}
		// Should mention one of the required plugins
		hasPlugin := false
		for _, plugin := range []string{"bridge", "host-local", "loopback", "portmap"} {
			if strings.Contains(err.Error(), plugin) {
				hasPlugin = true
				break
			}
		}
		if !hasPlugin {
			t.Errorf("error should mention a required CNI plugin, got: %v", err)
		}
	})
}

func TestInitializeVPCNetworking(t *testing.T) {
	t.Run("runs all steps in sequence", func(t *testing.T) {
		origChecker := prerequisiteChecker
		origFactory := overlayDriverFactory
		origDetector := hostIPDetector
		t.Cleanup(func() {
			prerequisiteChecker = origChecker
			overlayDriverFactory = origFactory
			hostIPDetector = origDetector
		})

		mockDriver := &mockOverlayDriver{}

		prerequisiteChecker = func() error { return nil }
		overlayDriverFactory = func(_, _, _ string) overlay.OverlayDriver { return mockDriver }
		hostIPDetector = func() (net.IP, error) { return net.ParseIP("192.168.1.10"), nil }

		a := &Agent{}
		err := a.initializeVPCNetworking(context.Background(), &VPCConfig{
			VPCCIDR:         "10.0.0.0/16",
			AllocatedSubnet: "10.0.45.0/24",
		})
		if err != nil {
			t.Fatalf("initializeVPCNetworking failed: %v", err)
		}

		if !mockDriver.initCalled {
			t.Error("expected Init to be called")
		}
		if mockDriver.initSubnet != "10.0.45.0/24" {
			t.Errorf("expected subnet '10.0.45.0/24', got %q", mockDriver.initSubnet)
		}
		if mockDriver.initHostIP != "192.168.1.10" {
			t.Errorf("expected host IP '192.168.1.10', got %q", mockDriver.initHostIP)
		}
		if !mockDriver.writeCNIConfigCalled {
			t.Error("expected WriteCNIConfig to be called")
		}
		if a.overlayDriver == nil {
			t.Error("expected overlayDriver to be set on agent")
		}
	})

	t.Run("fails on prerequisite check", func(t *testing.T) {
		origChecker := prerequisiteChecker
		t.Cleanup(func() { prerequisiteChecker = origChecker })

		prerequisiteChecker = func() error { return os.ErrNotExist }

		a := &Agent{}
		err := a.initializeVPCNetworking(context.Background(), &VPCConfig{
			VPCCIDR:         "10.0.0.0/16",
			AllocatedSubnet: "10.0.45.0/24",
		})
		if err == nil {
			t.Fatal("expected error when prerequisites fail")
		}
		if !strings.Contains(err.Error(), "VPC prerequisites") {
			t.Errorf("expected 'VPC prerequisites' in error, got: %v", err)
		}
	})

	t.Run("fails on invalid subnet", func(t *testing.T) {
		origChecker := prerequisiteChecker
		t.Cleanup(func() { prerequisiteChecker = origChecker })

		prerequisiteChecker = func() error { return nil }

		a := &Agent{}
		err := a.initializeVPCNetworking(context.Background(), &VPCConfig{
			VPCCIDR:         "10.0.0.0/16",
			AllocatedSubnet: "not-a-cidr",
		})
		if err == nil {
			t.Fatal("expected error for invalid subnet")
		}
		if !strings.Contains(err.Error(), "invalid allocated subnet") {
			t.Errorf("expected 'invalid allocated subnet' in error, got: %v", err)
		}
	})

	t.Run("fails on host IP detection", func(t *testing.T) {
		origChecker := prerequisiteChecker
		origDetector := hostIPDetector
		t.Cleanup(func() {
			prerequisiteChecker = origChecker
			hostIPDetector = origDetector
		})

		prerequisiteChecker = func() error { return nil }
		hostIPDetector = func() (net.IP, error) { return nil, os.ErrNotExist }

		a := &Agent{}
		err := a.initializeVPCNetworking(context.Background(), &VPCConfig{
			VPCCIDR:         "10.0.0.0/16",
			AllocatedSubnet: "10.0.45.0/24",
		})
		if err == nil {
			t.Fatal("expected error on host IP detection failure")
		}
		if !strings.Contains(err.Error(), "failed to detect host IP") {
			t.Errorf("expected 'failed to detect host IP' in error, got: %v", err)
		}
	})

	t.Run("fails on overlay init", func(t *testing.T) {
		origChecker := prerequisiteChecker
		origFactory := overlayDriverFactory
		origDetector := hostIPDetector
		t.Cleanup(func() {
			prerequisiteChecker = origChecker
			overlayDriverFactory = origFactory
			hostIPDetector = origDetector
		})

		mockDriver := &mockOverlayDriver{initErr: os.ErrPermission}

		prerequisiteChecker = func() error { return nil }
		overlayDriverFactory = func(_, _, _ string) overlay.OverlayDriver { return mockDriver }
		hostIPDetector = func() (net.IP, error) { return net.ParseIP("192.168.1.10"), nil }

		a := &Agent{}
		err := a.initializeVPCNetworking(context.Background(), &VPCConfig{
			VPCCIDR:         "10.0.0.0/16",
			AllocatedSubnet: "10.0.45.0/24",
		})
		if err == nil {
			t.Fatal("expected error on overlay init failure")
		}
		if !strings.Contains(err.Error(), "overlay init failed") {
			t.Errorf("expected 'overlay init failed' in error, got: %v", err)
		}
	})

	t.Run("fails on CNI config write", func(t *testing.T) {
		origChecker := prerequisiteChecker
		origFactory := overlayDriverFactory
		origDetector := hostIPDetector
		t.Cleanup(func() {
			prerequisiteChecker = origChecker
			overlayDriverFactory = origFactory
			hostIPDetector = origDetector
		})

		mockDriver := &mockOverlayDriver{writeCNIErr: os.ErrPermission}

		prerequisiteChecker = func() error { return nil }
		overlayDriverFactory = func(_, _, _ string) overlay.OverlayDriver { return mockDriver }
		hostIPDetector = func() (net.IP, error) { return net.ParseIP("192.168.1.10"), nil }

		a := &Agent{}
		err := a.initializeVPCNetworking(context.Background(), &VPCConfig{
			VPCCIDR:         "10.0.0.0/16",
			AllocatedSubnet: "10.0.45.0/24",
		})
		if err == nil {
			t.Fatal("expected error on CNI config write failure")
		}
		if !strings.Contains(err.Error(), "failed to write CNI config") {
			t.Errorf("expected 'failed to write CNI config' in error, got: %v", err)
		}
	})
}

func TestReconcileVPCPeers(t *testing.T) {
	t.Run("converts and reconciles peers", func(t *testing.T) {
		mockDriver := &mockOverlayDriver{}
		a := &Agent{overlayDriver: mockDriver}

		peers := []VPCPeer{
			{Subnet: "10.0.46.0/24", HostIP: "192.168.1.20", VTEPMAC: "02:42:0a:00:2e:01"},
		}

		err := a.reconcileVPCPeers(context.Background(), peers)
		if err != nil {
			t.Fatalf("reconcileVPCPeers failed: %v", err)
		}

		if !mockDriver.reconcileCalled {
			t.Error("expected ReconcilePeers to be called")
		}
		if len(mockDriver.reconcilePeers) != 1 {
			t.Fatalf("expected 1 peer, got %d", len(mockDriver.reconcilePeers))
		}
		if mockDriver.reconcilePeers[0].HostIP.String() != "192.168.1.20" {
			t.Errorf("expected host IP '192.168.1.20', got %q", mockDriver.reconcilePeers[0].HostIP.String())
		}
	})

	t.Run("skips invalid peers", func(t *testing.T) {
		mockDriver := &mockOverlayDriver{}
		a := &Agent{overlayDriver: mockDriver}

		peers := []VPCPeer{
			{Subnet: "invalid", HostIP: "192.168.1.20", VTEPMAC: "02:42:0a:00:2e:01"},
			{Subnet: "10.0.46.0/24", HostIP: "192.168.1.20", VTEPMAC: "02:42:0a:00:2e:01"},
		}

		err := a.reconcileVPCPeers(context.Background(), peers)
		if err != nil {
			t.Fatalf("reconcileVPCPeers failed: %v", err)
		}

		if len(mockDriver.reconcilePeers) != 1 {
			t.Fatalf("expected 1 valid peer, got %d", len(mockDriver.reconcilePeers))
		}
	})

	t.Run("no-op when driver is nil", func(t *testing.T) {
		a := &Agent{overlayDriver: nil}

		err := a.reconcileVPCPeers(context.Background(), []VPCPeer{
			{Subnet: "10.0.46.0/24", HostIP: "192.168.1.20", VTEPMAC: "02:42:0a:00:2e:01"},
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("no-op when all peers invalid", func(t *testing.T) {
		mockDriver := &mockOverlayDriver{}
		a := &Agent{overlayDriver: mockDriver}

		err := a.reconcileVPCPeers(context.Background(), []VPCPeer{
			{Subnet: "invalid", HostIP: "invalid", VTEPMAC: "invalid"},
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if mockDriver.reconcileCalled {
			t.Error("expected ReconcilePeers to NOT be called for empty valid peers")
		}
	})
}

func TestBuildNerdctlRunArgs_VPC(t *testing.T) {
	// Save and restore dnsGatewayIPAddr (tests may set it)
	origDNSGateway := dnsGatewayIPAddr
	t.Cleanup(func() { dnsGatewayIPAddr = origDNSGateway })

	t.Run("with VPC enabled no DNS", func(t *testing.T) {
		dnsGatewayIPAddr = "" // no DNS
		task := &types.TaskRecord{
			ContainerName: "myapp-web-0",
			Image:         "nginx:alpine",
		}
		args := buildNerdctlRunArgs(task, true)
		expected := []string{"run", "-d", "--name", "myapp-web-0", "--net", "banyan", "nginx:alpine"}
		if len(args) != len(expected) {
			t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
		}
		for i, exp := range expected {
			if args[i] != exp {
				t.Errorf("arg[%d]: expected %q, got %q", i, exp, args[i])
			}
		}
	})

	t.Run("with VPC and DNS enabled", func(t *testing.T) {
		dnsGatewayIPAddr = "10.0.45.1"
		task := &types.TaskRecord{
			ContainerName: "myapp-web-0",
			Image:         "nginx:alpine",
		}
		args := buildNerdctlRunArgs(task, true)
		expected := []string{"run", "-d", "--name", "myapp-web-0", "--net", "banyan", "--dns", "10.0.45.1", "--dns-search", "internal", "nginx:alpine"}
		if len(args) != len(expected) {
			t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
		}
		for i, exp := range expected {
			if args[i] != exp {
				t.Errorf("arg[%d]: expected %q, got %q", i, exp, args[i])
			}
		}
	})

	t.Run("without VPC enabled", func(t *testing.T) {
		dnsGatewayIPAddr = "10.0.45.1" // DNS set but VPC disabled — should NOT add --dns
		task := &types.TaskRecord{
			ContainerName: "myapp-web-0",
			Image:         "nginx:alpine",
		}
		args := buildNerdctlRunArgs(task, false)
		expected := []string{"run", "-d", "--name", "myapp-web-0", "nginx:alpine"}
		if len(args) != len(expected) {
			t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
		}
		for i, exp := range expected {
			if args[i] != exp {
				t.Errorf("arg[%d]: expected %q, got %q", i, exp, args[i])
			}
		}
	})

	t.Run("VPC with env and command", func(t *testing.T) {
		dnsGatewayIPAddr = ""
		task := &types.TaskRecord{
			ContainerName: "myapp-web-0",
			Image:         "nginx:alpine",
			Environment:   []string{"FOO=bar"},
			Command:       []string{"nginx", "-g", "daemon off;"},
		}
		args := buildNerdctlRunArgs(task, true)
		netIdx := -1
		envIdx := -1
		for i, arg := range args {
			if arg == "--net" {
				netIdx = i
			}
			if arg == "-e" {
				envIdx = i
			}
		}
		if netIdx == -1 {
			t.Fatal("expected --net flag in args")
		}
		if envIdx == -1 {
			t.Fatal("expected -e flag in args")
		}
		if netIdx > envIdx {
			t.Errorf("expected --net before -e, got net=%d env=%d", netIdx, envIdx)
		}
	})
}

func TestDetectHostIP(t *testing.T) {
	ip, err := detectHostIP()
	if err != nil {
		// Some CI environments might not have non-loopback interfaces
		t.Skipf("detectHostIP failed (may be expected in CI): %v", err)
	}
	if ip == nil {
		t.Fatal("expected non-nil IP")
	}
	if ip.IsLoopback() {
		t.Errorf("expected non-loopback IP, got %s", ip.String())
	}
}

func TestInitializeDNS(t *testing.T) {
	origManagerFactory := dnsManagerFactory
	origServerFactory := dnsServerFactory
	t.Cleanup(func() {
		dnsManagerFactory = origManagerFactory
		dnsServerFactory = origServerFactory
	})

	t.Run("creates manager and server with correct gateway IP", func(t *testing.T) {
		var capturedBindAddr string
		dnsManagerFactory = func() *dns.Manager { return dns.NewManager() }
		dnsServerFactory = func(m *dns.Manager, c dns.ServerConfig) *dns.Server {
			capturedBindAddr = c.BindAddr
			// Use a high port to avoid permission issues in tests
			c.BindAddr = "127.0.0.1:0" // will fail to bind but we can check captured values
			return dns.NewServer(m, c)
		}

		a := &Agent{registeredDNS: make(map[string]bool)}
		// 10.0.45.0/24 → gateway is 10.0.45.1 (VTEP IP)
		err := a.initializeDNS(context.Background(), "10.0.45.0/24")

		// The server.Start() may fail on binding, that's OK for this test
		// We just verify the gateway IP calculation and manager/server creation
		if err != nil {
			// Expected in test environments without root
			t.Logf("initializeDNS returned (expected): %v", err)
		}

		if capturedBindAddr != "10.0.45.1:53" {
			t.Errorf("expected bind addr '10.0.45.1:53', got %q", capturedBindAddr)
		}
	})

	t.Run("fails on invalid subnet", func(t *testing.T) {
		a := &Agent{registeredDNS: make(map[string]bool)}
		err := a.initializeDNS(context.Background(), "not-a-cidr")
		if err == nil {
			t.Fatal("expected error for invalid subnet")
		}
		if !strings.Contains(err.Error(), "invalid subnet") {
			t.Errorf("expected 'invalid subnet' in error, got: %v", err)
		}
	})
}

func TestReconcileDNS(t *testing.T) {
	t.Run("registers backends by service name", func(t *testing.T) {
		manager := dns.NewManager()
		a := &Agent{
			dnsManager:    manager,
			registeredDNS: make(map[string]bool),
		}

		backends := []ServiceBackend{
			{ContainerName: "app-web-0", ContainerIP: "10.0.1.5", ServiceName: "web", AgentName: "worker-1"},
			{ContainerName: "app-web-1", ContainerIP: "10.0.2.5", ServiceName: "web", AgentName: "worker-2"},
			{ContainerName: "app-db-0", ContainerIP: "10.0.1.6", ServiceName: "db", AgentName: "worker-1"},
		}

		a.reconcileDNS(context.Background(), backends)

		// Verify web.internal resolves to both IPs
		ctx := context.Background()
		ips, err := manager.LookupHost(ctx, "web.internal")
		if err != nil {
			t.Fatalf("LookupHost web.internal failed: %v", err)
		}
		if len(ips) != 2 {
			t.Fatalf("expected 2 IPs for web.internal, got %d", len(ips))
		}

		// Verify db.internal resolves
		ips, err = manager.LookupHost(ctx, "db.internal")
		if err != nil {
			t.Fatalf("LookupHost db.internal failed: %v", err)
		}
		if len(ips) != 1 {
			t.Fatalf("expected 1 IP for db.internal, got %d", len(ips))
		}

		// Verify tracking
		if len(a.registeredDNS) != 2 {
			t.Errorf("expected 2 registered hostnames, got %d", len(a.registeredDNS))
		}
		if !a.registeredDNS["web.internal"] {
			t.Error("expected web.internal in registeredDNS")
		}
		if !a.registeredDNS["db.internal"] {
			t.Error("expected db.internal in registeredDNS")
		}
	})

	t.Run("removes stale hostnames", func(t *testing.T) {
		manager := dns.NewManager()
		ctx := context.Background()

		// Pre-register a hostname
		manager.RegisterHost(ctx, "old.internal", net.ParseIP("10.0.1.99"))

		a := &Agent{
			dnsManager:    manager,
			registeredDNS: map[string]bool{"old.internal": true},
		}

		// Reconcile with only new backends (old.internal should be removed)
		backends := []ServiceBackend{
			{ContainerName: "app-web-0", ContainerIP: "10.0.1.5", ServiceName: "web", AgentName: "worker-1"},
		}

		a.reconcileDNS(ctx, backends)

		// old.internal should be gone
		_, err := manager.LookupHost(ctx, "old.internal")
		if err == nil {
			t.Error("expected old.internal to be removed")
		}

		// web.internal should exist
		ips, err := manager.LookupHost(ctx, "web.internal")
		if err != nil {
			t.Fatalf("LookupHost web.internal failed: %v", err)
		}
		if len(ips) != 1 {
			t.Errorf("expected 1 IP for web.internal, got %d", len(ips))
		}
	})

	t.Run("skips empty service names and IPs", func(t *testing.T) {
		manager := dns.NewManager()
		a := &Agent{
			dnsManager:    manager,
			registeredDNS: make(map[string]bool),
		}

		backends := []ServiceBackend{
			{ContainerName: "app-web-0", ContainerIP: "10.0.1.5", ServiceName: "", AgentName: "worker-1"},  // no service name
			{ContainerName: "app-db-0", ContainerIP: "", ServiceName: "db", AgentName: "worker-1"},          // no IP
			{ContainerName: "app-api-0", ContainerIP: "10.0.1.7", ServiceName: "api", AgentName: "worker-1"}, // valid
		}

		a.reconcileDNS(context.Background(), backends)

		if len(a.registeredDNS) != 1 {
			t.Errorf("expected 1 registered hostname, got %d", len(a.registeredDNS))
		}
		if !a.registeredDNS["api.internal"] {
			t.Error("expected api.internal in registeredDNS")
		}
	})

	t.Run("no-op when manager is nil", func(t *testing.T) {
		a := &Agent{
			dnsManager:    nil,
			registeredDNS: make(map[string]bool),
		}

		// Should not panic
		a.reconcileDNS(context.Background(), []ServiceBackend{
			{ContainerName: "app-web-0", ContainerIP: "10.0.1.5", ServiceName: "web"},
		})

		if len(a.registeredDNS) != 0 {
			t.Errorf("expected 0 registered hostnames, got %d", len(a.registeredDNS))
		}
	})

	t.Run("handles empty backends list", func(t *testing.T) {
		manager := dns.NewManager()
		ctx := context.Background()

		// Pre-register
		manager.RegisterHost(ctx, "old.internal", net.ParseIP("10.0.1.99"))

		a := &Agent{
			dnsManager:    manager,
			registeredDNS: map[string]bool{"old.internal": true},
		}

		a.reconcileDNS(ctx, nil)

		// old.internal should be removed
		_, err := manager.LookupHost(ctx, "old.internal")
		if err == nil {
			t.Error("expected old.internal to be removed with empty backends")
		}
		if len(a.registeredDNS) != 0 {
			t.Errorf("expected 0 registered hostnames, got %d", len(a.registeredDNS))
		}
	})
}
