package dns

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"sync"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/vpc"
)

// hostnameRegex validates DNS hostnames (RFC 1123)
// Allows: alphanumeric, hyphens, dots. No underscores, spaces, or other special chars.
var hostnameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-.]*[a-zA-Z0-9])?$|^[a-zA-Z0-9]$`)

// Manager implements vpc.DNSManager interface
type Manager struct {
	store     storage.StateStore
	hostsFile string
	mu        sync.RWMutex
}

// dnsEntry represents a DNS entry in storage
type dnsEntry struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
	Healthy  bool   `json:"healthy"`
}

// NewManager creates a new DNS manager with in-memory storage
func NewManager() *Manager {
	return &Manager{
		store:     storage.NewMemoryStore(),
		hostsFile: "/etc/hosts",
	}
}

// NewManagerWithStore creates a new DNS manager with custom storage
func NewManagerWithStore(store storage.StateStore) *Manager {
	return &Manager{
		store:     store,
		hostsFile: "/etc/hosts",
	}
}

// SetHostsFile sets the hosts file path (for testing)
func (m *Manager) SetHostsFile(path string) {
	m.hostsFile = path
}

// RegisterHost registers a hostname with an IP
func (m *Manager) RegisterHost(ctx context.Context, hostname string, ip net.IP) error {
	// Validate hostname
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}
	if !isValidHostname(hostname) {
		return fmt.Errorf("invalid hostname: %s", hostname)
	}

	// Validate IP
	if ip == nil {
		return fmt.Errorf("IP cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Save to store with health=true by default
	key := fmt.Sprintf("dns/hosts/%s/%s", hostname, ip.String())
	entry := dnsEntry{
		Hostname: hostname,
		IP:       ip.String(),
		Healthy:  true,
	}

	if err := m.store.Save(ctx, key, entry); err != nil {
		return fmt.Errorf("failed to save DNS entry: %w", err)
	}

	return nil
}

// UnregisterHost removes a hostname registration
func (m *Manager) UnregisterHost(ctx context.Context, hostname string) error {
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if hostname exists
	prefix := fmt.Sprintf("dns/hosts/%s/", hostname)
	keys, err := m.store.List(ctx, prefix)
	if err != nil {
		return fmt.Errorf("failed to list DNS entries: %w", err)
	}

	if len(keys) == 0 {
		return fmt.Errorf("hostname not found: %s", hostname)
	}

	// Delete all IPs for this hostname
	for _, key := range keys {
		if err := m.store.Delete(ctx, key); err != nil {
			return fmt.Errorf("failed to delete DNS entry: %w", err)
		}
	}

	return nil
}

// LookupHost resolves a hostname to IPs
func (m *Manager) LookupHost(ctx context.Context, hostname string) ([]net.IP, error) {
	if hostname == "" {
		return nil, fmt.Errorf("hostname cannot be empty")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Get all IPs for hostname
	prefix := fmt.Sprintf("dns/hosts/%s/", hostname)
	keys, err := m.store.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup host: %w", err)
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("host not found: %s", hostname)
	}

	var ips []net.IP
	for _, key := range keys {
		var entry dnsEntry
		if err := m.store.Get(ctx, key, &entry); err != nil {
			continue
		}

		// Only return healthy hosts
		if entry.Healthy {
			if ip := net.ParseIP(entry.IP); ip != nil {
				ips = append(ips, ip)
			}
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no healthy hosts for: %s", hostname)
	}

	return ips, nil
}

// UpdateHealth updates the health status of a host
func (m *Manager) UpdateHealth(ctx context.Context, hostname string, healthy bool) error {
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Get all IPs for hostname
	prefix := fmt.Sprintf("dns/hosts/%s/", hostname)
	keys, err := m.store.List(ctx, prefix)
	if err != nil {
		return fmt.Errorf("failed to list DNS entries: %w", err)
	}

	if len(keys) == 0 {
		return fmt.Errorf("hostname not found: %s", hostname)
	}

	// Update all IPs for this hostname
	for _, key := range keys {
		var entry dnsEntry
		if err := m.store.Get(ctx, key, &entry); err != nil {
			continue
		}

		entry.Healthy = healthy
		if err := m.store.Save(ctx, key, entry); err != nil {
			return fmt.Errorf("failed to update health: %w", err)
		}
	}

	return nil
}

// isValidHostname validates a hostname according to RFC 1123
// Allows alphanumeric characters, hyphens, and dots
// No underscores, spaces, or other special characters
func isValidHostname(hostname string) bool {
	if hostname == "" {
		return false
	}

	// Check for invalid characters (spaces, underscores, etc.)
	for _, c := range hostname {
		if c == ' ' || c == '_' {
			return false
		}
	}

	// Must match the hostname pattern
	return hostnameRegex.MatchString(hostname)
}

// Ensure Manager implements DNSManager interface
var _ vpc.DNSManager = (*Manager)(nil)
