package registry

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/fertile-org/banyan/pkg/storage"
)

// ServiceInstance represents a container instance of a service
type ServiceInstance struct {
	ContainerID string `json:"container_id"`
	IP          string `json:"ip"`
	ServiceName string `json:"service_name"`
}

// Registry manages service to container mappings
type Registry struct {
	store storage.StateStore
	mu    sync.RWMutex
}

// NewRegistry creates a new service registry
func NewRegistry(store storage.StateStore) *Registry {
	return &Registry{
		store: store,
	}
}

// PutService registers a container instance for a service
func (r *Registry) PutService(ctx context.Context, serviceName, containerID string, ip net.IP) error {
	if serviceName == "" {
		return fmt.Errorf("service name cannot be empty")
	}
	if containerID == "" {
		return fmt.Errorf("container ID cannot be empty")
	}
	if ip == nil {
		return fmt.Errorf("IP cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	instance := ServiceInstance{
		ContainerID: containerID,
		IP:          ip.String(),
		ServiceName: serviceName,
	}

	// Store service → container mapping
	serviceKey := fmt.Sprintf("registry/services/%s/%s", serviceName, containerID)
	if err := r.store.Save(ctx, serviceKey, instance); err != nil {
		return fmt.Errorf("failed to save service instance: %w", err)
	}

	// Store container → service mapping for reverse lookup
	containerKey := fmt.Sprintf("registry/containers/%s", containerID)
	containerMapping := struct {
		ServiceName string `json:"service_name"`
	}{
		ServiceName: serviceName,
	}
	if err := r.store.Save(ctx, containerKey, containerMapping); err != nil {
		return fmt.Errorf("failed to save container mapping: %w", err)
	}

	return nil
}

// DeleteService removes a container instance from a service
func (r *Registry) DeleteService(ctx context.Context, serviceName, containerID string) error {
	if serviceName == "" {
		return fmt.Errorf("service name cannot be empty")
	}
	if containerID == "" {
		return fmt.Errorf("container ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if service instance exists
	serviceKey := fmt.Sprintf("registry/services/%s/%s", serviceName, containerID)
	var instance ServiceInstance
	if err := r.store.Get(ctx, serviceKey, &instance); err != nil {
		return fmt.Errorf("service instance not found: %s/%s", serviceName, containerID)
	}

	// Delete service → container mapping
	if err := r.store.Delete(ctx, serviceKey); err != nil {
		return fmt.Errorf("failed to delete service instance: %w", err)
	}

	// Delete container → service mapping
	containerKey := fmt.Sprintf("registry/containers/%s", containerID)
	if err := r.store.Delete(ctx, containerKey); err != nil {
		// Log but don't fail if container mapping is missing
		return nil
	}

	return nil
}

// GetServiceInstances returns all container instances for a service
func (r *Registry) GetServiceInstances(ctx context.Context, serviceName string) ([]ServiceInstance, error) {
	if serviceName == "" {
		return nil, fmt.Errorf("service name cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := fmt.Sprintf("registry/services/%s/", serviceName)
	keys, err := r.store.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list service instances: %w", err)
	}

	var instances []ServiceInstance
	for _, key := range keys {
		var instance ServiceInstance
		if err := r.store.Get(ctx, key, &instance); err != nil {
			continue
		}
		instances = append(instances, instance)
	}

	return instances, nil
}

// GetContainerService returns the service name for a container
func (r *Registry) GetContainerService(ctx context.Context, containerID string) (string, error) {
	if containerID == "" {
		return "", fmt.Errorf("container ID cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	containerKey := fmt.Sprintf("registry/containers/%s", containerID)
	var mapping struct {
		ServiceName string `json:"service_name"`
	}
	if err := r.store.Get(ctx, containerKey, &mapping); err != nil {
		return "", fmt.Errorf("container not found: %s", containerID)
	}

	return mapping.ServiceName, nil
}

// GetServiceIPs returns all IPs for a service (for DNS/security resolution)
func (r *Registry) GetServiceIPs(ctx context.Context, serviceName string) ([]net.IP, error) {
	instances, err := r.GetServiceInstances(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	var ips []net.IP
	for _, inst := range instances {
		if ip := net.ParseIP(inst.IP); ip != nil {
			ips = append(ips, ip)
		}
	}

	return ips, nil
}
