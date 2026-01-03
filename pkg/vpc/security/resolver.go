package security

import (
	"context"
	"fmt"
	"net"

	"github.com/fertile-org/banyan/pkg/vpc/registry"
	"github.com/fertile-org/banyan/pkg/vpc/storage"
)

// RuntimeServiceResolver resolves service names to IPs using the service registry
type RuntimeServiceResolver struct {
	store    storage.StateStore
	registry *registry.Registry
}

// NewRuntimeServiceResolver creates a new runtime-based service resolver
func NewRuntimeServiceResolver(store storage.StateStore) *RuntimeServiceResolver {
	return &RuntimeServiceResolver{
		store:    store,
		registry: registry.NewRegistry(store),
	}
}

// NewRuntimeServiceResolverWithRegistry creates a resolver with an existing registry
func NewRuntimeServiceResolverWithRegistry(store storage.StateStore, reg *registry.Registry) *RuntimeServiceResolver {
	return &RuntimeServiceResolver{
		store:    store,
		registry: reg,
	}
}

// ResolveServiceIPs resolves a service name to a list of IP addresses
func (r *RuntimeServiceResolver) ResolveServiceIPs(ctx context.Context, serviceName string) ([]net.IP, error) {
	if serviceName == "" {
		return nil, fmt.Errorf("service name cannot be empty")
	}

	// Use the service registry to resolve IPs
	ips, err := r.registry.GetServiceIPs(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve service %q: %w", serviceName, err)
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("service %q not found in registry", serviceName)
	}

	return ips, nil
}

// GetRegistry returns the service registry for registration operations
func (r *RuntimeServiceResolver) GetRegistry() *registry.Registry {
	return r.registry
}
