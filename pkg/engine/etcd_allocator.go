package engine

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/fertile-org/banyan/pkg/storage"
)

// etcdSubnetAllocator allocates /24 subnets from a VPC CIDR using etcd for
// coordination. Used in multi-engine mode; single-engine uses the in-memory
// overlay.SubnetAllocator.
type etcdSubnetAllocator struct {
	vpcCIDR   *net.IPNet
	store     storage.StateStore
	lockStore storage.LockStore
	subnetLen int
}

const subnetKeyPrefix = "vpc/subnets/"

// newEtcdSubnetAllocator creates an etcd-backed subnet allocator.
func newEtcdSubnetAllocator(vpcCIDR string, store storage.StateStore, lockStore storage.LockStore) (*etcdSubnetAllocator, error) {
	_, cidr, err := net.ParseCIDR(vpcCIDR)
	if err != nil {
		return nil, fmt.Errorf("invalid VPC CIDR %q: %w", vpcCIDR, err)
	}

	ones, _ := cidr.Mask.Size()
	if ones >= 24 {
		return nil, fmt.Errorf("VPC CIDR %q is too small (must be larger than /24)", vpcCIDR)
	}

	return &etcdSubnetAllocator{
		vpcCIDR:   cidr,
		subnetLen: 24,
		store:     store,
		lockStore: lockStore,
	}, nil
}

// Allocate assigns a /24 subnet to the given agent. Idempotent.
func (a *etcdSubnetAllocator) Allocate(ctx context.Context, agentName string) (*net.IPNet, error) {
	// Idempotent check — return existing allocation
	var existing string
	if err := a.store.Get(ctx, subnetKeyPrefix+agentName, &existing); err == nil {
		_, subnet, parseErr := net.ParseCIDR(existing)
		if parseErr == nil {
			return subnet, nil
		}
	}

	// Lock, list used subnets, find available, save
	unlock, err := a.lockStore.Lock(ctx, "locks/subnet-allocator", 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("acquire subnet lock: %w", err)
	}
	defer unlock()

	used := a.listUsedSubnets(ctx)
	subnet := a.findAvailableSubnet(used)
	if subnet == nil {
		return nil, fmt.Errorf("no available /24 subnets in VPC CIDR %s", a.vpcCIDR)
	}

	cidrStr := subnet.String()
	if err := a.store.Save(ctx, subnetKeyPrefix+agentName, &cidrStr); err != nil {
		return nil, fmt.Errorf("save subnet allocation: %w", err)
	}
	return subnet, nil
}

// Release frees the subnet allocated to the given agent.
func (a *etcdSubnetAllocator) Release(ctx context.Context, agentName string) {
	_ = a.store.Delete(ctx, subnetKeyPrefix+agentName)
}

// GetAll returns all current allocations.
func (a *etcdSubnetAllocator) GetAll(ctx context.Context) map[string]*net.IPNet {
	keys, err := a.store.List(ctx, subnetKeyPrefix)
	if err != nil {
		return nil
	}

	result := make(map[string]*net.IPNet, len(keys))
	for _, key := range keys {
		var cidrStr string
		if err := a.store.Get(ctx, key, &cidrStr); err != nil {
			continue
		}
		_, subnet, parseErr := net.ParseCIDR(cidrStr)
		if parseErr != nil {
			continue
		}
		// Extract agent name from key (strip prefix)
		agentName := key[len(subnetKeyPrefix):]
		result[agentName] = subnet
	}
	return result
}

// listUsedSubnets returns all currently allocated subnet strings.
func (a *etcdSubnetAllocator) listUsedSubnets(ctx context.Context) map[string]bool {
	keys, err := a.store.List(ctx, subnetKeyPrefix)
	if err != nil {
		return nil
	}

	used := make(map[string]bool, len(keys))
	for _, key := range keys {
		var cidrStr string
		if err := a.store.Get(ctx, key, &cidrStr); err != nil {
			continue
		}
		used[cidrStr] = true
	}
	return used
}

// findAvailableSubnet iterates through /24 blocks and returns the first unused one.
func (a *etcdSubnetAllocator) findAvailableSubnet(used map[string]bool) *net.IPNet {
	baseIP := a.vpcCIDR.IP.To4()
	if baseIP == nil {
		return nil
	}

	base := binary.BigEndian.Uint32(baseIP)
	ones, _ := a.vpcCIDR.Mask.Size()
	totalBlocks := 1 << uint(a.subnetLen-ones) //nolint:gosec // subnetLen and ones are bounded by CIDR validation

	for i := range totalBlocks {
		blockIP := make(net.IP, 4)
		binary.BigEndian.PutUint32(blockIP, base+uint32(i)<<8) //nolint:gosec // i is bounded by totalBlocks

		subnet := &net.IPNet{
			IP:   blockIP,
			Mask: net.CIDRMask(a.subnetLen, 32),
		}

		if !used[subnet.String()] {
			return subnet
		}
	}

	return nil
}
