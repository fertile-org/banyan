package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var etcdCmd = &cobra.Command{
	Use:   "etcd",
	Short: "Manage etcd server for VPC state coordination",
	Long: `Manage etcd server lifecycle for distributed VPC state management.

The etcd server is used by the Banyan Engine to coordinate state across
multiple hosts. Agents connect to this central etcd server as clients.`,
}

var etcdStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start etcd server",
	Long: `Start etcd server for VPC state management.

This command starts a single-node etcd server. The Banyan Engine typically
runs this command to provide centralized state storage for all VPC agents.`,
	RunE: runEtcdStart,
}

var etcdStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop etcd server",
	Long:  `Stop the running etcd server gracefully.`,
	RunE:  runEtcdStop,
}

var etcdStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check etcd server status",
	Long:  `Check the health and status of the etcd server.`,
	RunE:  runEtcdStatus,
}

var etcdSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Download and install etcd binaries",
	Long: `Download and install etcd and etcdctl binaries for the current platform.

Supports Linux and macOS. The binaries will be installed to /usr/local/bin.
Requires sudo privileges for installation.`,
	RunE: runEtcdSetup,
}

// Flags for etcd start
var (
	etcdDataDir            string
	etcdListenClientURLs   string
	etcdAdvertiseClientURLs string
	etcdName               string
	etcdPidFile            string
	etcdLogFile            string
)

// Flags for etcd status
var (
	etcdEndpoints string
)

func init() {
	rootCmd.AddCommand(etcdCmd)
	etcdCmd.AddCommand(etcdSetupCmd)
	etcdCmd.AddCommand(etcdStartCmd)
	etcdCmd.AddCommand(etcdStopCmd)
	etcdCmd.AddCommand(etcdStatusCmd)

	// Start command flags
	etcdStartCmd.Flags().StringVar(&etcdDataDir, "data-dir", "/var/lib/banyan/etcd", "Data directory for etcd")
	etcdStartCmd.Flags().StringVar(&etcdListenClientURLs, "listen-client-urls", "http://0.0.0.0:2379", "Client URLs to listen on")
	etcdStartCmd.Flags().StringVar(&etcdAdvertiseClientURLs, "advertise-client-urls", "http://localhost:2379", "Client URLs to advertise")
	etcdStartCmd.Flags().StringVar(&etcdName, "name", "banyan-etcd", "Etcd member name")
	etcdStartCmd.Flags().StringVar(&etcdPidFile, "pid-file", "/var/run/banyan-etcd.pid", "PID file location")
	etcdStartCmd.Flags().StringVar(&etcdLogFile, "log-file", "/var/log/banyan-etcd.log", "Log file location")

	// Stop command flags
	etcdStopCmd.Flags().StringVar(&etcdPidFile, "pid-file", "/var/run/banyan-etcd.pid", "PID file location")

	// Status command flags
	etcdStatusCmd.Flags().StringVar(&etcdEndpoints, "endpoints", "http://localhost:2379", "Comma-separated etcd endpoints")
}

func runEtcdStart(cmd *cobra.Command, args []string) error {
	// Check if etcd is already running
	if isEtcdRunning() {
		return fmt.Errorf("etcd is already running (PID file exists: %s)", etcdPidFile)
	}

	// Check if etcd binary is available
	etcdBinary, err := exec.LookPath("etcd")
	if err != nil {
		return fmt.Errorf("etcd binary not found in PATH. Please install etcd: %w", err)
	}

	// Create data directory
	if err := os.MkdirAll(etcdDataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Create log file directory
	logDir := filepath.Dir(etcdLogFile)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Prepare etcd command
	etcdArgs := []string{
		"--name", etcdName,
		"--data-dir", etcdDataDir,
		"--listen-client-urls", etcdListenClientURLs,
		"--advertise-client-urls", etcdAdvertiseClientURLs,
		"--listen-peer-urls", "http://127.0.0.1:2380",
		"--initial-advertise-peer-urls", "http://127.0.0.1:2380",
		"--initial-cluster", fmt.Sprintf("%s=http://127.0.0.1:2380", etcdName),
		"--initial-cluster-token", "banyan-vpc-cluster",
		"--initial-cluster-state", "new",
	}

	// Open log file
	logFile, err := os.OpenFile(etcdLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	// Start etcd process
	fmt.Printf("Starting etcd server...\n")
	fmt.Printf("  Data directory: %s\n", etcdDataDir)
	fmt.Printf("  Client URLs: %s\n", etcdListenClientURLs)
	fmt.Printf("  Log file: %s\n", etcdLogFile)

	etcdProcess := exec.Command(etcdBinary, etcdArgs...)
	etcdProcess.Stdout = logFile
	etcdProcess.Stderr = logFile

	if err := etcdProcess.Start(); err != nil {
		return fmt.Errorf("failed to start etcd: %w", err)
	}

	// Write PID file
	pidDir := filepath.Dir(etcdPidFile)
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		etcdProcess.Process.Kill()
		return fmt.Errorf("failed to create PID directory: %w", err)
	}

	if err := os.WriteFile(etcdPidFile, []byte(fmt.Sprintf("%d", etcdProcess.Process.Pid)), 0644); err != nil {
		etcdProcess.Process.Kill()
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	fmt.Printf("etcd started with PID %d\n", etcdProcess.Process.Pid)

	// Wait a bit and verify it's running
	time.Sleep(2 * time.Second)

	// Check if process is still alive
	if err := etcdProcess.Process.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("etcd process died shortly after start. Check log file: %s", etcdLogFile)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{strings.Split(etcdAdvertiseClientURLs, ",")[0]},
		DialTimeout: 3 * time.Second,
	})
	if err != nil {
		fmt.Printf("Warning: Failed to connect to etcd for verification: %v\n", err)
		fmt.Printf("etcd is running but may not be ready yet. Check status with: banyan-cli etcd status\n")
		return nil
	}
	defer client.Close()

	// Test basic operation
	_, err = client.Status(ctx, strings.Split(etcdAdvertiseClientURLs, ",")[0])
	if err != nil {
		fmt.Printf("Warning: etcd health check failed: %v\n", err)
		fmt.Printf("etcd is running but may not be ready yet. Check status with: banyan-cli etcd status\n")
		return nil
	}

	fmt.Printf("✓ etcd server is running and healthy\n")
	fmt.Printf("  Endpoints: %s\n", etcdAdvertiseClientURLs)
	return nil
}

func runEtcdStop(cmd *cobra.Command, args []string) error {
	// Check if PID file exists
	if !isEtcdRunning() {
		return fmt.Errorf("etcd is not running (PID file not found: %s)", etcdPidFile)
	}

	// Read PID
	pidBytes, err := os.ReadFile(etcdPidFile)
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		return fmt.Errorf("invalid PID in file: %w", err)
	}

	// Find process
	process, err := os.FindProcess(pid)
	if err != nil {
		// PID file exists but process doesn't - clean up
		os.Remove(etcdPidFile)
		return fmt.Errorf("etcd process not found (stale PID file removed)")
	}

	fmt.Printf("Stopping etcd server (PID %d)...\n", pid)

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Process might already be dead
		os.Remove(etcdPidFile)
		return fmt.Errorf("failed to send SIGTERM: %w (PID file removed)", err)
	}

	// Wait up to 10 seconds for graceful shutdown
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		if err := process.Signal(syscall.Signal(0)); err != nil {
			// Process is dead
			os.Remove(etcdPidFile)
			fmt.Printf("✓ etcd server stopped successfully\n")
			return nil
		}
	}

	// Force kill if still running
	fmt.Printf("etcd didn't stop gracefully, forcing shutdown...\n")
	if err := process.Kill(); err != nil {
		return fmt.Errorf("failed to kill etcd process: %w", err)
	}

	os.Remove(etcdPidFile)
	fmt.Printf("✓ etcd server stopped (forced)\n")
	return nil
}

func runEtcdStatus(cmd *cobra.Command, args []string) error {
	endpoints := strings.Split(etcdEndpoints, ",")

	fmt.Printf("Checking etcd status...\n")
	fmt.Printf("  Endpoints: %s\n\n", strings.Join(endpoints, ", "))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 3 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to etcd: %w", err)
	}
	defer client.Close()

	// Check each endpoint
	for _, endpoint := range endpoints {
		fmt.Printf("Endpoint: %s\n", endpoint)

		statusResp, err := client.Status(ctx, endpoint)
		if err != nil {
			fmt.Printf("  Status: ✗ UNHEALTHY\n")
			fmt.Printf("  Error: %v\n\n", err)
			continue
		}

		fmt.Printf("  Status: ✓ HEALTHY\n")
		fmt.Printf("  Version: %s\n", statusResp.Version)
		fmt.Printf("  DB Size: %d bytes\n", statusResp.DbSize)
		fmt.Printf("  Leader: %t\n", statusResp.Leader == statusResp.Header.MemberId)
		fmt.Printf("  Raft Term: %d\n", statusResp.RaftTerm)
		fmt.Printf("  Raft Index: %d\n", statusResp.RaftIndex)
		fmt.Printf("\n")
	}

	// List members
	membersResp, err := client.MemberList(ctx)
	if err != nil {
		fmt.Printf("Failed to list members: %v\n", err)
		return nil
	}

	fmt.Printf("Cluster Members:\n")
	for _, member := range membersResp.Members {
		fmt.Printf("  - Name: %s\n", member.Name)
		fmt.Printf("    ID: %x\n", member.ID)
		fmt.Printf("    Client URLs: %s\n", strings.Join(member.ClientURLs, ", "))
		fmt.Printf("\n")
	}

	return nil
}

func runEtcdSetup(cmd *cobra.Command, args []string) error {
	const etcdVersion = "v3.5.11"

	// Detect OS and architecture
	goos := exec.Command("uname", "-s")
	goosOutput, err := goos.Output()
	if err != nil {
		return fmt.Errorf("failed to detect OS: %w", err)
	}
	osName := strings.ToLower(strings.TrimSpace(string(goosOutput)))

	goarch := exec.Command("uname", "-m")
	goarchOutput, err := goarch.Output()
	if err != nil {
		return fmt.Errorf("failed to detect architecture: %w", err)
	}
	arch := strings.TrimSpace(string(goarchOutput))

	// Map OS names
	var platform string
	switch osName {
	case "linux":
		platform = "linux"
	case "darwin":
		platform = "darwin"
	default:
		return fmt.Errorf("unsupported operating system: %s (only Linux and macOS are supported)", osName)
	}

	// Map architecture names
	var archName string
	switch arch {
	case "x86_64", "amd64":
		archName = "amd64"
	case "aarch64", "arm64":
		archName = "arm64"
	default:
		return fmt.Errorf("unsupported architecture: %s", arch)
	}

	// Build download URL
	filename := fmt.Sprintf("etcd-%s-%s-%s.tar.gz", etcdVersion, platform, archName)
	downloadURL := fmt.Sprintf("https://github.com/etcd-io/etcd/releases/download/%s/%s", etcdVersion, filename)

	fmt.Printf("📦 Downloading etcd %s for %s/%s...\n", etcdVersion, platform, archName)
	fmt.Printf("   URL: %s\n", downloadURL)

	// Download to /tmp
	tmpFile := filepath.Join("/tmp", filename)
	downloadCmd := exec.Command("wget", "-q", "-O", tmpFile, downloadURL)
	if err := downloadCmd.Run(); err != nil {
		// Try curl if wget fails
		downloadCmd = exec.Command("curl", "-sL", "-o", tmpFile, downloadURL)
		if err := downloadCmd.Run(); err != nil {
			return fmt.Errorf("failed to download etcd (tried wget and curl): %w", err)
		}
	}
	defer os.Remove(tmpFile)

	fmt.Println("✓ Download complete")

	// Extract to /tmp
	extractDir := filepath.Join("/tmp", fmt.Sprintf("etcd-%s-%s-%s", etcdVersion, platform, archName))
	os.RemoveAll(extractDir) // Clean up any existing directory

	fmt.Println("📂 Extracting archive...")
	extractCmd := exec.Command("tar", "xzf", tmpFile, "-C", "/tmp")
	if err := extractCmd.Run(); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}
	defer os.RemoveAll(extractDir)

	fmt.Println("✓ Extraction complete")

	// Install binaries to /usr/local/bin
	fmt.Println("📥 Installing binaries to /usr/local/bin...")

	etcdBin := filepath.Join(extractDir, "etcd")
	etcdctlBin := filepath.Join(extractDir, "etcdctl")

	// Copy etcd
	cpEtcd := exec.Command("sudo", "cp", etcdBin, "/usr/local/bin/etcd")
	if err := cpEtcd.Run(); err != nil {
		return fmt.Errorf("failed to install etcd: %w", err)
	}

	// Copy etcdctl
	cpEtcdctl := exec.Command("sudo", "cp", etcdctlBin, "/usr/local/bin/etcdctl")
	if err := cpEtcdctl.Run(); err != nil {
		return fmt.Errorf("failed to install etcdctl: %w", err)
	}

	// Make executable
	chmodEtcd := exec.Command("sudo", "chmod", "+x", "/usr/local/bin/etcd")
	chmodEtcd.Run()

	chmodEtcdctl := exec.Command("sudo", "chmod", "+x", "/usr/local/bin/etcdctl")
	chmodEtcdctl.Run()

	fmt.Println("✓ Installation complete")

	// Verify installation
	fmt.Println("\n🔍 Verifying installation...")

	verifyEtcd := exec.Command("etcd", "--version")
	if output, err := verifyEtcd.Output(); err == nil {
		fmt.Printf("✓ etcd: %s", string(output))
	}

	verifyEtcdctl := exec.Command("etcdctl", "version")
	if output, err := verifyEtcdctl.Output(); err == nil {
		fmt.Printf("✓ etcdctl: %s", string(output))
	}

	fmt.Println("\n✅ etcd installation successful!")
	return nil
}

func isEtcdRunning() bool {
	if _, err := os.Stat(etcdPidFile); os.IsNotExist(err) {
		return false
	}

	// Check if process with PID is actually running
	pidBytes, err := os.ReadFile(etcdPidFile)
	if err != nil {
		return false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Check if process is alive
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
