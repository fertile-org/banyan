//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fertile-org/banyan/pkg/vpc"
	"github.com/fertile-org/banyan/test/integration/helpers"
)

const (
	etcdEndpointHost = "http://127.0.0.1:2379" // For host machine (explicit IPv4)
	vpcCIDR          = "10.0.0.0/16"
	host1Name        = "vpc-host1"
	host2Name        = "vpc-host2"
	app1Name         = "app1"
	app2Name         = "app2"
	networkID        = "test-network"
	testNetworkName  = "vpc-test-net"
)

var (
	etcdEndpointContainer string // For containers - detected dynamically from network gateway
)

func main() {
	ctx := context.Background()
	p := helpers.NewPrinter()

	// Check if running as root
	if os.Geteuid() != 0 {
		p.Error("This test requires root privileges")
		p.Info("Please run with: sudo go run test/integration/vpc/run_multi_host_integration.go")
		os.Exit(1)
	}

	exitCode := runMultiHostTest(ctx, p)
	if exitCode == 0 {
		p.Result(true, "All multi-host integration tests passed!")
	} else {
		p.Result(false, fmt.Sprintf("Multi-host integration tests failed with exit code %d", exitCode))
	}
	os.Exit(exitCode)
}

func runMultiHostTest(ctx context.Context, p *helpers.Printer) int {
	p.Title("Multi-Host VPC Integration Test")
	p.Info("Testing cross-host container communication via Flannel CNI + etcd")

	// Cleanup on exit
	defer cleanup(ctx, p)

	// Phase 1: Infrastructure Setup
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Phase 1: Infrastructure Setup")
	fmt.Println(strings.Repeat("=", 60))

	if err := checkPrerequisites(ctx, p); err != nil {
		p.Error(fmt.Sprintf("Prerequisites check failed: %v", err))
		return 1
	}

	if err := buildVPCCLI(ctx, p); err != nil {
		p.Error(fmt.Sprintf("Failed to build vpc-cli: %v", err))
		return 1
	}

	if err := startEtcd(ctx, p); err != nil {
		p.Error(fmt.Sprintf("Failed to start etcd: %v", err))
		return 1
	}

	if err := createTestNetwork(ctx, p); err != nil {
		p.Error(fmt.Sprintf("Failed to create test network: %v", err))
		return 1
	}

	if err := createSimulatedHosts(ctx, p); err != nil {
		p.Error(fmt.Sprintf("Failed to create simulated hosts: %v", err))
		return 1
	}

	// Phase 2: Network Configuration
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Phase 2: Network Configuration")
	fmt.Println(strings.Repeat("=", 60))

	if err := configureFlannelNetwork(ctx, p); err != nil {
		p.Error(fmt.Sprintf("Failed to configure Flannel network: %v", err))
		return 1
	}

	if err := startFlannelOnHosts(ctx, p); err != nil {
		p.Error(fmt.Sprintf("Failed to start Flannel on hosts: %v", err))
		return 1
	}

	// Phase 3: Container Creation & Testing
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Phase 3: Container Creation & Testing")
	fmt.Println(strings.Repeat("=", 60))

	ip1, err := createAppContainer(ctx, p, host1Name, app1Name)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create app1 container: %v", err))
		return 1
	}

	ip2, err := createAppContainer(ctx, p, host2Name, app2Name)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create app2 container: %v", err))
		return 1
	}

	// Phase 4: Connectivity Tests
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Phase 4: Cross-Host Connectivity Tests")
	fmt.Println(strings.Repeat("=", 60))

	if err := testCrossHostPing(ctx, p, host1Name, app1Name, ip1, ip2); err != nil {
		p.Error(fmt.Sprintf("Ping test failed (app1 -> app2): %v", err))
		return 1
	}

	if err := testCrossHostPing(ctx, p, host2Name, app2Name, ip2, ip1); err != nil {
		p.Error(fmt.Sprintf("Ping test failed (app2 -> app1): %v", err))
		return 1
	}

	// Phase 5: State Verification
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Phase 5: State Verification")
	fmt.Println(strings.Repeat("=", 60))

	if err := verifyEtcdState(ctx, p); err != nil {
		p.Error(fmt.Sprintf("etcd state verification failed: %v", err))
		return 1
	}

	p.Success("All tests passed!")
	return 0
}

func checkPrerequisites(ctx context.Context, p *helpers.Printer) error {
	p.Step("Checking prerequisites")

	// Check containerd/nerdctl
	cmd := exec.CommandContext(ctx, "sudo", "nerdctl", "info")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nerdctl/containerd not available: %w", err)
	}
	p.Info("✓ containerd/nerdctl available")

	// Check etcdctl
	if err := exec.CommandContext(ctx, "which", "etcdctl").Run(); err != nil {
		return fmt.Errorf("etcdctl not found: %w", err)
	}
	p.Info("✓ etcdctl available")

	return nil
}

func buildVPCCLI(ctx context.Context, p *helpers.Printer) error {
	p.Step("Building vpc-cli binary")

	// Get project root
	projectRoot := "/home/hungnguyenba/workspace/fertile/banyan"
	vpcCliDir := filepath.Join(projectRoot, "cmd", "vpc-cli")

	cmd := exec.CommandContext(ctx, "go", "build", "-o", "/tmp/vpc-cli")
	cmd.Dir = vpcCliDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed: %w\nOutput: %s", err, output)
	}

	p.Info("✓ vpc-cli built successfully")
	return nil
}

func startEtcd(ctx context.Context, p *helpers.Printer) error {
	p.Step("Starting etcd server")

	// Stop any existing etcd
	exec.CommandContext(ctx, "/tmp/vpc-cli", "etcd", "stop").Run()

	// Clean up old data
	exec.CommandContext(ctx, "rm", "-rf", "/tmp/test-etcd").Run()

	// Start etcd
	cmd := exec.CommandContext(ctx, "/tmp/vpc-cli", "etcd", "start",
		"--data-dir=/tmp/test-etcd",
		"--listen-client-urls=http://0.0.0.0:2379",
		"--advertise-client-urls=http://127.0.0.1:2379")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start etcd: %w", err)
	}

	// Wait for etcd to be ready
	p.Info("Waiting for etcd to be ready...")
	time.Sleep(3 * time.Second)

	// Verify etcd is running
	cmd = exec.CommandContext(ctx, "/tmp/vpc-cli", "etcd", "status",
		"--endpoints="+etcdEndpointHost)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("etcd not ready: %w", err)
	}

	p.Info("✓ etcd server started")
	return nil
}

func createTestNetwork(ctx context.Context, p *helpers.Printer) error {
	p.Step("Creating containerd test network")

	// Remove old network if exists
	exec.CommandContext(ctx, "sudo", "nerdctl", "network", "rm", testNetworkName).Run()

	// Create bridge network for DinD containers
	cmd := exec.CommandContext(ctx, "sudo", "nerdctl", "network", "create", testNetworkName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create network: %w\nOutput: %s", err, output)
	}

	// Inspect network to get gateway IP
	cmd = exec.CommandContext(ctx, "sudo", "nerdctl", "network", "inspect", testNetworkName)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to inspect network: %w", err)
	}

	// Extract gateway from output (format: "Gateway": "10.4.1.1")
	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")
	var gateway string
	for _, line := range lines {
		if strings.Contains(line, `"Gateway"`) {
			// Extract IP from line like: "Gateway": "10.4.1.1"
			parts := strings.Split(line, `"`)
			if len(parts) >= 4 {
				gateway = parts[3]
				break
			}
		}
	}

	if gateway == "" {
		return fmt.Errorf("failed to detect network gateway from network inspect output")
	}

	// Set the container etcd endpoint using detected gateway
	etcdEndpointContainer = fmt.Sprintf("http://%s:2379", gateway)

	p.Info("✓ Test network created")
	p.Info(fmt.Sprintf("  Gateway: %s", gateway))
	p.Info(fmt.Sprintf("  Container etcd endpoint: %s", etcdEndpointContainer))
	return nil
}

func createSimulatedHosts(ctx context.Context, p *helpers.Printer) error {
	p.Step("Creating simulated hosts (containerd-in-containerd)")

	hosts := []string{host1Name, host2Name}

	for _, hostName := range hosts {
		// Remove old container if exists
		exec.CommandContext(ctx, "sudo", "nerdctl", "stop", hostName).Run()
		exec.CommandContext(ctx, "sudo", "nerdctl", "rm", hostName).Run()

		// Create containerd-in-containerd container
		// Using docker:dind image which has containerd installed
		cmd := exec.CommandContext(ctx, "sudo", "nerdctl", "run", "-d",
			"--privileged",
			"--name", hostName,
			"--hostname", hostName,
			"--network", testNetworkName,
			"docker:dind",
			"dockerd-entrypoint.sh", "--iptables=false")

		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to create %s: %w\nOutput: %s", hostName, err, output)
		}

		p.Info(fmt.Sprintf("✓ Created host: %s", hostName))

		// Wait for daemon inside container to be ready
		time.Sleep(3 * time.Second)

		// Copy vpc-cli binary into the host
		cmd = exec.CommandContext(ctx, "sudo", "nerdctl", "cp", "/tmp/vpc-cli",
			fmt.Sprintf("%s:/usr/local/bin/vpc-cli", hostName))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to copy vpc-cli to %s: %w", hostName, err)
		}

		// Install required tools
		nerdctlExec(ctx, hostName, "apk", "add", "--no-cache", "iptables", "iproute2", "curl")

		p.Info(fmt.Sprintf("✓ Configured host: %s", hostName))
	}

	return nil
}

func configureFlannelNetwork(ctx context.Context, p *helpers.Printer) error {
	p.Step("Configuring Flannel network in etcd")

	// Use VPC package to initialize network
	if err := vpc.InitializeNetwork(ctx, []string{etcdEndpointHost}, vpcCIDR); err != nil {
		return fmt.Errorf("failed to initialize network: %w", err)
	}

	p.Info("✓ Flannel network configured in etcd")
	return nil
}

func startFlannelOnHosts(ctx context.Context, p *helpers.Printer) error {
	p.Step("Starting Flannel daemon on each host")

	hosts := []string{host1Name, host2Name}

	for _, hostName := range hosts {
		// Install Flannel binary
		p.Info(fmt.Sprintf("Installing Flannel on %s...", hostName))
		nerdctlExec(ctx, hostName, "sh", "-c",
			"wget -q -O /usr/local/bin/flanneld https://github.com/flannel-io/flannel/releases/download/v0.22.0/flanneld-amd64 && chmod +x /usr/local/bin/flanneld")

		// Start Flannel daemon
		nerdctlExec(ctx, hostName, "mkdir", "-p", "/run/flannel")

		// Start flanneld in background (use container endpoint)
		cmd := fmt.Sprintf("nohup flanneld --etcd-endpoints=%s --iface=eth0 --subnet-file=/run/flannel/subnet.env > /var/log/flanneld.log 2>&1 &", etcdEndpointContainer)
		nerdctlExec(ctx, hostName, "sh", "-c", cmd)

		p.Info(fmt.Sprintf("Waiting for Flannel to allocate subnet on %s...", hostName))
		time.Sleep(5 * time.Second)

		// Verify subnet allocation
		output := nerdctlExec(ctx, hostName, "cat", "/run/flannel/subnet.env")
		if !strings.Contains(output, "FLANNEL_SUBNET") {
			// Show Flannel logs for debugging
			logs := nerdctlExec(ctx, hostName, "cat", "/var/log/flanneld.log")
			p.Error(fmt.Sprintf("Flannel logs on %s:\n%s", hostName, logs))
			return fmt.Errorf("Flannel failed to allocate subnet on %s", hostName)
		}

		p.Info(fmt.Sprintf("✓ Flannel started on %s", hostName))
		p.Info(fmt.Sprintf("  Subnet: %s", extractSubnet(output)))
	}

	return nil
}

func createAppContainer(ctx context.Context, p *helpers.Printer, hostName, appName string) (string, error) {
	p.Step(fmt.Sprintf("Creating container %s on %s", appName, hostName))

	// Create CNI directories
	nerdctlExec(ctx, hostName, "mkdir", "-p", "/etc/cni/net.d", "/opt/cni/bin", "/var/lib/cni/networks", "/var/run/netns")

	// Install CNI plugins (bridge, host-local, etc) - check if already installed
	output := nerdctlExec(ctx, hostName, "sh", "-c",
		"if [ ! -f /opt/cni/bin/bridge ]; then wget -q -O /tmp/cni.tgz https://github.com/containernetworking/plugins/releases/download/v1.3.0/cni-plugins-linux-amd64-v1.3.0.tgz && tar -xzf /tmp/cni.tgz -C /opt/cni/bin && rm /tmp/cni.tgz; fi && echo 'done'")
	if !strings.Contains(output, "done") {
		return "", fmt.Errorf("failed to install CNI plugins: %s", output)
	}

	// Install Flannel CNI plugin - check if already installed
	output = nerdctlExec(ctx, hostName, "sh", "-c",
		"if [ ! -f /opt/cni/bin/flannel ]; then wget -q -O /opt/cni/bin/flannel https://github.com/flannel-io/cni-plugin/releases/download/v1.2.0/flannel-amd64 && chmod +x /opt/cni/bin/flannel; fi && echo 'done'")
	if !strings.Contains(output, "done") {
		return "", fmt.Errorf("failed to install Flannel CNI plugin: %s", output)
	}

	// Create Flannel CNI config
	cniConfig := `{
  "name": "cbr0",
  "cniVersion": "0.3.1",
  "type": "flannel",
  "delegate": {
    "bridge": "cbr0",
    "hairpinMode": true,
    "isDefaultGateway": true
  }
}`
	nerdctlExec(ctx, hostName, "sh", "-c", fmt.Sprintf("cat > /etc/cni/net.d/10-flannel.conf <<'EOF'\n%s\nEOF", cniConfig))

	// Create container inside DinD host
	nerdctlExec(ctx, hostName, "docker", "run", "-d",
		"--name", appName,
		"--network=none",
		"alpine", "sleep", "3600")

	// Get container PID
	containerPID := strings.TrimSpace(nerdctlExec(ctx, hostName, "docker", "inspect", "-f", "{{.State.Pid}}", appName))
	if containerPID == "" || containerPID == "0" {
		return "", fmt.Errorf("failed to get container PID for %s", appName)
	}

	// Container network namespace is at /proc/<PID>/ns/net
	// But CNI expects a path in /var/run/netns/, so create a symlink
	netnsName := fmt.Sprintf("cni-%s", appName)
	netnsPath := fmt.Sprintf("/var/run/netns/%s", netnsName)
	nerdctlExec(ctx, hostName, "ln", "-sf", fmt.Sprintf("/proc/%s/ns/net", containerPID), netnsPath)

	// Get container ID
	containerID := strings.TrimSpace(nerdctlExec(ctx, hostName, "docker", "ps", "-q", "-f", fmt.Sprintf("name=%s", appName)))

	// Invoke CNI ADD to attach network
	cniEnv := fmt.Sprintf(`CNI_COMMAND=ADD CNI_CONTAINERID=%s CNI_NETNS=%s CNI_IFNAME=eth0 CNI_PATH=/opt/cni/bin`,
		containerID, netnsPath)
	output = nerdctlExec(ctx, hostName, "sh", "-c",
		fmt.Sprintf("%s /opt/cni/bin/flannel < /etc/cni/net.d/10-flannel.conf", cniEnv))

	// Check if CNI command succeeded
	if strings.Contains(output, "error") || strings.Contains(output, "failed") {
		return "", fmt.Errorf("CNI ADD failed: %s", output)
	}

	// Get container IP
	time.Sleep(2 * time.Second)
	ip := strings.TrimSpace(nerdctlExec(ctx, hostName, "docker", "exec", appName,
		"sh", "-c", "ip -4 addr show eth0 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1 || echo ''"))

	if ip == "" {
		return "", fmt.Errorf("failed to get IP for %s - eth0 interface not found", appName)
	}

	p.Info(fmt.Sprintf("✓ Container %s created with IP: %s", appName, ip))
	return ip, nil
}

func testCrossHostPing(ctx context.Context, p *helpers.Printer, hostName, containerName, sourceIP, targetIP string) error {
	p.Step(fmt.Sprintf("Testing ping from %s (%s) to %s", containerName, sourceIP, targetIP))

	// Ping from container
	output := nerdctlExec(ctx, hostName, "docker", "exec", containerName, "ping", "-c", "3", "-W", "5", targetIP)

	// Check for successful ping (Alpine format: "3 packets transmitted, 3 packets received")
	if !strings.Contains(output, "3 packets transmitted") || !strings.Contains(output, "3 packets received") {
		return fmt.Errorf("ping failed:\n%s", output)
	}

	// Also check for 0% packet loss
	if !strings.Contains(output, "0% packet loss") {
		return fmt.Errorf("ping had packet loss:\n%s", output)
	}

	p.Info(fmt.Sprintf("✓ Ping successful: %s -> %s", sourceIP, targetIP))
	return nil
}

func verifyEtcdState(ctx context.Context, p *helpers.Printer) error {
	p.Step("Verifying etcd state")

	// Check Flannel subnet leases
	cmd := exec.CommandContext(ctx, "etcdctl",
		"--endpoints="+etcdEndpointHost,
		"get", "/coreos.com/network/subnets/", "--prefix")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to query etcd: %w", err)
	}

	outputStr := string(output)
	
	// Check that we have at least 2 subnet allocations (for 2 hosts)
	// Flannel creates keys like /coreos.com/network/subnets/10.0.45.0-24
	subnetCount := strings.Count(outputStr, "/coreos.com/network/subnets/10.0.")
	if subnetCount < 2 {
		return fmt.Errorf("expected at least 2 subnet allocations, found %d in etcd:\n%s", subnetCount, outputStr)
	}

	// Verify both subnets are from our VPC CIDR (10.0.0.0/16)
	if !strings.Contains(outputStr, "10.0.") {
		return fmt.Errorf("subnet allocations not found in expected VPC CIDR range")
	}

	p.Info("✓ etcd state verified")
	p.Info(fmt.Sprintf("  Found %d subnet lease(s) in etcd", subnetCount))
	
	// Show first few lines of subnet leases
	lines := strings.Split(outputStr, "\n")
	if len(lines) > 0 {
		p.Info("  Sample subnet leases:")
		for i := 0; i < len(lines) && i < 6; i++ {
			if strings.TrimSpace(lines[i]) != "" {
				p.Info(fmt.Sprintf("    %s", lines[i]))
			}
		}
	}
	
	return nil
}

func cleanup(ctx context.Context, p *helpers.Printer) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Cleanup")
	fmt.Println(strings.Repeat("=", 60))

	// Stop containers inside containerd hosts
	nerdctlExec(ctx, host1Name, "docker", "stop", app1Name)
	nerdctlExec(ctx, host1Name, "docker", "rm", app1Name)
	nerdctlExec(ctx, host2Name, "docker", "stop", app2Name)
	nerdctlExec(ctx, host2Name, "docker", "rm", app2Name)

	// Stop and remove containerd hosts
	exec.CommandContext(ctx, "sudo", "nerdctl", "stop", host1Name, host2Name).Run()
	exec.CommandContext(ctx, "sudo", "nerdctl", "rm", host1Name, host2Name).Run()

	// Remove test network
	exec.CommandContext(ctx, "sudo", "nerdctl", "network", "rm", testNetworkName).Run()

	// Stop etcd
	exec.CommandContext(ctx, "/tmp/vpc-cli", "etcd", "stop").Run()

	// Clean up data
	exec.CommandContext(ctx, "rm", "-rf", "/tmp/test-etcd").Run()

	p.Info("✓ Cleanup completed")
}

// Helper function to execute command in containerd container
func nerdctlExec(ctx context.Context, containerName string, args ...string) string {
	fullArgs := append([]string{"nerdctl", "exec", containerName}, args...)
	cmd := exec.CommandContext(ctx, "sudo", fullArgs...)
	output, _ := cmd.CombinedOutput()
	return string(output)
}

// Helper to extract subnet from Flannel subnet.env
func extractSubnet(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "FLANNEL_SUBNET=") {
			return strings.TrimPrefix(line, "FLANNEL_SUBNET=")
		}
	}
	return "unknown"
}
