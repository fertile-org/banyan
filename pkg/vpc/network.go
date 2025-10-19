package vpc

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// InitializeNetwork configures the Flannel network in etcd
// This writes the network configuration that Flannel daemons will read
func InitializeNetwork(ctx context.Context, etcdEndpoints []string, vpcCIDR string) error {
	// Create Flannel network config JSON
	config := fmt.Sprintf(`{"Network": "%s", "SubnetLen": 24, "Backend": {"Type": "vxlan"}}`, vpcCIDR)
	
	// Use etcdctl command to write config (same as working test)
	// This ensures compatibility with Flannel's expectations
	args := []string{
		"--endpoints=" + strings.Join(etcdEndpoints, ","),
		"put", "/coreos.com/network/config", config,
	}
	
	cmd := exec.CommandContext(ctx, "etcdctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to write Flannel config to etcd: %w\nOutput: %s", err, output)
	}
	
	return nil
}

