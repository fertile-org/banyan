package adapters

import (
	"context"
	"net"
	"sync"

	"github.com/fertile-org/banyan/pkg/engine/vpc/ports/outbound"
	"github.com/fertile-org/banyan/pkg/vpc/dns"
	"github.com/fertile-org/banyan/pkg/vpc/storage"
)

// VPCDNSManagerAdapter wraps the pkg/vpc DNSManager.
type VPCDNSManagerAdapter struct {
	manager *dns.Manager
}

// NewVPCDNSManagerAdapter creates a new VPCDNSManagerAdapter.
func NewVPCDNSManagerAdapter(store storage.StateStore) *VPCDNSManagerAdapter {
	return &VPCDNSManagerAdapter{
		manager: dns.NewManagerWithStore(store),
	}
}

// RegisterHost registers a hostname with an IP.
func (a *VPCDNSManagerAdapter) RegisterHost(ctx context.Context, hostname string, ip net.IP) error {
	return a.manager.RegisterHost(ctx, hostname, ip)
}

// UnregisterHost removes a hostname registration.
func (a *VPCDNSManagerAdapter) UnregisterHost(ctx context.Context, hostname string) error {
	return a.manager.UnregisterHost(ctx, hostname)
}

// LookupHost resolves a hostname to IPs.
func (a *VPCDNSManagerAdapter) LookupHost(ctx context.Context, hostname string) ([]net.IP, error) {
	return a.manager.LookupHost(ctx, hostname)
}

// UpdateHealth updates the health status of a host.
func (a *VPCDNSManagerAdapter) UpdateHealth(ctx context.Context, hostname string, healthy bool) error {
	return a.manager.UpdateHealth(ctx, hostname, healthy)
}

// Ensure VPCDNSManagerAdapter implements DNSManager.
var _ outbound.DNSManager = (*VPCDNSManagerAdapter)(nil)

// MemoryDNSManagerAdapter is an in-memory implementation for testing.
type MemoryDNSManagerAdapter struct {
	hosts  map[string][]net.IP
	health map[string]bool
	mu     sync.RWMutex
}

// NewMemoryDNSManagerAdapter creates a new in-memory DNS manager.
func NewMemoryDNSManagerAdapter() *MemoryDNSManagerAdapter {
	return &MemoryDNSManagerAdapter{
		hosts:  make(map[string][]net.IP),
		health: make(map[string]bool),
	}
}

// RegisterHost registers a hostname with an IP.
func (a *MemoryDNSManagerAdapter) RegisterHost(_ context.Context, hostname string, ip net.IP) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.hosts[hostname] = append(a.hosts[hostname], ip)
	a.health[hostname] = true
	return nil
}

// UnregisterHost removes a hostname registration.
func (a *MemoryDNSManagerAdapter) UnregisterHost(_ context.Context, hostname string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.hosts, hostname)
	delete(a.health, hostname)
	return nil
}

// LookupHost resolves a hostname to IPs.
func (a *MemoryDNSManagerAdapter) LookupHost(_ context.Context, hostname string) ([]net.IP, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	ips, ok := a.hosts[hostname]
	if !ok {
		return nil, nil
	}
	return ips, nil
}

// UpdateHealth updates the health status of a host.
func (a *MemoryDNSManagerAdapter) UpdateHealth(_ context.Context, hostname string, healthy bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.health[hostname] = healthy
	return nil
}

// Ensure MemoryDNSManagerAdapter implements DNSManager.
var _ outbound.DNSManager = (*MemoryDNSManagerAdapter)(nil)
