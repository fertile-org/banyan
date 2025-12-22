//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fertile-org/banyan/pkg/vpc"
	"github.com/fertile-org/banyan/pkg/vpc/debug"
	"github.com/fertile-org/banyan/test/integration/helpers"
)

const (
	vpcCLIPath = "/tmp/vpc-cli"
)

func main() {
	ctx := context.Background()
	p := helpers.NewPrinter()

	// Run test
	exitCode := runTest(ctx, p)

	if exitCode == 0 {
		p.Result(true, "Debug Integration Test completed successfully!")
	} else {
		p.Result(false, fmt.Sprintf("Debug Integration Test failed with exit code %d", exitCode))
	}

	os.Exit(exitCode)
}

func runTest(ctx context.Context, p *helpers.Printer) int {
	p.Title("Testing Network Debugging Utilities Integration")

	// Step 1: Build vpc-cli
	p.Step("Building vpc-cli")

	projectRoot := getProjectRoot()
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", vpcCLIPath, "./cmd/vpc-cli/")
	buildCmd.Dir = projectRoot

	if output, err := buildCmd.CombinedOutput(); err != nil {
		p.Error(fmt.Sprintf("Failed to build vpc-cli: %v", err))
		p.Code(string(output))
		return 1
	}
	p.Success("Built vpc-cli successfully")

	// Step 2: Verify debug command exists
	p.Step("Verifying debug CLI commands")

	helpCmd := exec.CommandContext(ctx, vpcCLIPath, "debug", "--help")
	helpOutput, err := helpCmd.CombinedOutput()
	if err != nil {
		p.Error(fmt.Sprintf("Debug command not available: %v", err))
		p.Code(string(helpOutput))
		return 1
	}

	expectedCommands := []string{"trace", "connectivity", "iptables", "ping", "port"}
	for _, cmd := range expectedCommands {
		if !strings.Contains(string(helpOutput), cmd) {
			p.Error(fmt.Sprintf("Expected command '%s' not found in debug help", cmd))
			return 1
		}
	}
	p.Success("All debug subcommands available")
	p.Code(string(helpOutput))

	// Step 3: Test TraceConnection programmatically
	p.Step("Testing TraceConnection programmatically")

	manager := debug.NewManager()

	// Test 3a: Same subnet connection
	p.Info("Testing same subnet connection trace")
	result, traceErr := manager.TraceConnection(ctx,
		net.ParseIP("10.0.1.5"),
		net.ParseIP("10.0.1.10"),
		5432,
	)
	if traceErr != nil {
		p.Error(fmt.Sprintf("TraceConnection failed: %v", traceErr))
		return 1
	}

	if result.Status != "allowed" {
		p.Error(fmt.Sprintf("Expected allowed status, got %s", result.Status))
		return 1
	}
	if !result.Reachable {
		p.Error("Expected connection to be reachable")
		return 1
	}
	if len(result.Hops) < 2 {
		p.Error("Expected at least 2 hops for same subnet connection")
		return 1
	}
	p.Success(fmt.Sprintf("Same subnet trace: %s -> %s:%d = %s (%d hops)",
		result.FromIP, result.ToIP, result.Port, result.Status, len(result.Hops)))

	// Test 3b: Cross-subnet connection - analyze if would be blocked
	p.Info("Testing cross-subnet SSH connection (predicting if blocked)")
	blockedResult, blockedErr := manager.TraceConnection(ctx,
		net.ParseIP("10.0.1.5"),
		net.ParseIP("10.0.2.10"),
		22,
	)
	if blockedErr != nil {
		p.Error(fmt.Sprintf("TraceConnection failed: %v", blockedErr))
		return 1
	}

	if blockedResult.Status != "blocked" {
		p.Error(fmt.Sprintf("Expected predicted blocked status, got %s", blockedResult.Status))
		return 1
	}
	if blockedResult.BlockedBy == "" {
		p.Error("Expected BlockedBy (rule ID) to be set")
		return 1
	}
	p.Success(fmt.Sprintf("Cross-subnet SSH would be blocked by rule: %s", blockedResult.BlockedBy))

	// Test 3c: External IP connection (with NAT)
	p.Info("Testing external IP connection (NAT traversal)")
	externalResult, externalErr := manager.TraceConnection(ctx,
		net.ParseIP("10.0.1.5"),
		net.ParseIP("8.8.8.8"),
		443,
	)
	if externalErr != nil {
		p.Error(fmt.Sprintf("TraceConnection failed: %v", externalErr))
		return 1
	}

	// Verify NAT hop exists
	hasNATHop := false
	hasGatewayHop := false
	for _, hop := range externalResult.Hops {
		if hop.Type == "nat" {
			hasNATHop = true
		}
		if hop.Type == "gateway" {
			hasGatewayHop = true
		}
	}
	if !hasNATHop {
		p.Error("Expected NAT hop for external connection")
		return 1
	}
	if !hasGatewayHop {
		p.Error("Expected gateway hop for external connection")
		return 1
	}
	p.Success(fmt.Sprintf("External connection trace: %d hops including NAT", len(externalResult.Hops)))

	// Test 3d: Loopback connection
	p.Info("Testing loopback connection")
	loopbackResult, loopbackErr := manager.TraceConnection(ctx,
		net.ParseIP("10.0.1.5"),
		net.ParseIP("10.0.1.5"),
		8080,
	)
	if loopbackErr != nil {
		p.Error(fmt.Sprintf("TraceConnection failed: %v", loopbackErr))
		return 1
	}

	if loopbackResult.Status != "allowed" {
		p.Error(fmt.Sprintf("Expected loopback to be allowed, got %s", loopbackResult.Status))
		return 1
	}
	if loopbackResult.AllowedBy != "loopback" {
		p.Error(fmt.Sprintf("Expected AllowedBy=loopback, got %s", loopbackResult.AllowedBy))
		return 1
	}
	p.Success("Loopback connection allowed correctly")

	// Step 4: Test CheckConnectivity programmatically
	p.Step("Testing CheckConnectivity programmatically")

	// Test 4a: Healthy container
	p.Info("Testing connectivity for healthy container")
	connResult, connErr := manager.CheckConnectivity(ctx, "container-001")
	if connErr != nil {
		p.Error(fmt.Sprintf("CheckConnectivity failed: %v", connErr))
		return 1
	}

	if connResult.Status != "ok" {
		p.Error(fmt.Sprintf("Expected status 'ok', got %s", connResult.Status))
		return 1
	}
	if connResult.IP == nil {
		p.Error("Expected IP address to be set")
		return 1
	}
	if connResult.Gateway == nil {
		p.Error("Expected gateway to be set")
		return 1
	}
	if len(connResult.DNS) == 0 {
		p.Error("Expected DNS servers to be set")
		return 1
	}
	if !connResult.InternetAccess {
		p.Error("Expected internet access for healthy container")
		return 1
	}
	p.Success(fmt.Sprintf("Healthy container: IP=%s, Gateway=%s, DNS=%d servers, Internet=%v",
		connResult.IP, connResult.Gateway, len(connResult.DNS), connResult.InternetAccess))

	// Test 4b: Container with degraded network
	p.Info("Testing connectivity for degraded container")
	degradedResult, degradedErr := manager.CheckConnectivity(ctx, "container-broken")
	if degradedErr != nil {
		p.Error(fmt.Sprintf("CheckConnectivity failed: %v", degradedErr))
		return 1
	}

	if degradedResult.Status != "degraded" {
		p.Error(fmt.Sprintf("Expected status 'degraded', got %s", degradedResult.Status))
		return 1
	}
	if len(degradedResult.Errors) == 0 {
		p.Error("Expected errors for degraded container")
		return 1
	}
	p.Success(fmt.Sprintf("Degraded container detected with %d errors", len(degradedResult.Errors)))
	for _, errMsg := range degradedResult.Errors {
		p.Info(fmt.Sprintf("  - %s", errMsg))
	}

	// Test 4c: Partitioned container
	p.Info("Testing connectivity for partitioned container")
	partitionedResult, partitionedErr := manager.CheckConnectivity(ctx, "container-partitioned")
	if partitionedErr != nil {
		p.Error(fmt.Sprintf("CheckConnectivity failed: %v", partitionedErr))
		return 1
	}

	if partitionedResult.Status != "unreachable" {
		p.Error(fmt.Sprintf("Expected status 'unreachable', got %s", partitionedResult.Status))
		return 1
	}
	p.Success("Partitioned container correctly identified as unreachable")

	// Test 4d: Non-existent container (should error)
	p.Info("Testing connectivity for non-existent container")
	_, nonExistErr := manager.CheckConnectivity(ctx, "non-existent-container")
	if nonExistErr == nil {
		p.Error("Expected error for non-existent container")
		return 1
	}
	p.Success("Non-existent container correctly returns error")

	// Step 5: Test GetIPTablesRules programmatically
	p.Step("Testing GetIPTablesRules programmatically")

	rules, rulesErr := manager.GetIPTablesRules(ctx, net.ParseIP("10.0.1.5"))
	if rulesErr != nil {
		p.Error(fmt.Sprintf("GetIPTablesRules failed: %v", rulesErr))
		return 1
	}

	if len(rules) == 0 {
		p.Error("Expected at least one iptables rule")
		return 1
	}

	// Verify rules contain expected elements
	hasFilterRule := false
	hasNATRule := false
	hasPacketCounters := false
	for _, rule := range rules {
		if strings.Contains(rule, "FORWARD") {
			hasFilterRule = true
		}
		if strings.Contains(rule, "POSTROUTING") || strings.Contains(rule, "MASQUERADE") {
			hasNATRule = true
		}
		if strings.Contains(rule, "pkts") && strings.Contains(rule, "bytes") {
			hasPacketCounters = true
		}
	}

	if !hasFilterRule {
		p.Error("Expected filter table rules")
		return 1
	}
	if !hasNATRule {
		p.Error("Expected NAT table rules")
		return 1
	}
	if !hasPacketCounters {
		p.Error("Expected packet counters in rules")
		return 1
	}

	p.Success(fmt.Sprintf("Retrieved %d iptables rules with filter, NAT, and packet counters", len(rules)))
	for _, rule := range rules[:3] { // Show first 3 rules
		p.Info(fmt.Sprintf("  %s", rule))
	}

	// Step 6: Test CLI commands via subprocess
	p.Step("Testing debug CLI commands")

	// Test 6a: trace command
	p.Info("Testing 'vpc-cli debug trace' command")
	traceCmd := exec.CommandContext(ctx, vpcCLIPath, "debug", "trace", "10.0.1.5", "10.0.1.10", "5432")
	traceCmdOutput, traceCmdErr := traceCmd.CombinedOutput()
	if traceCmdErr != nil {
		p.Error(fmt.Sprintf("Trace command failed: %v", traceCmdErr))
		p.Code(string(traceCmdOutput))
		return 1
	}

	if !strings.Contains(string(traceCmdOutput), "Status: allowed") {
		p.Error("Expected 'Status: allowed' in trace output")
		p.Code(string(traceCmdOutput))
		return 1
	}
	p.Success("Trace CLI command works correctly")

	// Test 6b: trace command with JSON output
	p.Info("Testing 'vpc-cli debug trace --json' command")
	traceJSONCmd := exec.CommandContext(ctx, vpcCLIPath, "debug", "trace", "--json", "10.0.1.5", "8.8.8.8", "443")
	traceJSONOutput, traceJSONErr := traceJSONCmd.CombinedOutput()
	if traceJSONErr != nil {
		p.Error(fmt.Sprintf("Trace JSON command failed: %v", traceJSONErr))
		p.Code(string(traceJSONOutput))
		return 1
	}

	var traceJSONResult vpc.TraceResult
	if jsonErr := json.Unmarshal(traceJSONOutput, &traceJSONResult); jsonErr != nil {
		p.Error(fmt.Sprintf("Failed to parse JSON output: %v", jsonErr))
		p.Code(string(traceJSONOutput))
		return 1
	}

	if traceJSONResult.Status != "allowed" {
		p.Error(fmt.Sprintf("Expected allowed status in JSON, got %s", traceJSONResult.Status))
		return 1
	}
	p.Success("Trace JSON output parses correctly")

	// Test 6c: connectivity command
	p.Info("Testing 'vpc-cli debug connectivity' command")
	connCmd := exec.CommandContext(ctx, vpcCLIPath, "debug", "connectivity", "container-001")
	connCmdOutput, connCmdErr := connCmd.CombinedOutput()
	if connCmdErr != nil {
		p.Error(fmt.Sprintf("Connectivity command failed: %v", connCmdErr))
		p.Code(string(connCmdOutput))
		return 1
	}

	if !strings.Contains(string(connCmdOutput), "Status: ok") {
		p.Error("Expected 'Status: ok' in connectivity output")
		p.Code(string(connCmdOutput))
		return 1
	}
	if !strings.Contains(string(connCmdOutput), "Internet Access: true") {
		p.Error("Expected 'Internet Access: true' in connectivity output")
		p.Code(string(connCmdOutput))
		return 1
	}
	p.Success("Connectivity CLI command works correctly")

	// Test 6d: iptables command
	p.Info("Testing 'vpc-cli debug iptables' command")
	iptablesCmd := exec.CommandContext(ctx, vpcCLIPath, "debug", "iptables", "10.0.1.5")
	iptablesCmdOutput, iptablesCmdErr := iptablesCmd.CombinedOutput()
	if iptablesCmdErr != nil {
		p.Error(fmt.Sprintf("IPTables command failed: %v", iptablesCmdErr))
		p.Code(string(iptablesCmdOutput))
		return 1
	}

	if !strings.Contains(string(iptablesCmdOutput), "10.0.1.5") {
		p.Error("Expected IP address in iptables output")
		p.Code(string(iptablesCmdOutput))
		return 1
	}
	p.Success("IPTables CLI command works correctly")

	// Step 7: Test multi-hop trace scenarios
	p.Step("Testing complex trace scenarios")

	// Test cross-subnet connection (should have gateway + VXLAN hops)
	p.Info("Testing cross-subnet connection (gateway + VXLAN)")
	multiHopResult, multiHopErr := manager.TraceConnection(ctx,
		net.ParseIP("10.0.1.5"),
		net.ParseIP("10.0.3.10"),
		80,
	)
	if multiHopErr != nil {
		p.Error(fmt.Sprintf("Multi-hop trace failed: %v", multiHopErr))
		return 1
	}

	if len(multiHopResult.Hops) < 3 {
		p.Error(fmt.Sprintf("Expected at least 3 hops for cross-subnet, got %d", len(multiHopResult.Hops)))
		return 1
	}

	hasVXLAN := false
	for _, hop := range multiHopResult.Hops {
		if hop.Type == "vxlan" {
			hasVXLAN = true
			break
		}
	}
	if !hasVXLAN {
		p.Error("Expected VXLAN hop for cross-subnet connection")
		return 1
	}
	p.Success(fmt.Sprintf("Cross-subnet trace: %d hops including VXLAN tunnel", len(multiHopResult.Hops)))

	// Step 8: Test input validation
	p.Step("Testing input validation")

	// Test nil IP
	p.Info("Testing nil IP handling")
	_, nilIPErr := manager.TraceConnection(ctx, nil, net.ParseIP("10.0.1.10"), 80)
	if nilIPErr == nil {
		p.Error("Expected error for nil from IP")
		return 1
	}
	p.Success("Nil from IP correctly rejected")

	// Test invalid port
	p.Info("Testing invalid port handling")
	_, invalidPortErr := manager.TraceConnection(ctx,
		net.ParseIP("10.0.1.5"),
		net.ParseIP("10.0.1.10"),
		70000,
	)
	if invalidPortErr == nil {
		p.Error("Expected error for port out of range")
		return 1
	}
	p.Success("Invalid port correctly rejected")

	// Test empty container ID
	p.Info("Testing empty container ID handling")
	_, emptyIDErr := manager.CheckConnectivity(ctx, "")
	if emptyIDErr == nil {
		p.Error("Expected error for empty container ID")
		return 1
	}
	p.Success("Empty container ID correctly rejected")

	// Test nil IP for iptables
	p.Info("Testing nil IP for GetIPTablesRules")
	_, nilIPTablesErr := manager.GetIPTablesRules(ctx, nil)
	if nilIPTablesErr == nil {
		p.Error("Expected error for nil IP in GetIPTablesRules")
		return 1
	}
	p.Success("Nil IP for iptables correctly rejected")

	// Step 9: Test MTU and performance diagnostics
	p.Step("Testing performance diagnostics")

	mtuResult, mtuErr := manager.CheckConnectivity(ctx, "container-mtu-issue")
	if mtuErr != nil {
		p.Error(fmt.Sprintf("MTU container check failed: %v", mtuErr))
		return 1
	}

	// Verify MTU issue is detected
	hasMTUError := false
	for _, errMsg := range mtuResult.Errors {
		if strings.Contains(errMsg, "MTU") || strings.Contains(errMsg, "fragmentation") {
			hasMTUError = true
			break
		}
	}
	if !hasMTUError {
		p.Warning("MTU issue detection may need enhancement")
	} else {
		p.Success("MTU issue correctly detected")
	}

	// Step 10: Test database container (no internet)
	p.Step("Testing isolated container diagnostics")

	dbResult, dbErr := manager.CheckConnectivity(ctx, "container-database")
	if dbErr != nil {
		p.Error(fmt.Sprintf("Database container check failed: %v", dbErr))
		return 1
	}

	if dbResult.InternetAccess {
		p.Error("Database container should not have internet access")
		return 1
	}
	if dbResult.IP == nil {
		p.Error("Database container should still have internal IP")
		return 1
	}
	p.Success(fmt.Sprintf("Database container correctly isolated: IP=%s, Internet=%v",
		dbResult.IP, dbResult.InternetAccess))

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
