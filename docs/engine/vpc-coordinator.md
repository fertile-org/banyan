# VPC Coordinator

> **Implementation Status**: Phase 3 - Pending
> **Dependencies**: Agent Registry ✅, VPC Module ✅ (already implemented)

## Overview

The VPC Coordinator bridges the Engine control plane to the VPC networking layer. It acts as a facade that coordinates network provisioning, IP allocation, and security policy enforcement through the underlying VPC managers (NetworkManager, IPAMManager, SecurityManager).

## Philosophy

**"Implicit networking"** - Users don't need to think about networking. In banyan.yml, services reach each other by name:

```yaml
services:
  api:
    image: myapi:latest
    environment:
      - DATABASE_URL=postgres://db:5432/app  # "db" resolves via DNS
  db:
    image: postgres:15
```

Behind the scenes, VPC Coordinator:
1. Allocates IP addresses for each container
2. Registers service names in DNS
3. Configures network namespaces
4. (MVP-2) Applies security policies from `network_policy` plugin

## Responsibilities

1. **Network Provisioning** - Create and manage VPC networks and subnets
2. **IP Allocation Coordination** - Request and release IP addresses for containers
3. **DNS Registration** - Register service names for DNS-based discovery
4. **Security Policy Management** - Configure security groups and network policies (MVP-2)
5. **Network State Tracking** - Monitor network resource allocation
6. **Cross-Component Coordination** - Orchestrate multiple VPC managers

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         VPC Coordinator                                  │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                      Driving Adapters                            │   │
│  │  ┌─────────────────┐  ┌─────────────────┐                       │   │
│  │  │  Orchestrator   │  │  Reconciler     │                       │   │
│  │  │   Interface     │  │   Interface     │                       │   │
│  │  └────────┬────────┘  └────────┬────────┘                       │   │
│  └───────────┼────────────────────┼────────────────────────────────┘   │
│              │                    │                                     │
│  ┌───────────▼────────────────────▼────────────────────────────────┐   │
│  │                       Inbound Ports                              │   │
│  │  ┌─────────────────────────────────────────────────────────┐    │   │
│  │  │             VPCCoordinatorService Interface              │    │   │
│  │  │  - ProvisionNetwork(spec) → NetworkInfo                 │    │   │
│  │  │  - DeleteNetwork(vpcID) → error                         │    │   │
│  │  │  - AllocateContainerNetwork(req) → ContainerNetwork     │    │   │
│  │  │  - ReleaseContainerNetwork(containerID) → error         │    │   │
│  │  │  - ApplySecurityPolicy(policy) → error                  │    │   │
│  │  │  - GetNetworkStatus(vpcID) → NetworkStatus              │    │   │
│  │  └─────────────────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                │                                        │
│  ┌─────────────────────────────▼───────────────────────────────────┐   │
│  │                        Use Cases                                 │   │
│  │  ┌──────────────┐ ┌────────────────┐ ┌────────────────────┐    │   │
│  │  │  Network     │ │   Container    │ │    Security        │    │   │
│  │  │  Provisioning│ │   Networking   │ │    Policy          │    │   │
│  │  │  UseCase     │ │   UseCase      │ │    UseCase         │    │   │
│  │  └──────────────┘ └────────────────┘ └────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                │                                        │
│  ┌─────────────────────────────▼───────────────────────────────────┐   │
│  │                       Domain Layer                               │   │
│  │  ┌─────────────┐ ┌──────────────┐ ┌────────────────────────┐   │   │
│  │  │   VPC       │ │   Subnet     │ │  ContainerNetwork      │   │   │
│  │  │   Entity    │ │   Entity     │ │    Value Object        │   │   │
│  │  └─────────────┘ └──────────────┘ └────────────────────────┘   │   │
│  │  ┌─────────────┐ ┌──────────────┐ ┌────────────────────────┐   │   │
│  │  │ SecurityGrp │ │ NetworkPolicy│ │    NetworkInfo         │   │   │
│  │  │   Entity    │ │ Value Object │ │    Value Object        │   │   │
│  │  └─────────────┘ └──────────────┘ └────────────────────────┘   │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                │                                        │
│  ┌─────────────────────────────▼───────────────────────────────────┐   │
│  │                      Outbound Ports                              │   │
│  │  ┌─────────────────────────────────────────────────────────┐    │   │
│  │  │              NetworkManager Interface                    │    │   │
│  │  │  - CreateNetwork(spec) → VPC                            │    │   │
│  │  │  - DeleteNetwork(id) → error                            │    │   │
│  │  │  - GetNetwork(id) → VPC                                 │    │   │
│  │  └─────────────────────────────────────────────────────────┘    │   │
│  │  ┌─────────────────────────────────────────────────────────┐    │   │
│  │  │              IPAMManager Interface                       │    │   │
│  │  │  - AllocateSubnet(vpcID, cidr) → Subnet                 │    │   │
│  │  │  - AllocateIP(subnetID) → IPAddress                     │    │   │
│  │  │  - ReleaseIP(ip) → error                                │    │   │
│  │  └─────────────────────────────────────────────────────────┘    │   │
│  │  ┌─────────────────────────────────────────────────────────┐    │   │
│  │  │              SecurityManager Interface                   │    │   │
│  │  │  - CreateSecurityGroup(spec) → SecurityGroup            │    │   │
│  │  │  - ApplyPolicy(policy) → error                          │    │   │
│  │  │  - DeleteSecurityGroup(id) → error                      │    │   │
│  │  └─────────────────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                │                                        │
│  ┌─────────────────────────────▼───────────────────────────────────┐   │
│  │                      Driven Adapters                             │   │
│  │  ┌──────────────────────────────────────────────────────────┐   │   │
│  │  │                    VPC Module Adapters                    │   │   │
│  │  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────────┐    │   │   │
│  │  │  │  Network    │ │   IPAM      │ │   Security      │    │   │   │
│  │  │  │  Manager    │ │   Manager   │ │   Manager       │    │   │   │
│  │  │  └─────────────┘ └─────────────┘ └─────────────────┘    │   │   │
│  │  └──────────────────────────────────────────────────────────┘   │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

## Domain Layer

### Entities

```go
// VPC represents a virtual private cloud network
type VPC struct {
    ID          string
    Name        string
    CIDR        string
    Status      VPCStatus
    Subnets     []Subnet
    CreatedAt   time.Time
    Metadata    map[string]string
}

// VPCStatus represents the state of a VPC
type VPCStatus string

const (
    VPCStatusProvisioning VPCStatus = "provisioning"
    VPCStatusActive       VPCStatus = "active"
    VPCStatusDeleting     VPCStatus = "deleting"
    VPCStatusFailed       VPCStatus = "failed"
)

// Subnet represents a network subnet within a VPC
type Subnet struct {
    ID          string
    VPCID       string
    CIDR        string
    Gateway     string
    AvailableIPs int
    TotalIPs    int
    Purpose     SubnetPurpose
}

// SubnetPurpose defines what the subnet is used for
type SubnetPurpose string

const (
    SubnetPurposeContainer SubnetPurpose = "container"  // Container IP allocation
    SubnetPurposeService   SubnetPurpose = "service"    // Service VIPs
    SubnetPurposeManagement SubnetPurpose = "management" // Management traffic
)

// SecurityGroup represents a group of security rules
type SecurityGroup struct {
    ID        string
    Name      string
    VPCID     string
    Rules     []SecurityRule
    CreatedAt time.Time
}

// SecurityRule defines a single security rule
type SecurityRule struct {
    Direction   RuleDirection  // ingress or egress
    Protocol    string         // tcp, udp, icmp, all
    PortRange   PortRange
    Source      string         // CIDR or security group reference
    Destination string
    Action      RuleAction     // allow or deny
    Priority    int
}

type RuleDirection string
const (
    RuleDirectionIngress RuleDirection = "ingress"
    RuleDirectionEgress  RuleDirection = "egress"
)

type RuleAction string
const (
    RuleActionAllow RuleAction = "allow"
    RuleActionDeny  RuleAction = "deny"
)

type PortRange struct {
    Start int
    End   int
}
```

### Value Objects

```go
// ContainerNetworkRequest specifies network requirements for a container
type ContainerNetworkRequest struct {
    ContainerID   string
    ServiceName   string
    Namespace     string
    VPCID         string
    SubnetID      string            // Optional, auto-select if empty
    SecurityGroups []string
    Labels        map[string]string
}

// ContainerNetwork represents allocated network resources for a container
type ContainerNetwork struct {
    ContainerID   string
    IPAddress     string
    Gateway       string
    SubnetCIDR    string
    MAC           string
    DNS           []string
    SecurityGroups []string
    VPCID         string
    SubnetID      string
}

// NetworkProvisionSpec specifies network provisioning requirements
type NetworkProvisionSpec struct {
    Name          string
    CIDR          string
    Subnets       []SubnetSpec
    SecurityGroups []SecurityGroupSpec
    DNSServers    []string
    Metadata      map[string]string
}

// SubnetSpec specifies a subnet to create
type SubnetSpec struct {
    Name    string
    CIDR    string
    Purpose SubnetPurpose
}

// SecurityGroupSpec specifies a security group to create
type SecurityGroupSpec struct {
    Name  string
    Rules []SecurityRule
}

// NetworkInfo contains summary information about a provisioned network
type NetworkInfo struct {
    VPCID          string
    Name           string
    CIDR           string
    Status         VPCStatus
    Subnets        []SubnetInfo
    SecurityGroups []string
    AvailableIPs   int
    AllocatedIPs   int
}

// SubnetInfo contains summary information about a subnet
type SubnetInfo struct {
    SubnetID     string
    CIDR         string
    Purpose      SubnetPurpose
    AvailableIPs int
    AllocatedIPs int
}

// NetworkPolicy defines network-level access control
type NetworkPolicy struct {
    Name        string
    Namespace   string
    PodSelector map[string]string  // Label selector
    Ingress     []NetworkPolicyRule
    Egress      []NetworkPolicyRule
}

type NetworkPolicyRule struct {
    From    []NetworkPolicyPeer
    To      []NetworkPolicyPeer
    Ports   []PortRange
}

type NetworkPolicyPeer struct {
    PodSelector       map[string]string
    NamespaceSelector map[string]string
    IPBlock           string
}
```

## Ports

### Inbound Ports (Service Interface)

```go
// VPCCoordinatorService defines the VPC coordination operations
type VPCCoordinatorService interface {
    // Network Provisioning
    ProvisionNetwork(ctx context.Context, spec NetworkProvisionSpec) (*NetworkInfo, error)
    DeleteNetwork(ctx context.Context, vpcID string) error
    GetNetworkStatus(ctx context.Context, vpcID string) (*NetworkInfo, error)

    // Container Networking
    AllocateContainerNetwork(ctx context.Context, req ContainerNetworkRequest) (*ContainerNetwork, error)
    ReleaseContainerNetwork(ctx context.Context, containerID string) error
    UpdateContainerNetwork(ctx context.Context, containerID string, updates ContainerNetworkUpdate) error

    // Security
    ApplySecurityPolicy(ctx context.Context, policy NetworkPolicy) error
    DeleteSecurityPolicy(ctx context.Context, name, namespace string) error
    CreateSecurityGroup(ctx context.Context, vpcID string, spec SecurityGroupSpec) (*SecurityGroup, error)
    DeleteSecurityGroup(ctx context.Context, groupID string) error

    // Query
    GetContainerNetwork(ctx context.Context, containerID string) (*ContainerNetwork, error)
    ListContainersByNetwork(ctx context.Context, vpcID string) ([]string, error)
}

// ContainerNetworkUpdate specifies updates to container network
type ContainerNetworkUpdate struct {
    SecurityGroups []string // New security groups to apply
}
```

### Outbound Ports (VPC Manager Interfaces)

```go
// NetworkManager handles VPC and subnet operations
type NetworkManager interface {
    CreateVPC(ctx context.Context, name, cidr string, metadata map[string]string) (*VPC, error)
    GetVPC(ctx context.Context, id string) (*VPC, error)
    DeleteVPC(ctx context.Context, id string) error
    ListVPCs(ctx context.Context) ([]VPC, error)
}

// IPAMManager handles IP address allocation
type IPAMManager interface {
    CreateSubnet(ctx context.Context, vpcID, cidr string, purpose SubnetPurpose) (*Subnet, error)
    GetSubnet(ctx context.Context, id string) (*Subnet, error)
    DeleteSubnet(ctx context.Context, id string) error

    AllocateIP(ctx context.Context, subnetID, containerID string) (string, error)
    ReleaseIP(ctx context.Context, ip string) error
    GetIPAllocation(ctx context.Context, containerID string) (*IPAllocation, error)

    GetSubnetCapacity(ctx context.Context, subnetID string) (available, total int, err error)
}

// IPAllocation represents an IP allocation record
type IPAllocation struct {
    IP          string
    SubnetID    string
    ContainerID string
    AllocatedAt time.Time
    LeaseExpiry time.Time
}

// SecurityManager handles security groups and policies
type SecurityManager interface {
    CreateSecurityGroup(ctx context.Context, vpcID string, spec SecurityGroupSpec) (*SecurityGroup, error)
    GetSecurityGroup(ctx context.Context, id string) (*SecurityGroup, error)
    UpdateSecurityGroup(ctx context.Context, id string, rules []SecurityRule) error
    DeleteSecurityGroup(ctx context.Context, id string) error

    ApplyNetworkPolicy(ctx context.Context, policy NetworkPolicy) error
    DeleteNetworkPolicy(ctx context.Context, name, namespace string) error
}

// ContainerNetworkStore tracks container network allocations
type ContainerNetworkStore interface {
    Save(ctx context.Context, network *ContainerNetwork) error
    Get(ctx context.Context, containerID string) (*ContainerNetwork, error)
    Delete(ctx context.Context, containerID string) error
    FindByVPC(ctx context.Context, vpcID string) ([]ContainerNetwork, error)
}
```

## Use Cases

### Network Provisioning Use Case

```go
// NetworkProvisioningUseCase handles VPC and subnet creation
type NetworkProvisioningUseCase struct {
    networkMgr  NetworkManager
    ipamMgr     IPAMManager
    securityMgr SecurityManager
    logger      *slog.Logger
}

func (uc *NetworkProvisioningUseCase) ProvisionNetwork(
    ctx context.Context,
    spec NetworkProvisionSpec,
) (*NetworkInfo, error) {
    // Step 1: Create VPC
    vpc, err := uc.networkMgr.CreateVPC(ctx, spec.Name, spec.CIDR, spec.Metadata)
    if err != nil {
        return nil, fmt.Errorf("failed to create VPC: %w", err)
    }

    uc.logger.Info("VPC created", "vpc_id", vpc.ID, "cidr", spec.CIDR)

    // Step 2: Create subnets
    var subnets []SubnetInfo
    for _, subnetSpec := range spec.Subnets {
        subnet, err := uc.ipamMgr.CreateSubnet(ctx, vpc.ID, subnetSpec.CIDR, subnetSpec.Purpose)
        if err != nil {
            // Rollback: delete VPC
            uc.networkMgr.DeleteVPC(ctx, vpc.ID)
            return nil, fmt.Errorf("failed to create subnet %s: %w", subnetSpec.Name, err)
        }

        subnets = append(subnets, SubnetInfo{
            SubnetID:     subnet.ID,
            CIDR:         subnet.CIDR,
            Purpose:      subnet.Purpose,
            AvailableIPs: subnet.AvailableIPs,
            AllocatedIPs: 0,
        })
    }

    // Step 3: Create default security groups
    var securityGroups []string
    for _, sgSpec := range spec.SecurityGroups {
        sg, err := uc.securityMgr.CreateSecurityGroup(ctx, vpc.ID, sgSpec)
        if err != nil {
            uc.logger.Warn("failed to create security group", "name", sgSpec.Name, "error", err)
            continue
        }
        securityGroups = append(securityGroups, sg.ID)
    }

    // Create default security group if none specified
    if len(securityGroups) == 0 {
        defaultSG, _ := uc.securityMgr.CreateSecurityGroup(ctx, vpc.ID, SecurityGroupSpec{
            Name: "default",
            Rules: []SecurityRule{
                {Direction: RuleDirectionEgress, Protocol: "all", Action: RuleActionAllow},
            },
        })
        if defaultSG != nil {
            securityGroups = append(securityGroups, defaultSG.ID)
        }
    }

    return &NetworkInfo{
        VPCID:          vpc.ID,
        Name:           vpc.Name,
        CIDR:           vpc.CIDR,
        Status:         VPCStatusActive,
        Subnets:        subnets,
        SecurityGroups: securityGroups,
    }, nil
}

func (uc *NetworkProvisioningUseCase) DeleteNetwork(
    ctx context.Context,
    vpcID string,
) error {
    // Get VPC to find associated resources
    vpc, err := uc.networkMgr.GetVPC(ctx, vpcID)
    if err != nil {
        return fmt.Errorf("VPC not found: %w", err)
    }

    // Delete subnets first
    for _, subnet := range vpc.Subnets {
        if err := uc.ipamMgr.DeleteSubnet(ctx, subnet.ID); err != nil {
            uc.logger.Warn("failed to delete subnet", "subnet_id", subnet.ID, "error", err)
        }
    }

    // Delete VPC
    if err := uc.networkMgr.DeleteVPC(ctx, vpcID); err != nil {
        return fmt.Errorf("failed to delete VPC: %w", err)
    }

    uc.logger.Info("VPC deleted", "vpc_id", vpcID)
    return nil
}
```

### Container Networking Use Case

```go
// ContainerNetworkingUseCase handles container network allocation
type ContainerNetworkingUseCase struct {
    networkMgr  NetworkManager
    ipamMgr     IPAMManager
    securityMgr SecurityManager
    store       ContainerNetworkStore
    logger      *slog.Logger
}

func (uc *ContainerNetworkingUseCase) AllocateContainerNetwork(
    ctx context.Context,
    req ContainerNetworkRequest,
) (*ContainerNetwork, error) {
    // Validate VPC exists
    vpc, err := uc.networkMgr.GetVPC(ctx, req.VPCID)
    if err != nil {
        return nil, fmt.Errorf("VPC not found: %w", err)
    }

    // Select subnet (auto-select if not specified)
    subnetID := req.SubnetID
    if subnetID == "" {
        subnetID, err = uc.selectSubnet(ctx, vpc, SubnetPurposeContainer)
        if err != nil {
            return nil, fmt.Errorf("failed to select subnet: %w", err)
        }
    }

    // Get subnet details
    subnet, err := uc.ipamMgr.GetSubnet(ctx, subnetID)
    if err != nil {
        return nil, fmt.Errorf("subnet not found: %w", err)
    }

    // Allocate IP
    ip, err := uc.ipamMgr.AllocateIP(ctx, subnetID, req.ContainerID)
    if err != nil {
        return nil, fmt.Errorf("failed to allocate IP: %w", err)
    }

    // Generate MAC address
    mac := uc.generateMAC(req.ContainerID)

    // Build container network info
    containerNet := &ContainerNetwork{
        ContainerID:    req.ContainerID,
        IPAddress:      ip,
        Gateway:        subnet.Gateway,
        SubnetCIDR:     subnet.CIDR,
        MAC:            mac,
        DNS:            []string{"10.96.0.10"}, // Default cluster DNS
        SecurityGroups: req.SecurityGroups,
        VPCID:          req.VPCID,
        SubnetID:       subnetID,
    }

    // Persist allocation
    if err := uc.store.Save(ctx, containerNet); err != nil {
        // Rollback IP allocation
        uc.ipamMgr.ReleaseIP(ctx, ip)
        return nil, fmt.Errorf("failed to save allocation: %w", err)
    }

    uc.logger.Info("container network allocated",
        "container_id", req.ContainerID,
        "ip", ip,
        "subnet_id", subnetID,
    )

    return containerNet, nil
}

func (uc *ContainerNetworkingUseCase) ReleaseContainerNetwork(
    ctx context.Context,
    containerID string,
) error {
    // Get current allocation
    allocation, err := uc.store.Get(ctx, containerID)
    if err != nil {
        return fmt.Errorf("allocation not found: %w", err)
    }

    // Release IP
    if err := uc.ipamMgr.ReleaseIP(ctx, allocation.IPAddress); err != nil {
        uc.logger.Warn("failed to release IP", "ip", allocation.IPAddress, "error", err)
    }

    // Delete allocation record
    if err := uc.store.Delete(ctx, containerID); err != nil {
        return fmt.Errorf("failed to delete allocation: %w", err)
    }

    uc.logger.Info("container network released",
        "container_id", containerID,
        "ip", allocation.IPAddress,
    )

    return nil
}

func (uc *ContainerNetworkingUseCase) selectSubnet(
    ctx context.Context,
    vpc *VPC,
    purpose SubnetPurpose,
) (string, error) {
    // Find subnet with matching purpose and available capacity
    var bestSubnet *Subnet
    maxAvailable := 0

    for _, subnet := range vpc.Subnets {
        if subnet.Purpose != purpose {
            continue
        }

        available, _, err := uc.ipamMgr.GetSubnetCapacity(ctx, subnet.ID)
        if err != nil {
            continue
        }

        if available > maxAvailable {
            maxAvailable = available
            s := subnet
            bestSubnet = &s
        }
    }

    if bestSubnet == nil {
        return "", ErrNoAvailableSubnet
    }

    return bestSubnet.ID, nil
}

func (uc *ContainerNetworkingUseCase) generateMAC(containerID string) string {
    // Generate deterministic MAC from container ID
    hash := sha256.Sum256([]byte(containerID))
    return fmt.Sprintf("02:%02x:%02x:%02x:%02x:%02x",
        hash[0], hash[1], hash[2], hash[3], hash[4])
}
```

### Security Policy Use Case

```go
// SecurityPolicyUseCase handles network security policies
type SecurityPolicyUseCase struct {
    securityMgr SecurityManager
    store       ContainerNetworkStore
    logger      *slog.Logger
}

func (uc *SecurityPolicyUseCase) ApplySecurityPolicy(
    ctx context.Context,
    policy NetworkPolicy,
) error {
    // Translate NetworkPolicy to security rules
    if err := uc.securityMgr.ApplyNetworkPolicy(ctx, policy); err != nil {
        return fmt.Errorf("failed to apply policy: %w", err)
    }

    uc.logger.Info("security policy applied",
        "name", policy.Name,
        "namespace", policy.Namespace,
    )

    return nil
}

func (uc *SecurityPolicyUseCase) UpdateContainerSecurityGroups(
    ctx context.Context,
    containerID string,
    securityGroups []string,
) error {
    allocation, err := uc.store.Get(ctx, containerID)
    if err != nil {
        return fmt.Errorf("allocation not found: %w", err)
    }

    // Update security groups
    allocation.SecurityGroups = securityGroups

    if err := uc.store.Save(ctx, allocation); err != nil {
        return fmt.Errorf("failed to update allocation: %w", err)
    }

    uc.logger.Info("container security groups updated",
        "container_id", containerID,
        "security_groups", securityGroups,
    )

    return nil
}
```

## Adapters

### Driven Adapter: VPC Module Integration

```go
// VPCModuleAdapter connects to the VPC module managers
type VPCModuleAdapter struct {
    networkMgr  *vpc.NetworkManager
    ipamMgr     *vpc.IPAMManager
    securityMgr *vpc.SecurityManager
}

func NewVPCModuleAdapter(config *vpc.Config) (*VPCModuleAdapter, error) {
    networkMgr, err := vpc.NewNetworkManager(config)
    if err != nil {
        return nil, err
    }

    ipamMgr, err := vpc.NewIPAMManager(config)
    if err != nil {
        return nil, err
    }

    securityMgr, err := vpc.NewSecurityManager(config)
    if err != nil {
        return nil, err
    }

    return &VPCModuleAdapter{
        networkMgr:  networkMgr,
        ipamMgr:     ipamMgr,
        securityMgr: securityMgr,
    }, nil
}

// NetworkManager implementation
func (a *VPCModuleAdapter) CreateVPC(
    ctx context.Context,
    name, cidr string,
    metadata map[string]string,
) (*VPC, error) {
    vpcNet, err := a.networkMgr.CreateNetwork(ctx, &vpc.NetworkSpec{
        Name:     name,
        CIDR:     cidr,
        Metadata: metadata,
    })
    if err != nil {
        return nil, err
    }

    return &VPC{
        ID:       vpcNet.ID,
        Name:     vpcNet.Name,
        CIDR:     vpcNet.CIDR,
        Status:   VPCStatusActive,
        Metadata: vpcNet.Metadata,
    }, nil
}

// IPAMManager implementation
func (a *VPCModuleAdapter) AllocateIP(
    ctx context.Context,
    subnetID, containerID string,
) (string, error) {
    ip, err := a.ipamMgr.AllocateIP(ctx, subnetID, containerID)
    if err != nil {
        return "", err
    }
    return ip.String(), nil
}

func (a *VPCModuleAdapter) ReleaseIP(ctx context.Context, ip string) error {
    return a.ipamMgr.ReleaseIP(ctx, net.ParseIP(ip))
}

// SecurityManager implementation
func (a *VPCModuleAdapter) CreateSecurityGroup(
    ctx context.Context,
    vpcID string,
    spec SecurityGroupSpec,
) (*SecurityGroup, error) {
    sg, err := a.securityMgr.CreateSecurityGroup(ctx, &vpc.SecurityGroupSpec{
        VPCID: vpcID,
        Name:  spec.Name,
        Rules: convertRules(spec.Rules),
    })
    if err != nil {
        return nil, err
    }

    return &SecurityGroup{
        ID:    sg.ID,
        Name:  sg.Name,
        VPCID: vpcID,
        Rules: spec.Rules,
    }, nil
}
```

### Driven Adapter: etcd Container Network Store

```go
// EtcdContainerNetworkStore stores container network allocations in etcd
type EtcdContainerNetworkStore struct {
    client *clientv3.Client
    prefix string
}

func NewEtcdContainerNetworkStore(client *clientv3.Client) *EtcdContainerNetworkStore {
    return &EtcdContainerNetworkStore{
        client: client,
        prefix: "/banyan/container-networks/",
    }
}

func (s *EtcdContainerNetworkStore) Save(
    ctx context.Context,
    network *ContainerNetwork,
) error {
    data, err := json.Marshal(network)
    if err != nil {
        return err
    }

    key := s.prefix + network.ContainerID
    _, err = s.client.Put(ctx, key, string(data))
    return err
}

func (s *EtcdContainerNetworkStore) Get(
    ctx context.Context,
    containerID string,
) (*ContainerNetwork, error) {
    key := s.prefix + containerID
    resp, err := s.client.Get(ctx, key)
    if err != nil {
        return nil, err
    }

    if len(resp.Kvs) == 0 {
        return nil, ErrAllocationNotFound
    }

    var network ContainerNetwork
    if err := json.Unmarshal(resp.Kvs[0].Value, &network); err != nil {
        return nil, err
    }

    return &network, nil
}

func (s *EtcdContainerNetworkStore) Delete(ctx context.Context, containerID string) error {
    key := s.prefix + containerID
    _, err := s.client.Delete(ctx, key)
    return err
}

func (s *EtcdContainerNetworkStore) FindByVPC(
    ctx context.Context,
    vpcID string,
) ([]ContainerNetwork, error) {
    resp, err := s.client.Get(ctx, s.prefix, clientv3.WithPrefix())
    if err != nil {
        return nil, err
    }

    var networks []ContainerNetwork
    for _, kv := range resp.Kvs {
        var network ContainerNetwork
        if err := json.Unmarshal(kv.Value, &network); err != nil {
            continue
        }
        if network.VPCID == vpcID {
            networks = append(networks, network)
        }
    }

    return networks, nil
}
```

## Configuration

```go
type VPCCoordinatorConfig struct {
    // VPC Module settings
    VPCConfig *vpc.Config `yaml:"vpc"`

    // Default network settings
    DefaultVPCCIDR    string `yaml:"default_vpc_cidr"`    // Default: 10.0.0.0/16
    DefaultSubnetMask int    `yaml:"default_subnet_mask"` // Default: 24

    // DNS settings
    DefaultDNSServers []string `yaml:"default_dns_servers"`

    // etcd settings
    EtcdEndpoints []string `yaml:"etcd_endpoints"`
    EtcdPrefix    string   `yaml:"etcd_prefix"`
}
```

## Error Handling

```go
var (
    ErrVPCNotFound         = errors.New("VPC not found")
    ErrSubnetNotFound      = errors.New("subnet not found")
    ErrNoAvailableSubnet   = errors.New("no available subnet")
    ErrIPExhausted         = errors.New("IP addresses exhausted")
    ErrAllocationNotFound  = errors.New("allocation not found")
    ErrSecurityGroupNotFound = errors.New("security group not found")
)
```

## Testing

```go
func TestContainerNetworkingUseCase_AllocateContainerNetwork(t *testing.T) {
    networkMgr := NewMockNetworkManager()
    ipamMgr := NewMockIPAMManager()
    store := NewMockContainerNetworkStore()

    networkMgr.vpcs["vpc-1"] = &VPC{
        ID:   "vpc-1",
        CIDR: "10.0.0.0/16",
        Subnets: []Subnet{
            {ID: "subnet-1", CIDR: "10.0.1.0/24", Purpose: SubnetPurposeContainer},
        },
    }

    ipamMgr.nextIP = "10.0.1.10"

    uc := NewContainerNetworkingUseCase(networkMgr, ipamMgr, nil, store)

    result, err := uc.AllocateContainerNetwork(context.Background(), ContainerNetworkRequest{
        ContainerID: "container-1",
        VPCID:       "vpc-1",
    })

    assert.NoError(t, err)
    assert.Equal(t, "10.0.1.10", result.IPAddress)
    assert.Equal(t, "vpc-1", result.VPCID)
    assert.Equal(t, "subnet-1", result.SubnetID)
}

func TestNetworkProvisioningUseCase_ProvisionNetwork(t *testing.T) {
    networkMgr := NewMockNetworkManager()
    ipamMgr := NewMockIPAMManager()
    securityMgr := NewMockSecurityManager()

    uc := NewNetworkProvisioningUseCase(networkMgr, ipamMgr, securityMgr)

    result, err := uc.ProvisionNetwork(context.Background(), NetworkProvisionSpec{
        Name: "test-vpc",
        CIDR: "10.0.0.0/16",
        Subnets: []SubnetSpec{
            {Name: "containers", CIDR: "10.0.1.0/24", Purpose: SubnetPurposeContainer},
        },
    })

    assert.NoError(t, err)
    assert.NotEmpty(t, result.VPCID)
    assert.Equal(t, VPCStatusActive, result.Status)
    assert.Len(t, result.Subnets, 1)
}
```

## Network Policy Plugin (MVP-2)

Explicit network policies are supported via the `network_policy` plugin:

```yaml
services:
  api:
    image: myapi:latest
    plugins:
      - name: network_policy
        config:
          allow:
            - db
          deny_all_others: true
```

This translates to security rules that:
1. Allow traffic from `api` containers to `db` containers
2. Deny all other outbound traffic from `api`

## Related Documents

- [Orchestrator](./orchestrator.md) - Uses VPC coordinator for container networking
- [VPC Module](../../pkg/vpc/README.md) - Underlying VPC implementation ✅
- [Network Node](../agent/network-node.md) - Agent-side network operations
- [DNS Server](../../pkg/vpc/README.md#dns-server) - DNS-based service discovery ✅
