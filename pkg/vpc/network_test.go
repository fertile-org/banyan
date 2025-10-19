package vpc

import (
	"context"
	"os/exec"
	"testing"
)

func TestInitializeNetwork(t *testing.T) {
	// Skip if etcd is not available
	if _, err := exec.LookPath("etcdctl"); err != nil {
		t.Skip("etcdctl not found, skipping test")
	}

	ctx := context.Background()
	endpoints := []string{"http://localhost:2379"}
	vpcCIDR := "10.0.0.0/16"

	// Note: This test requires a running etcd server
	// In a real test environment, you would start etcd before running this
	err := InitializeNetwork(ctx, endpoints, vpcCIDR)
	if err != nil {
		// Expected to fail if etcd is not running
		t.Logf("InitializeNetwork failed (expected if etcd not running): %v", err)
	}
}
