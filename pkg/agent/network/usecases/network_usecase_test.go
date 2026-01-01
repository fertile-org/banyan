package usecases

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/network/domain"
	"github.com/fertile-org/banyan/pkg/agent/network/ports/inbound"
	"github.com/fertile-org/banyan/pkg/agent/network/ports/outbound"
)

// Mock implementations

type mockIPAllocator struct {
	allocateFunc func(ctx context.Context, subnet *net.IPNet, containerID string) (net.IP, error)
	releaseFunc  func(ctx context.Context, ip net.IP) error
	getSubnet    func(ctx context.Context, networkID string) (*net.IPNet, error)
}

func (m *mockIPAllocator) AllocateIP(ctx context.Context, subnet *net.IPNet, containerID string) (net.IP, error) {
	if m.allocateFunc != nil {
		return m.allocateFunc(ctx, subnet, containerID)
	}
	return net.ParseIP("10.0.0.2"), nil
}

func (m *mockIPAllocator) ReleaseIP(ctx context.Context, ip net.IP) error {
	if m.releaseFunc != nil {
		return m.releaseFunc(ctx, ip)
	}
	return nil
}

func (m *mockIPAllocator) GetSubnet(ctx context.Context, networkID string) (*net.IPNet, error) {
	if m.getSubnet != nil {
		return m.getSubnet(ctx, networkID)
	}
	_, subnet, _ := net.ParseCIDR("10.0.0.0/24")
	return subnet, nil
}

type mockCNIProvider struct {
	addFunc    func(ctx context.Context, containerID, networkID string, ip net.IP) error
	removeFunc func(ctx context.Context, containerID, networkID string) error
	statusFunc func(ctx context.Context) (*outbound.PluginStatus, error)
}

func (m *mockCNIProvider) AddToNetwork(ctx context.Context, containerID, networkID string, ip net.IP) error {
	if m.addFunc != nil {
		return m.addFunc(ctx, containerID, networkID, ip)
	}
	return nil
}

func (m *mockCNIProvider) RemoveFromNetwork(ctx context.Context, containerID, networkID string) error {
	if m.removeFunc != nil {
		return m.removeFunc(ctx, containerID, networkID)
	}
	return nil
}

func (m *mockCNIProvider) GetPluginStatus(ctx context.Context) (*outbound.PluginStatus, error) {
	if m.statusFunc != nil {
		return m.statusFunc(ctx)
	}
	return &outbound.PluginStatus{Ready: true}, nil
}

type mockEndpointStore struct {
	endpoints    map[string]*domain.Endpoint
	saveFunc     func(ctx context.Context, endpoint *domain.Endpoint) error
	getFunc      func(ctx context.Context, containerID domain.ContainerID, networkID domain.NetworkID) (*domain.Endpoint, error)
	deleteFunc   func(ctx context.Context, containerID domain.ContainerID, networkID domain.NetworkID) error
	listFunc     func(ctx context.Context, containerID domain.ContainerID) ([]*domain.Endpoint, error)
	listNetFunc  func(ctx context.Context, networkID domain.NetworkID) ([]*domain.Endpoint, error)
}

func newMockEndpointStore() *mockEndpointStore {
	return &mockEndpointStore{
		endpoints: make(map[string]*domain.Endpoint),
	}
}

func (m *mockEndpointStore) key(containerID domain.ContainerID, networkID domain.NetworkID) string {
	return string(containerID) + ":" + string(networkID)
}

func (m *mockEndpointStore) SaveEndpoint(ctx context.Context, endpoint *domain.Endpoint) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, endpoint)
	}
	m.endpoints[m.key(endpoint.ContainerID, endpoint.NetworkID)] = endpoint
	return nil
}

func (m *mockEndpointStore) GetEndpoint(ctx context.Context, containerID domain.ContainerID, networkID domain.NetworkID) (*domain.Endpoint, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, containerID, networkID)
	}
	ep, ok := m.endpoints[m.key(containerID, networkID)]
	if !ok {
		return nil, errors.New("not found")
	}
	return ep, nil
}

func (m *mockEndpointStore) DeleteEndpoint(ctx context.Context, containerID domain.ContainerID, networkID domain.NetworkID) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, containerID, networkID)
	}
	delete(m.endpoints, m.key(containerID, networkID))
	return nil
}

func (m *mockEndpointStore) ListEndpoints(ctx context.Context, containerID domain.ContainerID) ([]*domain.Endpoint, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, containerID)
	}
	var result []*domain.Endpoint
	for _, ep := range m.endpoints {
		if ep.ContainerID == containerID {
			result = append(result, ep)
		}
	}
	return result, nil
}

func (m *mockEndpointStore) ListNetworkEndpoints(ctx context.Context, networkID domain.NetworkID) ([]*domain.Endpoint, error) {
	if m.listNetFunc != nil {
		return m.listNetFunc(ctx, networkID)
	}
	var result []*domain.Endpoint
	for _, ep := range m.endpoints {
		if ep.NetworkID == networkID {
			result = append(result, ep)
		}
	}
	return result, nil
}

type mockNetworkStore struct {
	networks map[domain.NetworkID]*domain.Network
	getFunc  func(ctx context.Context, networkID domain.NetworkID) (*domain.Network, error)
	listFunc func(ctx context.Context) ([]*domain.Network, error)
}

func newMockNetworkStore() *mockNetworkStore {
	_, subnet, _ := net.ParseCIDR("10.0.0.0/24")
	return &mockNetworkStore{
		networks: map[domain.NetworkID]*domain.Network{
			"network1": {
				ID:      "network1",
				Name:    "test-network",
				Subnet:  subnet,
				Gateway: net.ParseIP("10.0.0.1"),
			},
		},
	}
}

func (m *mockNetworkStore) GetNetwork(ctx context.Context, networkID domain.NetworkID) (*domain.Network, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, networkID)
	}
	net, ok := m.networks[networkID]
	if !ok {
		return nil, errors.New("network not found")
	}
	return net, nil
}

func (m *mockNetworkStore) ListNetworks(ctx context.Context) ([]*domain.Network, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	var result []*domain.Network
	for _, n := range m.networks {
		result = append(result, n)
	}
	return result, nil
}

// Tests

func TestConnectContainer_Success(t *testing.T) {
	ipAllocator := &mockIPAllocator{}
	cniProvider := &mockCNIProvider{}
	endpointStore := newMockEndpointStore()
	networkStore := newMockNetworkStore()

	uc := NewNetworkUseCase(ipAllocator, cniProvider, endpointStore, networkStore)

	endpoint, err := uc.ConnectContainer(
		context.Background(),
		"container1",
		"network1",
		nil,
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if endpoint == nil {
		t.Fatal("expected endpoint, got nil")
	}

	if endpoint.ContainerID != "container1" {
		t.Errorf("expected container1, got %s", endpoint.ContainerID)
	}

	if endpoint.NetworkID != "network1" {
		t.Errorf("expected network1, got %s", endpoint.NetworkID)
	}

	if endpoint.IPAddress == nil {
		t.Error("expected IP address, got nil")
	}
}

func TestConnectContainer_WithProvidedIP(t *testing.T) {
	ipAllocator := &mockIPAllocator{}
	cniProvider := &mockCNIProvider{}
	endpointStore := newMockEndpointStore()
	networkStore := newMockNetworkStore()

	uc := NewNetworkUseCase(ipAllocator, cniProvider, endpointStore, networkStore)

	providedIP := net.ParseIP("10.0.0.50")
	endpoint, err := uc.ConnectContainer(
		context.Background(),
		"container1",
		"network1",
		&inbound.ConnectOptions{IPAddress: providedIP},
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !endpoint.IPAddress.Equal(providedIP) {
		t.Errorf("expected %v, got %v", providedIP, endpoint.IPAddress)
	}
}

func TestConnectContainer_InvalidContainerID(t *testing.T) {
	uc := NewNetworkUseCase(nil, nil, nil, nil)

	_, err := uc.ConnectContainer(
		context.Background(),
		"",
		"network1",
		nil,
	)

	if !errors.Is(err, domain.ErrInvalidContainerID) {
		t.Errorf("expected ErrInvalidContainerID, got %v", err)
	}
}

func TestConnectContainer_InvalidNetworkID(t *testing.T) {
	uc := NewNetworkUseCase(nil, nil, nil, nil)

	_, err := uc.ConnectContainer(
		context.Background(),
		"container1",
		"",
		nil,
	)

	if !errors.Is(err, domain.ErrInvalidNetworkID) {
		t.Errorf("expected ErrInvalidNetworkID, got %v", err)
	}
}

func TestConnectContainer_AlreadyConnected(t *testing.T) {
	ipAllocator := &mockIPAllocator{}
	cniProvider := &mockCNIProvider{}
	endpointStore := newMockEndpointStore()
	networkStore := newMockNetworkStore()

	// Pre-populate endpoint
	endpointStore.endpoints["container1:network1"] = &domain.Endpoint{
		ContainerID: "container1",
		NetworkID:   "network1",
		IPAddress:   net.ParseIP("10.0.0.5"),
	}

	uc := NewNetworkUseCase(ipAllocator, cniProvider, endpointStore, networkStore)

	_, err := uc.ConnectContainer(
		context.Background(),
		"container1",
		"network1",
		nil,
	)

	if !errors.Is(err, domain.ErrAlreadyConnected) {
		t.Errorf("expected ErrAlreadyConnected, got %v", err)
	}
}

func TestConnectContainer_NetworkNotFound(t *testing.T) {
	ipAllocator := &mockIPAllocator{}
	cniProvider := &mockCNIProvider{}
	endpointStore := newMockEndpointStore()
	networkStore := newMockNetworkStore()

	uc := NewNetworkUseCase(ipAllocator, cniProvider, endpointStore, networkStore)

	_, err := uc.ConnectContainer(
		context.Background(),
		"container1",
		"nonexistent",
		nil,
	)

	if !errors.Is(err, domain.ErrNetworkNotFound) {
		t.Errorf("expected ErrNetworkNotFound, got %v", err)
	}
}

func TestConnectContainer_IPAllocationFailed(t *testing.T) {
	ipAllocator := &mockIPAllocator{
		allocateFunc: func(ctx context.Context, subnet *net.IPNet, containerID string) (net.IP, error) {
			return nil, errors.New("no IPs available")
		},
	}
	cniProvider := &mockCNIProvider{}
	endpointStore := newMockEndpointStore()
	networkStore := newMockNetworkStore()

	uc := NewNetworkUseCase(ipAllocator, cniProvider, endpointStore, networkStore)

	_, err := uc.ConnectContainer(
		context.Background(),
		"container1",
		"network1",
		nil,
	)

	if !errors.Is(err, domain.ErrIPAllocationFailed) {
		t.Errorf("expected ErrIPAllocationFailed, got %v", err)
	}
}

func TestConnectContainer_CNIFailed(t *testing.T) {
	ipReleased := false
	ipAllocator := &mockIPAllocator{
		releaseFunc: func(ctx context.Context, ip net.IP) error {
			ipReleased = true
			return nil
		},
	}
	cniProvider := &mockCNIProvider{
		addFunc: func(ctx context.Context, containerID, networkID string, ip net.IP) error {
			return errors.New("CNI error")
		},
	}
	endpointStore := newMockEndpointStore()
	networkStore := newMockNetworkStore()

	uc := NewNetworkUseCase(ipAllocator, cniProvider, endpointStore, networkStore)

	_, err := uc.ConnectContainer(
		context.Background(),
		"container1",
		"network1",
		nil,
	)

	if !errors.Is(err, domain.ErrNetworkOperationFailed) {
		t.Errorf("expected ErrNetworkOperationFailed, got %v", err)
	}

	if !ipReleased {
		t.Error("expected IP to be released after CNI failure")
	}
}

func TestDisconnectContainer_Success(t *testing.T) {
	ipAllocator := &mockIPAllocator{}
	cniProvider := &mockCNIProvider{}
	endpointStore := newMockEndpointStore()
	networkStore := newMockNetworkStore()

	// Pre-populate endpoint
	endpointStore.endpoints["container1:network1"] = &domain.Endpoint{
		ContainerID: "container1",
		NetworkID:   "network1",
		IPAddress:   net.ParseIP("10.0.0.5"),
	}

	uc := NewNetworkUseCase(ipAllocator, cniProvider, endpointStore, networkStore)

	err := uc.DisconnectContainer(
		context.Background(),
		"container1",
		"network1",
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify endpoint was deleted
	_, err = endpointStore.GetEndpoint(context.Background(), "container1", "network1")
	if err == nil {
		t.Error("expected endpoint to be deleted")
	}
}

func TestDisconnectContainer_NotConnected(t *testing.T) {
	ipAllocator := &mockIPAllocator{}
	cniProvider := &mockCNIProvider{}
	endpointStore := newMockEndpointStore()
	networkStore := newMockNetworkStore()

	uc := NewNetworkUseCase(ipAllocator, cniProvider, endpointStore, networkStore)

	err := uc.DisconnectContainer(
		context.Background(),
		"container1",
		"network1",
	)

	if !errors.Is(err, domain.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}

func TestGetEndpoint_Success(t *testing.T) {
	endpointStore := newMockEndpointStore()
	endpointStore.endpoints["container1:network1"] = &domain.Endpoint{
		ContainerID: "container1",
		NetworkID:   "network1",
		IPAddress:   net.ParseIP("10.0.0.5"),
	}

	uc := NewNetworkUseCase(nil, nil, endpointStore, nil)

	endpoint, err := uc.GetEndpoint(
		context.Background(),
		"container1",
		"network1",
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if endpoint.ContainerID != "container1" {
		t.Errorf("expected container1, got %s", endpoint.ContainerID)
	}
}

func TestGetEndpoint_NotFound(t *testing.T) {
	endpointStore := newMockEndpointStore()

	uc := NewNetworkUseCase(nil, nil, endpointStore, nil)

	_, err := uc.GetEndpoint(
		context.Background(),
		"container1",
		"network1",
	)

	if !errors.Is(err, domain.ErrEndpointNotFound) {
		t.Errorf("expected ErrEndpointNotFound, got %v", err)
	}
}

func TestListEndpoints_Success(t *testing.T) {
	endpointStore := newMockEndpointStore()
	endpointStore.endpoints["container1:network1"] = &domain.Endpoint{
		ContainerID: "container1",
		NetworkID:   "network1",
	}
	endpointStore.endpoints["container1:network2"] = &domain.Endpoint{
		ContainerID: "container1",
		NetworkID:   "network2",
	}

	uc := NewNetworkUseCase(nil, nil, endpointStore, nil)

	endpoints, err := uc.ListEndpoints(context.Background(), "container1")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(endpoints) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(endpoints))
	}
}

func TestListEndpoints_EmptyContainerID(t *testing.T) {
	uc := NewNetworkUseCase(nil, nil, nil, nil)

	_, err := uc.ListEndpoints(context.Background(), "")

	if !errors.Is(err, domain.ErrInvalidContainerID) {
		t.Errorf("expected ErrInvalidContainerID, got %v", err)
	}
}

func TestGetEndpointStatus_Connected(t *testing.T) {
	now := time.Now()
	endpointStore := newMockEndpointStore()
	endpointStore.endpoints["container1:network1"] = &domain.Endpoint{
		ContainerID: "container1",
		NetworkID:   "network1",
		IPAddress:   net.ParseIP("10.0.0.5"),
		Gateway:     net.ParseIP("10.0.0.1"),
		CreatedAt:   now,
	}

	uc := NewNetworkUseCase(nil, nil, endpointStore, nil)

	status, err := uc.GetEndpointStatus(
		context.Background(),
		"container1",
		"network1",
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if status.State != domain.NetworkStateConnected {
		t.Errorf("expected connected state, got %s", status.State)
	}

	if !status.IPAddress.Equal(net.ParseIP("10.0.0.5")) {
		t.Errorf("unexpected IP address: %v", status.IPAddress)
	}
}

func TestGetEndpointStatus_Disconnected(t *testing.T) {
	endpointStore := newMockEndpointStore()

	uc := NewNetworkUseCase(nil, nil, endpointStore, nil)

	status, err := uc.GetEndpointStatus(
		context.Background(),
		"container1",
		"network1",
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if status.State != domain.NetworkStateDisconnected {
		t.Errorf("expected disconnected state, got %s", status.State)
	}
}
