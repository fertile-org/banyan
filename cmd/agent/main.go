package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/fertile-org/banyan/internal/common"
	"github.com/fertile-org/banyan/pkg/vpc/cni"
	"github.com/fertile-org/banyan/pkg/vpc/dns"
	"github.com/fertile-org/banyan/pkg/vpc/ipam"
	"github.com/fertile-org/banyan/pkg/vpc/security"
	"github.com/fertile-org/banyan/pkg/vpc/storage"
)

var (
	etcdEndpoints = flag.String("etcd", "http://localhost:2379", "Etcd endpoints (comma-separated)")
	hostID        = flag.String("host-id", "", "Unique host identifier (default: hostname)")
	vpcCIDR       = flag.String("vpc-cidr", "10.0.0.0/16", "VPC CIDR range")
)

func main() {
	flag.Parse()

	fmt.Printf("Banyan Agent %s\n", common.Version)
	fmt.Println("========================================")

	// Get host ID
	if *hostID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			fmt.Printf("Failed to get hostname: %v\n", err)
			os.Exit(1)
		}
		*hostID = hostname
	}

	fmt.Printf("Host ID: %s\n", *hostID)
	fmt.Printf("System: %s/%s, CPUs: %d\n", runtime.GOOS, runtime.GOARCH, runtime.NumCPU())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Initialize storage (etcd)
	fmt.Printf("Connecting to etcd at %s...\n", *etcdEndpoints)
	store, err := storage.NewEtcdStore([]string{*etcdEndpoints}, "/banyan")
	if err != nil {
		fmt.Printf("Failed to connect to etcd: %v\n", err)
		fmt.Println("Hint: Start the Engine first, which will initialize etcd")
		cancel()
		return
	}
	fmt.Println("Connected to etcd")

	// Initialize VPC components
	fmt.Println("Initializing VPC components...")

	ipamManager, err := ipam.NewManager(store, *vpcCIDR)
	if err != nil {
		fmt.Printf("Failed to initialize IPAM: %v\n", err)
		cancel()
		return
	}

	dnsManager := dns.NewManagerWithStore(store)
	resolver := security.NewRuntimeServiceResolver(store)
	securityManager := security.NewManager(resolver, false)
	cniRuntime := cni.NewRuntime(store, securityManager)

	fmt.Println("VPC components initialized: IPAM, DNS, Security, CNI")

	// Allocate subnet for this host
	fmt.Printf("Allocating subnet for host %s...\n", *hostID)
	subnet, err := ipamManager.AllocateHostSubnet(ctx, *hostID)
	if err != nil {
		// Try to get existing subnet
		subnet, err = ipamManager.GetHostSubnet(ctx, *hostID)
		if err != nil {
			fmt.Printf("Failed to allocate/get subnet: %v\n", err)
			cancel()
			return
		}
		fmt.Printf("Using existing subnet: %s\n", subnet.String())
	} else {
		fmt.Printf("Allocated new subnet: %s\n", subnet.String())
	}

	// Start lease renewal goroutine
	go renewLeaseLoop(ctx, ipamManager, *hostID)

	// Print agent status
	fmt.Println("========================================")
	fmt.Printf("Agent Status: READY\n")
	fmt.Printf("Host ID:      %s\n", *hostID)
	fmt.Printf("Subnet:       %s\n", subnet.String())
	fmt.Printf("etcd:         %s\n", *etcdEndpoints)
	fmt.Println("========================================")

	// Show capabilities
	fmt.Println("Capabilities:")
	fmt.Println("  - Container Runtime: containerd")
	fmt.Println("  - Network: Flannel CNI")
	fmt.Println("  - IPAM: VPC IPAM")
	fmt.Println("  - DNS: VPC DNS")
	fmt.Println("")
	fmt.Println("Agent is running. Press Ctrl+C to stop")
	fmt.Println("")

	// Demo: allocate an IP and register DNS
	demoNetworkOperations(ctx, ipamManager, dnsManager, cniRuntime, subnet, *hostID)

	// Keep running until shutdown
	<-ctx.Done()

	// Cleanup
	fmt.Println("Cleaning up...")
	fmt.Println("Agent stopped")
}

func renewLeaseLoop(ctx context.Context, ipamManager *ipam.Manager, hostID string) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := ipamManager.RenewLease(ctx, hostID); err != nil {
				fmt.Printf("Warning: Failed to renew lease: %v\n", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func demoNetworkOperations(ctx context.Context,
	ipamManager *ipam.Manager,
	dnsManager *dns.Manager,
	cniRuntime *cni.Runtime,
	subnet *net.IPNet,
	hostID string) {

	fmt.Println("Demo: Testing network operations...")

	// Allocate an IP
	ip, err := ipamManager.AllocateIP(ctx, subnet)
	if err != nil {
		fmt.Printf("  IPAM allocate: FAILED (%v)\n", err)
	} else {
		fmt.Printf("  IPAM allocate: OK (got %s)\n", ip.String())

		// Register DNS
		hostname := fmt.Sprintf("agent-%s.internal", hostID)
		if err := dnsManager.RegisterHost(ctx, hostname, ip); err != nil {
			fmt.Printf("  DNS register: FAILED (%v)\n", err)
		} else {
			fmt.Printf("  DNS register: OK (%s -> %s)\n", hostname, ip.String())
		}

		// Lookup DNS
		ips, err := dnsManager.LookupHost(ctx, hostname)
		if err != nil {
			fmt.Printf("  DNS lookup: FAILED (%v)\n", err)
		} else {
			fmt.Printf("  DNS lookup: OK (%s -> %v)\n", hostname, ips)
		}

		// Check CNI plugin status
		status, err := cniRuntime.GetPluginStatus(ctx, "flannel")
		if err != nil {
			fmt.Printf("  CNI status: FAILED (%v)\n", err)
		} else {
			fmt.Printf("  CNI status: OK (%s)\n", status.Status)
		}

		// Cleanup demo resources
		_ = dnsManager.UnregisterHost(ctx, hostname)
		_ = ipamManager.ReleaseIP(ctx, ip)
	}

	fmt.Println("")
}
