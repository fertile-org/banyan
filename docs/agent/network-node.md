# Network Node - Detailed Design

## Overview

The Network Node is the Agent component responsible for all node-level networking operations. It acts as the **data plane** counterpart to the Engine's VPC Coordinator, executing network configurations received from the control plane. This includes setting up container networking via CNI, managing routes, configuring interfaces, and interacting with the VPC module.

## Responsibilities

1. **Container Networking** - Execute CNI plugins to connect containers to networks
2. **Route Management** - Configure and maintain routing tables for overlay networks
3. **Interface Configuration** - Manage network interfaces (VXLAN, bridge, veth pairs)
4. **DNS Configuration** - Configure local DNS resolution for service discovery
5. **VPC Integration** - Interact with VPC module for IP allocation and network setup
6. **Network Health** - Monitor network connectivity and report issues

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│                             Network Node                                  │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │                       Driving Adapters                               │ │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌───────────────────┐ │ │
│  │  │  Task Handler    │  │  gRPC Handler    │  │  Container Event  │ │ │
│  │  │  (from Executor) │  │  (direct calls)  │  │     Handler       │ │ │
│  │  └────────┬─────────┘  └────────┬─────────┘  └─────────┬─────────┘ │ │
│  └───────────┼─────────────────────┼─────────────────────┼───────────┘ │
│              │                     │                     │              │
│              └─────────────────────┴─────────────────────┘              │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                        Inbound Ports                                 │ │
│  │  ┌─────────────────────────────────────────────────────────────┐   │ │
│  │  │                  NetworkNodeService                          │   │ │
│  │  │  - ConnectContainer(containerID, networkID, ip) -> error    │   │ │
│  │  │  - DisconnectContainer(containerID, networkID) -> error     │   │ │
│  │  │  - ConfigureNetwork(spec) -> error                          │   │ │
│  │  │  - GetNetworkInfo(containerID) -> NetworkInfo               │   │ │
│  │  │  - GetNodeNetworkStatus() -> NodeNetworkStatus              │   │ │
│  │  └─────────────────────────────────────────────────────────────┘   │ │
│  │  ┌─────────────────────────────────────────────────────────────┐   │ │
│  │  │                   RouteService                               │   │ │
│  │  │  - AddRoute(route) -> error                                 │   │ │
│  │  │  - RemoveRoute(route) -> error                              │   │ │
│  │  │  - ListRoutes() -> []Route                                  │   │ │
│  │  │  - SyncRoutes(routes) -> error                              │   │ │
│  │  └─────────────────────────────────────────────────────────────┘   │ │
│  │  ┌─────────────────────────────────────────────────────────────┐   │ │
│  │  │                    DNSService                                │   │ │
│  │  │  - ConfigureDNS(config) -> error                            │   │ │
│  │  │  - AddServiceRecord(record) -> error                        │   │ │
│  │  │  - RemoveServiceRecord(name) -> error                       │   │ │
│  │  └─────────────────────────────────────────────────────────────┘   │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                          Use Cases                                   │ │
│  │  ┌─────────────────┐ ┌─────────────────┐ ┌───────────────────────┐ │ │
│  │  │ContainerNetwork │ │   RouteUseCase  │ │   DNSUseCase          │ │ │
│  │  │   UseCase       │ │                 │ │                       │ │ │
│  │  │ - Connect       │ │ - Add/Remove    │ │ - Configure           │ │ │
│  │  │ - Disconnect    │ │ - Sync          │ │ - Update Records      │ │ │
│  │  │ - Configure     │ │ - Validate      │ │ - Resolve             │ │ │
│  │  └─────────────────┘ └─────────────────┘ └───────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                         Domain Layer                                 │ │
│  │  ┌────────────────────────────────────────────────────────────────┐ │ │
│  │  │  Entities: NetworkConfig, Route, Interface, DNSRecord         │ │ │
│  │  │  Value Objects: CIDR, IPAddress, MAC, NetworkID               │ │ │
│  │  │  Domain Logic: CIDR validation, Route conflicts               │ │ │
│  │  └────────────────────────────────────────────────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                        Outbound Ports                                │ │
│  │  ┌───────────────────┐ ┌───────────────────┐ ┌───────────────────┐ │ │
│  │  │   CNIExecutor     │ │  RouteManager     │ │  InterfaceManager │ │ │
│  │  │ (CNI plugins)     │ │  (netlink)        │ │  (netlink)        │ │ │
│  │  └───────────────────┘ └───────────────────┘ └───────────────────┘ │ │
│  │  ┌───────────────────┐ ┌───────────────────┐ ┌───────────────────┐ │ │
│  │  │   VPCClient       │ │   DNSConfigurer   │ │   IPAMClient      │ │ │
│  │  │ (VPC module)      │ │   (resolv.conf)   │ │   (IP allocation) │ │ │
│  │  └───────────────────┘ └───────────────────┘ └───────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                        Driven Adapters                               │ │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌───────────────────┐ │ │
│  │  │  CNI Plugin Exec │  │ Netlink Adapter  │  │  VPC Module       │ │ │
│  │  │  (flannel, etc.) │  │ (vishvananda)    │  │  Client           │ │ │
│  │  └──────────────────┘  └──────────────────┘  └───────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                                                           │
└──────────────────────────────────────────────────────────────────────────┘
```

## Domain Layer

### Entities

```go
// NetworkConfig represents a network configuration for the node
type NetworkConfig struct {
    ID          NetworkID
    Name        string
    CIDR        CIDR
    Gateway     IPAddress
    DNS         []IPAddress
    Type        NetworkType    // overlay, bridge, host
    MTU         int
    VNI         uint32         // VXLAN Network Identifier
    CreatedAt   time.Time
}

// Route represents a network route
type Route struct {
    Destination CIDR
    Gateway     IPAddress
    Interface   string
    Metric      int
    Source      IPAddress
    Table       int
    Protocol    RouteProtocol
}

// Interface represents a network interface
type Interface struct {
    Name      string
    Type      InterfaceType   // veth, bridge, vxlan, flannel
    MAC       MAC
    MTU       int
    IPAddress IPAddress
    CIDR      CIDR
    State     InterfaceState  // up, down
    Master    string          // Bridge name if enslaved
}

// ContainerNetworkConfig holds networking config for a container
type ContainerNetworkConfig struct {
    ContainerID   string
    NetworkID     NetworkID
    IPAddress     IPAddress
    Gateway       IPAddress
    MAC           MAC
    DNS           []IPAddress
    InterfaceName string
}

// DNSRecord represents a service DNS record
type DNSRecord struct {
    Name      string      // service name
    Type      DNSType     // A, AAAA, CNAME, SRV
    Value     string      // IP or hostname
    TTL       int
    Priority  int         // for SRV records
}
```

### Value Objects

```go
// CIDR represents an IP network in CIDR notation
type CIDR struct {
    IP      net.IP
    Network *net.IPNet
}

func NewCIDR(s string) (CIDR, error) {
    ip, network, err := net.ParseCIDR(s)
    if err != nil {
        return CIDR{}, ErrInvalidCIDR
    }
    return CIDR{IP: ip, Network: network}, nil
}

func (c CIDR) Contains(ip IPAddress) bool {
    return c.Network.Contains(net.IP(ip))
}

func (c CIDR) String() string {
    ones, _ := c.Network.Mask.Size()
    return fmt.Sprintf("%s/%d", c.IP.String(), ones)
}

// IPAddress represents an IP address
type IPAddress net.IP

func NewIPAddress(s string) (IPAddress, error) {
    ip := net.ParseIP(s)
    if ip == nil {
        return nil, ErrInvalidIPAddress
    }
    return IPAddress(ip), nil
}

// MAC represents a MAC address
type MAC net.HardwareAddr

func NewMAC(s string) (MAC, error) {
    mac, err := net.ParseMAC(s)
    if err != nil {
        return nil, ErrInvalidMAC
    }
    return MAC(mac), nil
}

// NetworkID identifies a network
type NetworkID string

// NetworkType defines the type of network
type NetworkType string

const (
    NetworkTypeOverlay NetworkType = "overlay"
    NetworkTypeBridge  NetworkType = "bridge"
    NetworkTypeHost    NetworkType = "host"
)

// InterfaceType defines the type of interface
type InterfaceType string

const (
    InterfaceTypeVeth    InterfaceType = "veth"
    InterfaceTypeBridge  InterfaceType = "bridge"
    InterfaceTypeVXLAN   InterfaceType = "vxlan"
    InterfaceTypeFlannel InterfaceType = "flannel"
)
```

### Domain Logic

```go
// Validate route doesn't conflict with existing routes
func (r *Route) ValidateAgainst(existing []Route) error {
    for _, e := range existing {
        if r.Destination.Overlaps(e.Destination) && r.Table == e.Table {
            if !r.Gateway.Equal(e.Gateway) {
                return ErrRouteConflict
            }
        }
    }
    return nil
}

// Check if CIDRs overlap
func (c CIDR) Overlaps(other CIDR) bool {
    return c.Network.Contains(other.IP) || other.Network.Contains(c.IP)
}

// Validate network configuration
func (nc *NetworkConfig) Validate() error {
    if nc.ID == "" {
        return ErrNetworkIDRequired
    }

    if nc.CIDR.Network == nil {
        return ErrCIDRRequired
    }

    if nc.MTU < 576 || nc.MTU > 65535 {
        return ErrInvalidMTU
    }

    if nc.Type == NetworkTypeOverlay && nc.VNI == 0 {
        return ErrVNIRequired
    }

    return nil
}
```

## Inbound Ports

### NetworkNodeService

```go
// NetworkNodeService is the main interface for network operations
type NetworkNodeService interface {
    // Container networking
    ConnectContainer(ctx context.Context, containerID string, networkID NetworkID, ip IPAddress) error
    DisconnectContainer(ctx context.Context, containerID string, networkID NetworkID) error
    GetContainerNetwork(ctx context.Context, containerID string) (*ContainerNetworkInfo, error)

    // Network configuration
    ConfigureNetwork(ctx context.Context, spec NetworkConfigSpec) error
    RemoveNetwork(ctx context.Context, networkID NetworkID) error
    GetNetwork(ctx context.Context, networkID NetworkID) (*NetworkConfig, error)
    ListNetworks(ctx context.Context) ([]*NetworkConfig, error)

    // Node status
    GetNodeNetworkStatus(ctx context.Context) (*NodeNetworkStatus, error)
    CheckConnectivity(ctx context.Context, target string) (*ConnectivityResult, error)
}

// ContainerNetworkInfo contains networking details for a container
type ContainerNetworkInfo struct {
    ContainerID   string
    NetworkID     NetworkID
    IPAddress     IPAddress
    Gateway       IPAddress
    MAC           MAC
    InterfaceName string
    DNS           []IPAddress
}

// NodeNetworkStatus represents the network status of the node
type NodeNetworkStatus struct {
    NodeIP        IPAddress
    PodCIDR       CIDR
    Interfaces    []InterfaceInfo
    Routes        []Route
    DNS           DNSConfig
    Connectivity  map[string]bool
    LastUpdated   time.Time
}

// NetworkConfigSpec for creating/updating a network
type NetworkConfigSpec struct {
    ID       NetworkID
    Name     string
    CIDR     CIDR
    Gateway  IPAddress
    Type     NetworkType
    MTU      int
    VNI      uint32
    Options  map[string]string
}
```

### RouteService

```go
// RouteService manages network routes
type RouteService interface {
    // Route management
    AddRoute(ctx context.Context, route Route) error
    RemoveRoute(ctx context.Context, route Route) error
    UpdateRoute(ctx context.Context, route Route) error
    GetRoute(ctx context.Context, dest CIDR) (*Route, error)
    ListRoutes(ctx context.Context, filter RouteFilter) ([]Route, error)

    // Bulk operations
    SyncRoutes(ctx context.Context, desired []Route) error

    // Validation
    ValidateRoute(ctx context.Context, route Route) error
}

// RouteFilter for listing routes
type RouteFilter struct {
    Table     *int
    Interface *string
    Protocol  *RouteProtocol
}
```

### DNSService

```go
// DNSService manages DNS configuration
type DNSService interface {
    // Configuration
    ConfigureDNS(ctx context.Context, config DNSConfig) error
    GetDNSConfig(ctx context.Context) (*DNSConfig, error)

    // Service records
    AddServiceRecord(ctx context.Context, record DNSRecord) error
    RemoveServiceRecord(ctx context.Context, name string, recordType DNSType) error
    ListServiceRecords(ctx context.Context) ([]DNSRecord, error)

    // Resolution
    Resolve(ctx context.Context, name string, recordType DNSType) ([]string, error)
}

// DNSConfig represents DNS configuration
type DNSConfig struct {
    Nameservers   []IPAddress
    SearchDomains []string
    Options       []string
}
```

## Outbound Ports

### CNIExecutor

```go
// CNIExecutor executes CNI plugin operations
type CNIExecutor interface {
    // Add container to network
    AddNetwork(ctx context.Context, netConfig *CNINetConfig, runtimeConfig *CNIRuntimeConfig) (*CNIResult, error)

    // Remove container from network
    DelNetwork(ctx context.Context, netConfig *CNINetConfig, runtimeConfig *CNIRuntimeConfig) error

    // Check container network status
    CheckNetwork(ctx context.Context, netConfig *CNINetConfig, runtimeConfig *CNIRuntimeConfig) error
}

// CNINetConfig represents CNI network configuration
type CNINetConfig struct {
    CNIVersion   string            `json:"cniVersion"`
    Name         string            `json:"name"`
    Type         string            `json:"type"`
    Bridge       string            `json:"bridge,omitempty"`
    IPAM         *CNIIPAMConfig    `json:"ipam,omitempty"`
    Delegate     *CNINetConfig     `json:"delegate,omitempty"`
    IsGateway    bool              `json:"isGateway,omitempty"`
    IPMasq       bool              `json:"ipMasq,omitempty"`
}

// CNIRuntimeConfig represents runtime-specific configuration
type CNIRuntimeConfig struct {
    ContainerID string
    NetNS       string
    IfName      string
    Args        [][2]string
    CapabilityArgs map[string]interface{}
}

// CNIResult represents the result of a CNI operation
type CNIResult struct {
    Interfaces []CNIInterface
    IPs        []CNIIP
    Routes     []CNIRoute
    DNS        CNIDNS
}
```

### RouteManager

```go
// RouteManager manages system routing tables
type RouteManager interface {
    // Route operations
    AddRoute(route *netlinkRoute) error
    DelRoute(route *netlinkRoute) error
    ReplaceRoute(route *netlinkRoute) error
    GetRoute(dest *net.IPNet) (*netlinkRoute, error)
    ListRoutes(table int) ([]*netlinkRoute, error)

    // Rule operations (for policy routing)
    AddRule(rule *netlinkRule) error
    DelRule(rule *netlinkRule) error
    ListRules() ([]*netlinkRule, error)
}
```

### InterfaceManager

```go
// InterfaceManager manages network interfaces
type InterfaceManager interface {
    // Interface operations
    CreateBridge(name string, mtu int) error
    CreateVeth(name, peerName string, mtu int) error
    CreateVXLAN(name string, vni uint32, port int, dstPort int) error
    DeleteInterface(name string) error

    // Interface configuration
    SetInterfaceUp(name string) error
    SetInterfaceDown(name string) error
    SetInterfaceAddress(name string, addr *net.IPNet) error
    SetInterfaceMaster(name, master string) error
    SetMTU(name string, mtu int) error

    // Interface info
    GetInterface(name string) (*InterfaceInfo, error)
    ListInterfaces() ([]*InterfaceInfo, error)
}
```

### VPCClient

```go
// VPCClient interfaces with the VPC module
type VPCClient interface {
    // Network operations
    GetNetwork(ctx context.Context, networkID string) (*VPCNetwork, error)
    GetSubnet(ctx context.Context, subnetID string) (*VPCSubnet, error)

    // IP management
    AllocateIP(ctx context.Context, subnetID string, containerID string) (*IPAllocation, error)
    ReleaseIP(ctx context.Context, ip string) error
    GetIPAllocation(ctx context.Context, ip string) (*IPAllocation, error)

    // Route information
    GetRemoteHosts(ctx context.Context, networkID string) ([]RemoteHost, error)
}
```

## Use Cases

### ContainerNetworkUseCase

```go
type ContainerNetworkUseCase struct {
    cni       CNIExecutor
    vpc       VPCClient
    routes    RouteManager
    iface     InterfaceManager
    dns       DNSConfigurer
}

func (uc *ContainerNetworkUseCase) ConnectContainer(
    ctx context.Context,
    containerID string,
    networkID NetworkID,
    ip IPAddress,
) error {
    // 1. Get network configuration from VPC
    network, err := uc.vpc.GetNetwork(ctx, string(networkID))
    if err != nil {
        return fmt.Errorf("failed to get network: %w", err)
    }

    // 2. Get container network namespace
    netns, err := uc.getContainerNetNS(containerID)
    if err != nil {
        return fmt.Errorf("failed to get container netns: %w", err)
    }

    // 3. Prepare CNI configuration
    cniConfig := &CNINetConfig{
        CNIVersion: "1.0.0",
        Name:       string(networkID),
        Type:       "flannel",
        Delegate: &CNINetConfig{
            Type:      "bridge",
            IsGateway: true,
        },
        IPAM: &CNIIPAMConfig{
            Type:    "static",
            Address: ip.String() + "/24",
            Gateway: network.Gateway,
        },
    }

    runtimeConfig := &CNIRuntimeConfig{
        ContainerID: containerID,
        NetNS:       netns,
        IfName:      "eth0",
    }

    // 4. Execute CNI ADD
    result, err := uc.cni.AddNetwork(ctx, cniConfig, runtimeConfig)
    if err != nil {
        return fmt.Errorf("CNI add failed: %w", err)
    }

    // 5. Configure DNS for the container
    if err := uc.configureDNS(ctx, containerID, network.DNS); err != nil {
        // Rollback network on DNS failure
        _ = uc.cni.DelNetwork(ctx, cniConfig, runtimeConfig)
        return fmt.Errorf("DNS configuration failed: %w", err)
    }

    log.Printf("Container %s connected to network %s with IP %s",
        containerID, networkID, result.IPs[0].Address)

    return nil
}

func (uc *ContainerNetworkUseCase) DisconnectContainer(
    ctx context.Context,
    containerID string,
    networkID NetworkID,
) error {
    // 1. Get container network namespace
    netns, err := uc.getContainerNetNS(containerID)
    if err != nil {
        // Container might already be gone
        if errors.Is(err, ErrContainerNotFound) {
            return nil
        }
        return fmt.Errorf("failed to get container netns: %w", err)
    }

    // 2. Prepare CNI configuration
    cniConfig := &CNINetConfig{
        CNIVersion: "1.0.0",
        Name:       string(networkID),
        Type:       "flannel",
    }

    runtimeConfig := &CNIRuntimeConfig{
        ContainerID: containerID,
        NetNS:       netns,
        IfName:      "eth0",
    }

    // 3. Execute CNI DEL
    if err := uc.cni.DelNetwork(ctx, cniConfig, runtimeConfig); err != nil {
        return fmt.Errorf("CNI del failed: %w", err)
    }

    // 4. Release IP back to VPC
    // This is typically handled by the Engine, but we ensure cleanup
    log.Printf("Container %s disconnected from network %s", containerID, networkID)

    return nil
}

func (uc *ContainerNetworkUseCase) getContainerNetNS(containerID string) (string, error) {
    // Get network namespace path from container runtime
    // Typically: /proc/<pid>/ns/net or /var/run/netns/<containerID>
    nsPath := fmt.Sprintf("/var/run/docker/netns/%s", containerID)
    if _, err := os.Stat(nsPath); err != nil {
        return "", ErrContainerNotFound
    }
    return nsPath, nil
}
```

### RouteUseCase

```go
type RouteUseCase struct {
    routes RouteManager
    vpc    VPCClient
}

func (uc *RouteUseCase) SyncRoutes(ctx context.Context, networkID NetworkID) error {
    // 1. Get desired routes from VPC
    remoteHosts, err := uc.vpc.GetRemoteHosts(ctx, string(networkID))
    if err != nil {
        return fmt.Errorf("failed to get remote hosts: %w", err)
    }

    // 2. Convert to routes
    desiredRoutes := make([]Route, 0, len(remoteHosts))
    for _, host := range remoteHosts {
        route := Route{
            Destination: host.PodCIDR,
            Gateway:     host.NodeIP,
            Interface:   "flannel.1",
            Metric:      100,
            Protocol:    RouteProtocolBanyan,
        }
        desiredRoutes = append(desiredRoutes, route)
    }

    // 3. Get current routes
    currentRoutes, err := uc.routes.ListRoutes(0)
    if err != nil {
        return fmt.Errorf("failed to list routes: %w", err)
    }

    // 4. Calculate diff
    toAdd, toRemove := uc.diffRoutes(desiredRoutes, currentRoutes)

    // 5. Apply changes
    for _, route := range toRemove {
        if err := uc.routes.DelRoute(uc.toNetlinkRoute(&route)); err != nil {
            log.Printf("Warning: failed to remove route %s: %v", route.Destination, err)
        }
    }

    for _, route := range toAdd {
        if err := uc.routes.AddRoute(uc.toNetlinkRoute(&route)); err != nil {
            log.Printf("Warning: failed to add route %s: %v", route.Destination, err)
        }
    }

    log.Printf("Route sync complete: added %d, removed %d", len(toAdd), len(toRemove))
    return nil
}

func (uc *RouteUseCase) diffRoutes(desired, current []Route) (toAdd, toRemove []Route) {
    desiredMap := make(map[string]Route)
    for _, r := range desired {
        key := r.Destination.String() + "-" + r.Gateway.String()
        desiredMap[key] = r
    }

    currentMap := make(map[string]Route)
    for _, r := range current {
        // Only consider routes managed by Banyan
        if r.Protocol != RouteProtocolBanyan {
            continue
        }
        key := r.Destination.String() + "-" + r.Gateway.String()
        currentMap[key] = r
    }

    // Find routes to add
    for key, route := range desiredMap {
        if _, exists := currentMap[key]; !exists {
            toAdd = append(toAdd, route)
        }
    }

    // Find routes to remove
    for key, route := range currentMap {
        if _, exists := desiredMap[key]; !exists {
            toRemove = append(toRemove, route)
        }
    }

    return toAdd, toRemove
}
```

## Driven Adapters

### CNIPluginAdapter

```go
type CNIPluginAdapter struct {
    cniDir     string
    netConfDir string
    binDir     string
}

func NewCNIPluginAdapter(cniDir string) *CNIPluginAdapter {
    return &CNIPluginAdapter{
        cniDir:     cniDir,
        netConfDir: filepath.Join(cniDir, "net.d"),
        binDir:     filepath.Join(cniDir, "bin"),
    }
}

func (a *CNIPluginAdapter) AddNetwork(
    ctx context.Context,
    netConfig *CNINetConfig,
    runtimeConfig *CNIRuntimeConfig,
) (*CNIResult, error) {
    // 1. Serialize network config
    configBytes, err := json.Marshal(netConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal CNI config: %w", err)
    }

    // 2. Build environment variables
    env := []string{
        "CNI_COMMAND=ADD",
        "CNI_CONTAINERID=" + runtimeConfig.ContainerID,
        "CNI_NETNS=" + runtimeConfig.NetNS,
        "CNI_IFNAME=" + runtimeConfig.IfName,
        "CNI_PATH=" + a.binDir,
    }

    // 3. Execute CNI plugin
    pluginPath := filepath.Join(a.binDir, netConfig.Type)
    cmd := exec.CommandContext(ctx, pluginPath)
    cmd.Stdin = bytes.NewReader(configBytes)
    cmd.Env = append(os.Environ(), env...)

    output, err := cmd.Output()
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            return nil, fmt.Errorf("CNI plugin failed: %s", string(exitErr.Stderr))
        }
        return nil, fmt.Errorf("CNI plugin execution failed: %w", err)
    }

    // 4. Parse result
    var result CNIResult
    if err := json.Unmarshal(output, &result); err != nil {
        return nil, fmt.Errorf("failed to parse CNI result: %w", err)
    }

    return &result, nil
}

func (a *CNIPluginAdapter) DelNetwork(
    ctx context.Context,
    netConfig *CNINetConfig,
    runtimeConfig *CNIRuntimeConfig,
) error {
    configBytes, err := json.Marshal(netConfig)
    if err != nil {
        return fmt.Errorf("failed to marshal CNI config: %w", err)
    }

    env := []string{
        "CNI_COMMAND=DEL",
        "CNI_CONTAINERID=" + runtimeConfig.ContainerID,
        "CNI_NETNS=" + runtimeConfig.NetNS,
        "CNI_IFNAME=" + runtimeConfig.IfName,
        "CNI_PATH=" + a.binDir,
    }

    pluginPath := filepath.Join(a.binDir, netConfig.Type)
    cmd := exec.CommandContext(ctx, pluginPath)
    cmd.Stdin = bytes.NewReader(configBytes)
    cmd.Env = append(os.Environ(), env...)

    if err := cmd.Run(); err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            return fmt.Errorf("CNI plugin DEL failed: %s", string(exitErr.Stderr))
        }
        return fmt.Errorf("CNI plugin execution failed: %w", err)
    }

    return nil
}
```

### NetlinkAdapter

```go
type NetlinkAdapter struct {
    handle *netlink.Handle
}

func NewNetlinkAdapter() (*NetlinkAdapter, error) {
    handle, err := netlink.NewHandle()
    if err != nil {
        return nil, fmt.Errorf("failed to create netlink handle: %w", err)
    }
    return &NetlinkAdapter{handle: handle}, nil
}

func (a *NetlinkAdapter) AddRoute(route *netlink.Route) error {
    return a.handle.RouteAdd(route)
}

func (a *NetlinkAdapter) DelRoute(route *netlink.Route) error {
    return a.handle.RouteDel(route)
}

func (a *NetlinkAdapter) ListRoutes(table int) ([]*netlink.Route, error) {
    filter := &netlink.Route{
        Table: table,
    }
    return a.handle.RouteListFiltered(netlink.FAMILY_V4, filter, netlink.RT_FILTER_TABLE)
}

func (a *NetlinkAdapter) CreateBridge(name string, mtu int) error {
    la := netlink.NewLinkAttrs()
    la.Name = name
    la.MTU = mtu

    bridge := &netlink.Bridge{LinkAttrs: la}
    if err := a.handle.LinkAdd(bridge); err != nil {
        return fmt.Errorf("failed to create bridge: %w", err)
    }

    return a.handle.LinkSetUp(bridge)
}

func (a *NetlinkAdapter) CreateVXLAN(name string, vni uint32, port, dstPort int) error {
    la := netlink.NewLinkAttrs()
    la.Name = name

    vxlan := &netlink.Vxlan{
        LinkAttrs: la,
        VxlanId:   int(vni),
        Port:      port,
        Learning:  true,
    }

    if err := a.handle.LinkAdd(vxlan); err != nil {
        return fmt.Errorf("failed to create VXLAN: %w", err)
    }

    return a.handle.LinkSetUp(vxlan)
}

func (a *NetlinkAdapter) SetInterfaceAddress(name string, addr *net.IPNet) error {
    link, err := a.handle.LinkByName(name)
    if err != nil {
        return fmt.Errorf("interface not found: %w", err)
    }

    netlinkAddr := &netlink.Addr{IPNet: addr}
    return a.handle.AddrAdd(link, netlinkAddr)
}
```

## Network Setup Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                   Container Network Setup Flow                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Container Runtime                                                       │
│       │                                                                  │
│       │ 1. ConnectContainer(containerID, networkID, ip)                 │
│       ▼                                                                  │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                   Network Node Service                           │    │
│  │                                                                  │    │
│  │  ┌──────────────┐                                               │    │
│  │  │  Get Network │ 2. Fetch network config from VPC              │    │
│  │  │    Config    │    - CIDR, Gateway, DNS                       │    │
│  │  └──────┬───────┘    - VNI for VXLAN                            │    │
│  │         │                                                        │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │   Get NetNS  │ 3. Get container network namespace            │    │
│  │  │    Path      │    - /proc/<pid>/ns/net                       │    │
│  │  └──────┬───────┘                                               │    │
│  │         │                                                        │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │  Execute CNI │ 4. Run CNI plugin                             │    │
│  │  │     ADD      │    - Create veth pair                         │    │
│  │  └──────┬───────┘    - Assign IP address                        │    │
│  │         │            - Add to bridge                             │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │ Configure    │ 5. Setup DNS in container                     │    │
│  │  │    DNS       │    - /etc/resolv.conf                         │    │
│  │  └──────┬───────┘                                               │    │
│  │         │                                                        │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │   Return     │ 6. Return network info                        │    │
│  │  │    Info      │    - IP, MAC, Gateway                         │    │
│  │  └──────────────┘                                               │    │
│  │                                                                  │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

## Error Handling

```go
// Domain errors
var (
    ErrNetworkNotFound      = errors.New("network not found")
    ErrContainerNotFound    = errors.New("container not found")
    ErrInvalidCIDR          = errors.New("invalid CIDR notation")
    ErrInvalidIPAddress     = errors.New("invalid IP address")
    ErrInvalidMAC           = errors.New("invalid MAC address")
    ErrRouteConflict        = errors.New("route conflicts with existing route")
    ErrInterfaceNotFound    = errors.New("interface not found")
    ErrCNIFailed            = errors.New("CNI operation failed")
    ErrNetworkIDRequired    = errors.New("network ID is required")
    ErrCIDRRequired         = errors.New("CIDR is required")
    ErrVNIRequired          = errors.New("VNI required for overlay network")
    ErrInvalidMTU           = errors.New("invalid MTU value")
)

// Error classification
func IsNetworkError(err error) bool {
    return errors.Is(err, ErrNetworkNotFound) ||
           errors.Is(err, ErrRouteConflict) ||
           errors.Is(err, ErrCNIFailed)
}

func IsTransientError(err error) bool {
    // Transient errors that can be retried
    if strings.Contains(err.Error(), "resource temporarily unavailable") {
        return true
    }
    if strings.Contains(err.Error(), "device or resource busy") {
        return true
    }
    return false
}
```

## Testing Strategy

```go
// Unit test with mock CNI
func TestContainerNetworkUseCase_Connect(t *testing.T) {
    mockCNI := &MockCNIExecutor{}
    mockVPC := &MockVPCClient{}
    mockRoutes := &MockRouteManager{}
    mockIface := &MockInterfaceManager{}

    uc := &ContainerNetworkUseCase{
        cni:    mockCNI,
        vpc:    mockVPC,
        routes: mockRoutes,
        iface:  mockIface,
    }

    // Setup mocks
    mockVPC.On("GetNetwork", mock.Anything, "net-123").Return(&VPCNetwork{
        ID:      "net-123",
        CIDR:    "10.0.0.0/16",
        Gateway: "10.0.0.1",
        DNS:     []string{"10.0.0.2"},
    }, nil)

    mockCNI.On("AddNetwork", mock.Anything, mock.Anything, mock.Anything).Return(&CNIResult{
        IPs: []CNIIP{{Address: "10.0.1.5/24"}},
    }, nil)

    // Execute
    err := uc.ConnectContainer(
        context.Background(),
        "container-123",
        NetworkID("net-123"),
        IPAddress(net.ParseIP("10.0.1.5")),
    )

    // Assert
    assert.NoError(t, err)
    mockVPC.AssertExpectations(t)
    mockCNI.AssertExpectations(t)
}

// Integration test with real netlink
func TestNetlinkAdapter_Integration(t *testing.T) {
    if os.Getuid() != 0 {
        t.Skip("Requires root")
    }

    adapter, err := NewNetlinkAdapter()
    require.NoError(t, err)

    // Create test bridge
    bridgeName := "test-br-" + strconv.Itoa(os.Getpid())
    err = adapter.CreateBridge(bridgeName, 1500)
    require.NoError(t, err)

    defer func() {
        _ = adapter.DeleteInterface(bridgeName)
    }()

    // Verify bridge exists
    link, err := adapter.handle.LinkByName(bridgeName)
    require.NoError(t, err)
    assert.Equal(t, "bridge", link.Type())
}
```

## Related Documents

- [Container Runtime](./container-runtime.md) - Uses Network Node for container connectivity
- [VPC Coordinator](../engine/vpc-coordinator.md) - Control plane for networking
- [VPC Module](../../pkg/vpc/README.md) - Underlying VPC implementation
- [Security Executor](./security-executor.md) - Network security policies
