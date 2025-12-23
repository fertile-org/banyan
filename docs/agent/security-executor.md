# Security Executor - Detailed Design

## Overview

The Security Executor is the Agent component responsible for enforcing network security policies on the node. It acts as the **data plane** counterpart to the Engine's security policy management, translating high-level security rules into low-level firewall configurations using iptables/nftables.

## Responsibilities

1. **Policy Enforcement** - Apply network security policies via iptables/nftables
2. **Traffic Control** - Manage ingress/egress rules for containers
3. **Service Mesh Integration** - Configure traffic policies for service-to-service communication
4. **Network Isolation** - Enforce network segmentation between services
5. **Audit Logging** - Log security events and policy violations
6. **Rule Synchronization** - Keep firewall rules in sync with desired state

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│                          Security Executor                                │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │                       Driving Adapters                               │ │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌───────────────────┐ │ │
│  │  │  Task Handler    │  │  gRPC Handler    │  │  Policy Watcher   │ │ │
│  │  │  (from Executor) │  │  (direct calls)  │  │  (policy changes) │ │ │
│  │  └────────┬─────────┘  └────────┬─────────┘  └─────────┬─────────┘ │ │
│  └───────────┼─────────────────────┼─────────────────────┼───────────┘ │
│              │                     │                     │              │
│              └─────────────────────┴─────────────────────┘              │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                        Inbound Ports                                 │ │
│  │  ┌─────────────────────────────────────────────────────────────┐   │ │
│  │  │                  SecurityService                             │   │ │
│  │  │  - ApplyPolicy(policy) -> error                             │   │ │
│  │  │  - RemovePolicy(policyID) -> error                          │   │ │
│  │  │  - ListPolicies() -> []Policy                               │   │ │
│  │  │  - GetPolicyStatus(policyID) -> PolicyStatus                │   │ │
│  │  │  - SyncPolicies(policies) -> error                          │   │ │
│  │  └─────────────────────────────────────────────────────────────┘   │ │
│  │  ┌─────────────────────────────────────────────────────────────┐   │ │
│  │  │                  NetworkPolicyService                        │   │ │
│  │  │  - AllowIngress(rule) -> error                              │   │ │
│  │  │  - DenyIngress(rule) -> error                               │   │ │
│  │  │  - AllowEgress(rule) -> error                               │   │ │
│  │  │  - DenyEgress(rule) -> error                                │   │ │
│  │  └─────────────────────────────────────────────────────────────┘   │ │
│  │  ┌─────────────────────────────────────────────────────────────┐   │ │
│  │  │                  AuditService                                │   │ │
│  │  │  - GetSecurityEvents(filter) -> []SecurityEvent             │   │ │
│  │  │  - EnableAuditLogging(enabled) -> error                     │   │ │
│  │  └─────────────────────────────────────────────────────────────┘   │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                          Use Cases                                   │ │
│  │  ┌─────────────────┐ ┌─────────────────┐ ┌───────────────────────┐ │ │
│  │  │  PolicyUseCase  │ │ NetworkPolicy   │ │   AuditUseCase        │ │ │
│  │  │                 │ │    UseCase      │ │                       │ │ │
│  │  │ - Apply         │ │ - Allow/Deny    │ │ - Log Events          │ │ │
│  │  │ - Remove        │ │ - Ingress/Egress│ │ - Query Events        │ │ │
│  │  │ - Sync          │ │ - Validate      │ │ - Alert               │ │ │
│  │  └─────────────────┘ └─────────────────┘ └───────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                         Domain Layer                                 │ │
│  │  ┌────────────────────────────────────────────────────────────────┐ │ │
│  │  │  Entities: SecurityPolicy, NetworkRule, SecurityGroup         │ │ │
│  │  │  Value Objects: PolicyID, RuleAction, Protocol, PortRange     │ │ │
│  │  │  Domain Logic: Rule validation, Policy conflict detection     │ │ │
│  │  └────────────────────────────────────────────────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                        Outbound Ports                                │ │
│  │  ┌───────────────────┐ ┌───────────────────┐ ┌───────────────────┐ │ │
│  │  │  FirewallManager  │ │   RuleStore       │ │  AuditLogger      │ │ │
│  │  │  (iptables)       │ │   (persistence)   │ │  (logging)        │ │ │
│  │  └───────────────────┘ └───────────────────┘ └───────────────────┘ │ │
│  │  ┌───────────────────┐ ┌───────────────────┐                       │ │
│  │  │  IPSetManager     │ │  ConnTracker      │                       │ │
│  │  │  (ipsets)         │ │  (conntrack)      │                       │ │
│  │  └───────────────────┘ └───────────────────┘                       │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                        Driven Adapters                               │ │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌───────────────────┐ │ │
│  │  │ IPTables Adapter │  │ NFTables Adapter │  │  IPSet Adapter    │ │ │
│  │  │  (iptables cmd)  │  │  (nft cmd)       │  │  (ipset cmd)      │ │ │
│  │  └──────────────────┘  └──────────────────┘  └───────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                                                           │
└──────────────────────────────────────────────────────────────────────────┘
```

## Domain Layer

### Entities

```go
// SecurityPolicy represents a complete security policy
type SecurityPolicy struct {
    ID          PolicyID
    Name        string
    Description string
    Priority    int              // Lower is higher priority
    Selector    PolicySelector   // Which containers this applies to
    IngressRules []NetworkRule
    EgressRules  []NetworkRule
    DefaultAction RuleAction
    CreatedAt   time.Time
    UpdatedAt   time.Time
    Status      PolicyStatus
}

// NetworkRule defines a single network rule
type NetworkRule struct {
    ID          RuleID
    PolicyID    PolicyID
    Direction   RuleDirection    // ingress or egress
    Action      RuleAction       // allow or deny
    Protocol    Protocol         // tcp, udp, icmp, all
    Source      NetworkEndpoint
    Destination NetworkEndpoint
    Ports       []PortRange
    Comment     string
    Priority    int
}

// NetworkEndpoint represents a source or destination
type NetworkEndpoint struct {
    Type       EndpointType     // cidr, service, selector
    CIDR       string           // e.g., "10.0.0.0/8"
    ServiceID  string           // service name/ID
    Selector   map[string]string // label selector
    IPSetName  string           // for ipset references
}

// SecurityGroup represents a group of rules
type SecurityGroup struct {
    ID          SecurityGroupID
    Name        string
    Description string
    VPCNetworkID string
    InboundRules []NetworkRule
    OutboundRules []NetworkRule
}

// PolicySelector defines which containers a policy applies to
type PolicySelector struct {
    ServiceIDs   []string
    Labels       map[string]string
    NetworkID    string
    ContainerIDs []string // Direct container targeting
}
```

### Value Objects

```go
// PolicyID uniquely identifies a policy
type PolicyID string

// RuleID uniquely identifies a rule
type RuleID string

// RuleDirection indicates traffic direction
type RuleDirection string

const (
    DirectionIngress RuleDirection = "ingress"
    DirectionEgress  RuleDirection = "egress"
)

// RuleAction defines what to do with matching traffic
type RuleAction string

const (
    ActionAllow RuleAction = "allow"
    ActionDeny  RuleAction = "deny"
    ActionLog   RuleAction = "log"
    ActionDrop  RuleAction = "drop"
    ActionReject RuleAction = "reject"
)

// Protocol defines the network protocol
type Protocol string

const (
    ProtocolTCP  Protocol = "tcp"
    ProtocolUDP  Protocol = "udp"
    ProtocolICMP Protocol = "icmp"
    ProtocolAll  Protocol = "all"
)

// PortRange defines a range of ports
type PortRange struct {
    Start uint16
    End   uint16
}

func (p PortRange) IsSinglePort() bool {
    return p.Start == p.End
}

func (p PortRange) String() string {
    if p.IsSinglePort() {
        return strconv.Itoa(int(p.Start))
    }
    return fmt.Sprintf("%d:%d", p.Start, p.End)
}

// EndpointType defines the type of network endpoint
type EndpointType string

const (
    EndpointTypeCIDR     EndpointType = "cidr"
    EndpointTypeService  EndpointType = "service"
    EndpointTypeSelector EndpointType = "selector"
    EndpointTypeIPSet    EndpointType = "ipset"
)

// PolicyStatus represents the status of a policy
type PolicyStatus string

const (
    PolicyStatusPending  PolicyStatus = "pending"
    PolicyStatusApplied  PolicyStatus = "applied"
    PolicyStatusFailed   PolicyStatus = "failed"
    PolicyStatusConflict PolicyStatus = "conflict"
)
```

### Domain Logic

```go
// Validate policy configuration
func (p *SecurityPolicy) Validate() error {
    if p.ID == "" {
        return ErrPolicyIDRequired
    }

    if p.Name == "" {
        return ErrPolicyNameRequired
    }

    // Validate all ingress rules
    for i, rule := range p.IngressRules {
        if err := rule.Validate(); err != nil {
            return fmt.Errorf("ingress rule %d: %w", i, err)
        }
        if rule.Direction != DirectionIngress {
            return fmt.Errorf("ingress rule %d: wrong direction", i)
        }
    }

    // Validate all egress rules
    for i, rule := range p.EgressRules {
        if err := rule.Validate(); err != nil {
            return fmt.Errorf("egress rule %d: %w", i, err)
        }
        if rule.Direction != DirectionEgress {
            return fmt.Errorf("egress rule %d: wrong direction", i)
        }
    }

    return nil
}

// Validate network rule
func (r *NetworkRule) Validate() error {
    if r.Action == "" {
        return ErrActionRequired
    }

    if r.Protocol == "" {
        return ErrProtocolRequired
    }

    // Validate port ranges
    for _, pr := range r.Ports {
        if pr.Start > pr.End {
            return ErrInvalidPortRange
        }
        if pr.End > 65535 {
            return ErrInvalidPortRange
        }
    }

    // Validate endpoint
    if err := r.Source.Validate(); err != nil {
        return fmt.Errorf("source: %w", err)
    }
    if err := r.Destination.Validate(); err != nil {
        return fmt.Errorf("destination: %w", err)
    }

    return nil
}

// Check for policy conflicts
func (p *SecurityPolicy) ConflictsWith(other *SecurityPolicy) bool {
    // Check if selectors overlap
    if !p.Selector.Overlaps(other.Selector) {
        return false
    }

    // Check if rules conflict
    for _, rule1 := range append(p.IngressRules, p.EgressRules...) {
        for _, rule2 := range append(other.IngressRules, other.EgressRules...) {
            if rule1.Direction == rule2.Direction &&
               rule1.Matches(rule2) &&
               rule1.Action != rule2.Action {
                return true
            }
        }
    }

    return false
}
```

## Inbound Ports

### SecurityService

```go
// SecurityService is the main interface for security operations
type SecurityService interface {
    // Policy management
    ApplyPolicy(ctx context.Context, policy *SecurityPolicy) error
    RemovePolicy(ctx context.Context, policyID PolicyID) error
    UpdatePolicy(ctx context.Context, policy *SecurityPolicy) error
    GetPolicy(ctx context.Context, policyID PolicyID) (*SecurityPolicy, error)
    ListPolicies(ctx context.Context, filter PolicyFilter) ([]*SecurityPolicy, error)
    GetPolicyStatus(ctx context.Context, policyID PolicyID) (*PolicyStatus, error)

    // Bulk operations
    SyncPolicies(ctx context.Context, policies []*SecurityPolicy) error

    // Container operations
    GetContainerPolicies(ctx context.Context, containerID string) ([]*SecurityPolicy, error)
}

// PolicyFilter for listing policies
type PolicyFilter struct {
    ServiceID  string
    NetworkID  string
    Status     PolicyStatus
    Labels     map[string]string
}
```

### NetworkPolicyService

```go
// NetworkPolicyService provides low-level network policy operations
type NetworkPolicyService interface {
    // Ingress rules
    AllowIngress(ctx context.Context, rule NetworkRule) error
    DenyIngress(ctx context.Context, rule NetworkRule) error

    // Egress rules
    AllowEgress(ctx context.Context, rule NetworkRule) error
    DenyEgress(ctx context.Context, rule NetworkRule) error

    // Rule management
    RemoveRule(ctx context.Context, ruleID RuleID) error
    ListRules(ctx context.Context, direction RuleDirection) ([]NetworkRule, error)

    // IPSet operations (for efficient rule matching)
    CreateIPSet(ctx context.Context, name string, ips []string) error
    UpdateIPSet(ctx context.Context, name string, ips []string) error
    DeleteIPSet(ctx context.Context, name string) error
}
```

### AuditService

```go
// AuditService provides security audit capabilities
type AuditService interface {
    // Event retrieval
    GetSecurityEvents(ctx context.Context, filter EventFilter) ([]*SecurityEvent, error)
    StreamSecurityEvents(ctx context.Context, filter EventFilter) (<-chan *SecurityEvent, error)

    // Configuration
    EnableAuditLogging(ctx context.Context, enabled bool) error
    SetLogLevel(ctx context.Context, level AuditLogLevel) error

    // Statistics
    GetViolationStats(ctx context.Context, duration time.Duration) (*ViolationStats, error)
}

// SecurityEvent represents a security-related event
type SecurityEvent struct {
    ID          string
    Timestamp   time.Time
    Type        SecurityEventType
    PolicyID    PolicyID
    RuleID      RuleID
    Source      NetworkEndpoint
    Destination NetworkEndpoint
    Protocol    Protocol
    Port        uint16
    Action      RuleAction
    Verdict     string
    Message     string
}

// EventFilter for querying events
type EventFilter struct {
    StartTime   time.Time
    EndTime     time.Time
    PolicyID    PolicyID
    Type        SecurityEventType
    Source      string
    Destination string
    Limit       int
}
```

## Outbound Ports

### FirewallManager

```go
// FirewallManager manages firewall rules
type FirewallManager interface {
    // Chain management
    CreateChain(table, chain string) error
    DeleteChain(table, chain string) error
    FlushChain(table, chain string) error

    // Rule management
    AppendRule(table, chain string, rule *FirewallRule) error
    InsertRule(table, chain string, position int, rule *FirewallRule) error
    DeleteRule(table, chain string, rule *FirewallRule) error
    ReplaceRule(table, chain string, position int, rule *FirewallRule) error
    ListRules(table, chain string) ([]*FirewallRule, error)

    // Bulk operations
    RestoreRules(rules string) error
    SaveRules() (string, error)
}

// FirewallRule represents a firewall rule
type FirewallRule struct {
    Source      string
    Destination string
    Protocol    string
    SourcePort  string
    DestPort    string
    InInterface string
    OutInterface string
    State       string
    Action      string
    Comment     string
    Match       []string // Additional match extensions
}
```

### IPSetManager

```go
// IPSetManager manages IP sets for efficient rule matching
type IPSetManager interface {
    // Set management
    Create(name string, setType IPSetType, options *IPSetOptions) error
    Destroy(name string) error
    Flush(name string) error
    Rename(oldName, newName string) error

    // Entry management
    Add(name string, entry string) error
    Del(name string, entry string) error
    Test(name string, entry string) (bool, error)

    // Bulk operations
    AddBulk(name string, entries []string) error
    Swap(set1, set2 string) error

    // Information
    List(name string) ([]string, error)
    ListAll() (map[string][]string, error)
}

// IPSetType defines the type of IP set
type IPSetType string

const (
    IPSetTypeHashIP    IPSetType = "hash:ip"
    IPSetTypeHashNet   IPSetType = "hash:net"
    IPSetTypeHashIPPort IPSetType = "hash:ip,port"
)
```

### AuditLogger

```go
// AuditLogger handles security audit logging
type AuditLogger interface {
    // Logging
    LogEvent(event *SecurityEvent) error
    LogViolation(violation *PolicyViolation) error

    // Query
    QueryEvents(filter EventFilter) ([]*SecurityEvent, error)

    // Management
    Rotate() error
    SetRetention(duration time.Duration) error
}
```

## Use Cases

### PolicyUseCase

```go
type PolicyUseCase struct {
    firewall   FirewallManager
    ipset      IPSetManager
    store      RuleStore
    audit      AuditLogger
}

func (uc *PolicyUseCase) ApplyPolicy(ctx context.Context, policy *SecurityPolicy) error {
    // 1. Validate policy
    if err := policy.Validate(); err != nil {
        return fmt.Errorf("invalid policy: %w", err)
    }

    // 2. Check for conflicts with existing policies
    existing, err := uc.store.ListPolicies(ctx)
    if err != nil {
        return fmt.Errorf("failed to list existing policies: %w", err)
    }

    for _, ep := range existing {
        if ep.ID != policy.ID && policy.ConflictsWith(ep) {
            return fmt.Errorf("policy conflicts with %s: %w", ep.ID, ErrPolicyConflict)
        }
    }

    // 3. Create IP sets for endpoint selectors
    if err := uc.createIPSets(ctx, policy); err != nil {
        return fmt.Errorf("failed to create IP sets: %w", err)
    }

    // 4. Create chains for this policy
    policyChain := uc.policyChainName(policy.ID)
    if err := uc.firewall.CreateChain("filter", policyChain); err != nil {
        return fmt.Errorf("failed to create chain: %w", err)
    }

    // 5. Add rules to the chain
    for _, rule := range policy.IngressRules {
        fwRule := uc.toFirewallRule(rule, policy)
        if err := uc.firewall.AppendRule("filter", policyChain, fwRule); err != nil {
            return fmt.Errorf("failed to add ingress rule: %w", err)
        }
    }

    for _, rule := range policy.EgressRules {
        fwRule := uc.toFirewallRule(rule, policy)
        if err := uc.firewall.AppendRule("filter", policyChain, fwRule); err != nil {
            return fmt.Errorf("failed to add egress rule: %w", err)
        }
    }

    // 6. Add jump rule to main chain
    jumpRule := &FirewallRule{
        Comment: fmt.Sprintf("policy:%s", policy.ID),
        Action:  policyChain,
    }
    if err := uc.firewall.InsertRule("filter", "BANYAN-FORWARD", policy.Priority, jumpRule); err != nil {
        return fmt.Errorf("failed to add jump rule: %w", err)
    }

    // 7. Store policy
    policy.Status = PolicyStatusApplied
    policy.UpdatedAt = time.Now()
    if err := uc.store.SavePolicy(ctx, policy); err != nil {
        return fmt.Errorf("failed to store policy: %w", err)
    }

    // 8. Log the event
    uc.audit.LogEvent(&SecurityEvent{
        ID:        uuid.New().String(),
        Timestamp: time.Now(),
        Type:      SecurityEventTypePolicyApplied,
        PolicyID:  policy.ID,
        Message:   fmt.Sprintf("Applied policy %s with %d rules", policy.Name, len(policy.IngressRules)+len(policy.EgressRules)),
    })

    return nil
}

func (uc *PolicyUseCase) RemovePolicy(ctx context.Context, policyID PolicyID) error {
    // 1. Get policy
    policy, err := uc.store.GetPolicy(ctx, policyID)
    if err != nil {
        return fmt.Errorf("policy not found: %w", err)
    }

    // 2. Remove jump rule
    policyChain := uc.policyChainName(policyID)
    jumpRule := &FirewallRule{
        Comment: fmt.Sprintf("policy:%s", policyID),
        Action:  policyChain,
    }
    if err := uc.firewall.DeleteRule("filter", "BANYAN-FORWARD", jumpRule); err != nil {
        log.Printf("Warning: failed to remove jump rule: %v", err)
    }

    // 3. Flush and delete chain
    if err := uc.firewall.FlushChain("filter", policyChain); err != nil {
        log.Printf("Warning: failed to flush chain: %v", err)
    }
    if err := uc.firewall.DeleteChain("filter", policyChain); err != nil {
        log.Printf("Warning: failed to delete chain: %v", err)
    }

    // 4. Remove IP sets
    if err := uc.deleteIPSets(ctx, policy); err != nil {
        log.Printf("Warning: failed to delete IP sets: %v", err)
    }

    // 5. Remove from store
    if err := uc.store.DeletePolicy(ctx, policyID); err != nil {
        return fmt.Errorf("failed to delete policy from store: %w", err)
    }

    // 6. Log event
    uc.audit.LogEvent(&SecurityEvent{
        ID:        uuid.New().String(),
        Timestamp: time.Now(),
        Type:      SecurityEventTypePolicyRemoved,
        PolicyID:  policyID,
        Message:   fmt.Sprintf("Removed policy %s", policy.Name),
    })

    return nil
}

func (uc *PolicyUseCase) SyncPolicies(ctx context.Context, desired []*SecurityPolicy) error {
    // 1. Get current policies
    current, err := uc.store.ListPolicies(ctx)
    if err != nil {
        return fmt.Errorf("failed to list current policies: %w", err)
    }

    // 2. Build maps for comparison
    desiredMap := make(map[PolicyID]*SecurityPolicy)
    for _, p := range desired {
        desiredMap[p.ID] = p
    }

    currentMap := make(map[PolicyID]*SecurityPolicy)
    for _, p := range current {
        currentMap[p.ID] = p
    }

    // 3. Remove policies not in desired state
    for id := range currentMap {
        if _, exists := desiredMap[id]; !exists {
            if err := uc.RemovePolicy(ctx, id); err != nil {
                log.Printf("Warning: failed to remove policy %s: %v", id, err)
            }
        }
    }

    // 4. Apply/update desired policies
    for id, policy := range desiredMap {
        if current, exists := currentMap[id]; exists {
            // Update if changed
            if !policy.Equals(current) {
                if err := uc.ApplyPolicy(ctx, policy); err != nil {
                    log.Printf("Warning: failed to update policy %s: %v", id, err)
                }
            }
        } else {
            // Add new policy
            if err := uc.ApplyPolicy(ctx, policy); err != nil {
                log.Printf("Warning: failed to add policy %s: %v", id, err)
            }
        }
    }

    return nil
}

func (uc *PolicyUseCase) toFirewallRule(rule NetworkRule, policy *SecurityPolicy) *FirewallRule {
    fwRule := &FirewallRule{
        Protocol: string(rule.Protocol),
        Comment:  fmt.Sprintf("rule:%s,policy:%s", rule.ID, policy.ID),
    }

    // Set source
    switch rule.Source.Type {
    case EndpointTypeCIDR:
        fwRule.Source = rule.Source.CIDR
    case EndpointTypeIPSet:
        fwRule.Match = append(fwRule.Match, "-m set --match-set "+rule.Source.IPSetName+" src")
    }

    // Set destination
    switch rule.Destination.Type {
    case EndpointTypeCIDR:
        fwRule.Destination = rule.Destination.CIDR
    case EndpointTypeIPSet:
        fwRule.Match = append(fwRule.Match, "-m set --match-set "+rule.Destination.IPSetName+" dst")
    }

    // Set ports
    if len(rule.Ports) > 0 {
        portStr := uc.portsToString(rule.Ports)
        fwRule.DestPort = portStr
    }

    // Set action
    switch rule.Action {
    case ActionAllow:
        fwRule.Action = "ACCEPT"
    case ActionDeny, ActionDrop:
        fwRule.Action = "DROP"
    case ActionReject:
        fwRule.Action = "REJECT"
    case ActionLog:
        fwRule.Action = "LOG"
    }

    return fwRule
}
```

## Driven Adapters

### IPTablesAdapter

```go
type IPTablesAdapter struct {
    ipt *iptables.IPTables
    mu  sync.Mutex
}

func NewIPTablesAdapter() (*IPTablesAdapter, error) {
    ipt, err := iptables.New()
    if err != nil {
        return nil, fmt.Errorf("failed to create iptables handle: %w", err)
    }
    return &IPTablesAdapter{ipt: ipt}, nil
}

func (a *IPTablesAdapter) CreateChain(table, chain string) error {
    a.mu.Lock()
    defer a.mu.Unlock()

    exists, err := a.ipt.ChainExists(table, chain)
    if err != nil {
        return err
    }
    if exists {
        return nil
    }
    return a.ipt.NewChain(table, chain)
}

func (a *IPTablesAdapter) DeleteChain(table, chain string) error {
    a.mu.Lock()
    defer a.mu.Unlock()

    return a.ipt.DeleteChain(table, chain)
}

func (a *IPTablesAdapter) FlushChain(table, chain string) error {
    a.mu.Lock()
    defer a.mu.Unlock()

    return a.ipt.ClearChain(table, chain)
}

func (a *IPTablesAdapter) AppendRule(table, chain string, rule *FirewallRule) error {
    a.mu.Lock()
    defer a.mu.Unlock()

    ruleSpec := a.buildRuleSpec(rule)
    return a.ipt.Append(table, chain, ruleSpec...)
}

func (a *IPTablesAdapter) InsertRule(table, chain string, position int, rule *FirewallRule) error {
    a.mu.Lock()
    defer a.mu.Unlock()

    ruleSpec := a.buildRuleSpec(rule)
    return a.ipt.Insert(table, chain, position, ruleSpec...)
}

func (a *IPTablesAdapter) DeleteRule(table, chain string, rule *FirewallRule) error {
    a.mu.Lock()
    defer a.mu.Unlock()

    ruleSpec := a.buildRuleSpec(rule)
    return a.ipt.Delete(table, chain, ruleSpec...)
}

func (a *IPTablesAdapter) ListRules(table, chain string) ([]*FirewallRule, error) {
    a.mu.Lock()
    defer a.mu.Unlock()

    rules, err := a.ipt.List(table, chain)
    if err != nil {
        return nil, err
    }

    var result []*FirewallRule
    for _, ruleStr := range rules {
        rule := a.parseRuleString(ruleStr)
        if rule != nil {
            result = append(result, rule)
        }
    }
    return result, nil
}

func (a *IPTablesAdapter) buildRuleSpec(rule *FirewallRule) []string {
    var spec []string

    if rule.Source != "" {
        spec = append(spec, "-s", rule.Source)
    }
    if rule.Destination != "" {
        spec = append(spec, "-d", rule.Destination)
    }
    if rule.Protocol != "" && rule.Protocol != "all" {
        spec = append(spec, "-p", rule.Protocol)
    }
    if rule.DestPort != "" {
        spec = append(spec, "--dport", rule.DestPort)
    }
    if rule.SourcePort != "" {
        spec = append(spec, "--sport", rule.SourcePort)
    }
    if rule.InInterface != "" {
        spec = append(spec, "-i", rule.InInterface)
    }
    if rule.OutInterface != "" {
        spec = append(spec, "-o", rule.OutInterface)
    }
    for _, match := range rule.Match {
        spec = append(spec, strings.Split(match, " ")...)
    }
    if rule.Comment != "" {
        spec = append(spec, "-m", "comment", "--comment", rule.Comment)
    }
    spec = append(spec, "-j", rule.Action)

    return spec
}
```

### IPSetAdapter

```go
type IPSetAdapter struct {
    mu sync.Mutex
}

func NewIPSetAdapter() *IPSetAdapter {
    return &IPSetAdapter{}
}

func (a *IPSetAdapter) Create(name string, setType IPSetType, options *IPSetOptions) error {
    a.mu.Lock()
    defer a.mu.Unlock()

    args := []string{"create", name, string(setType)}
    if options != nil {
        if options.MaxElem > 0 {
            args = append(args, "maxelem", strconv.Itoa(options.MaxElem))
        }
        if options.Timeout > 0 {
            args = append(args, "timeout", strconv.Itoa(options.Timeout))
        }
    }
    args = append(args, "-exist")

    return a.exec(args...)
}

func (a *IPSetAdapter) Destroy(name string) error {
    a.mu.Lock()
    defer a.mu.Unlock()

    return a.exec("destroy", name)
}

func (a *IPSetAdapter) Add(name string, entry string) error {
    a.mu.Lock()
    defer a.mu.Unlock()

    return a.exec("add", name, entry, "-exist")
}

func (a *IPSetAdapter) Del(name string, entry string) error {
    a.mu.Lock()
    defer a.mu.Unlock()

    return a.exec("del", name, entry, "-exist")
}

func (a *IPSetAdapter) AddBulk(name string, entries []string) error {
    a.mu.Lock()
    defer a.mu.Unlock()

    // Use restore for bulk operations
    var buf bytes.Buffer
    buf.WriteString(fmt.Sprintf("create %s hash:ip -exist\n", name))
    for _, entry := range entries {
        buf.WriteString(fmt.Sprintf("add %s %s\n", name, entry))
    }

    cmd := exec.Command("ipset", "restore")
    cmd.Stdin = &buf
    return cmd.Run()
}

func (a *IPSetAdapter) Swap(set1, set2 string) error {
    a.mu.Lock()
    defer a.mu.Unlock()

    return a.exec("swap", set1, set2)
}

func (a *IPSetAdapter) List(name string) ([]string, error) {
    a.mu.Lock()
    defer a.mu.Unlock()

    cmd := exec.Command("ipset", "list", name)
    output, err := cmd.Output()
    if err != nil {
        return nil, err
    }

    var entries []string
    lines := strings.Split(string(output), "\n")
    inMembers := false
    for _, line := range lines {
        if strings.HasPrefix(line, "Members:") {
            inMembers = true
            continue
        }
        if inMembers && strings.TrimSpace(line) != "" {
            entries = append(entries, strings.TrimSpace(line))
        }
    }
    return entries, nil
}

func (a *IPSetAdapter) exec(args ...string) error {
    cmd := exec.Command("ipset", args...)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("ipset %v failed: %s", args, string(output))
    }
    return nil
}
```

## Policy Application Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Security Policy Application Flow                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Task Executor                                                           │
│       │                                                                  │
│       │ 1. ApplyPolicy(policy)                                          │
│       ▼                                                                  │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                   Security Service                               │    │
│  │                                                                  │    │
│  │  ┌──────────────┐                                               │    │
│  │  │   Validate   │ 2. Validate policy                            │    │
│  │  │   Policy     │    - Check required fields                    │    │
│  │  └──────┬───────┘    - Validate rules                           │    │
│  │         │                                                        │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │    Check     │ 3. Check for conflicts                        │    │
│  │  │  Conflicts   │    - With existing policies                   │    │
│  │  └──────┬───────┘                                               │    │
│  │         │                                                        │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │   Create     │ 4. Create IP sets                             │    │
│  │  │   IP Sets    │    - For service selectors                    │    │
│  │  └──────┬───────┘    - For CIDR groups                          │    │
│  │         │                                                        │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │   Create     │ 5. Create iptables chain                      │    │
│  │  │    Chain     │    - BANYAN-POLICY-<id>                       │    │
│  │  └──────┬───────┘                                               │    │
│  │         │                                                        │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │    Add       │ 6. Add firewall rules                         │    │
│  │  │    Rules     │    - Ingress and egress                       │    │
│  │  └──────┬───────┘                                               │    │
│  │         │                                                        │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │    Store     │ 7. Persist policy                             │    │
│  │  │   Policy     │    - Update status                            │    │
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
    ErrPolicyNotFound     = errors.New("policy not found")
    ErrPolicyConflict     = errors.New("policy conflicts with existing policy")
    ErrPolicyIDRequired   = errors.New("policy ID is required")
    ErrPolicyNameRequired = errors.New("policy name is required")
    ErrActionRequired     = errors.New("action is required")
    ErrProtocolRequired   = errors.New("protocol is required")
    ErrInvalidPortRange   = errors.New("invalid port range")
    ErrRuleNotFound       = errors.New("rule not found")
    ErrIPSetNotFound      = errors.New("IP set not found")
    ErrChainNotFound      = errors.New("chain not found")
    ErrPermissionDenied   = errors.New("permission denied (requires root)")
)

// Error classification
func IsConflictError(err error) bool {
    return errors.Is(err, ErrPolicyConflict)
}

func IsNotFoundError(err error) bool {
    return errors.Is(err, ErrPolicyNotFound) ||
           errors.Is(err, ErrRuleNotFound) ||
           errors.Is(err, ErrIPSetNotFound) ||
           errors.Is(err, ErrChainNotFound)
}

func IsPermissionError(err error) bool {
    return errors.Is(err, ErrPermissionDenied)
}
```

## Testing Strategy

```go
// Unit test with mock firewall
func TestPolicyUseCase_Apply(t *testing.T) {
    mockFirewall := &MockFirewallManager{}
    mockIPSet := &MockIPSetManager{}
    mockStore := &MockRuleStore{}
    mockAudit := &MockAuditLogger{}

    uc := &PolicyUseCase{
        firewall: mockFirewall,
        ipset:    mockIPSet,
        store:    mockStore,
        audit:    mockAudit,
    }

    policy := &SecurityPolicy{
        ID:   "policy-123",
        Name: "test-policy",
        IngressRules: []NetworkRule{
            {
                ID:        "rule-1",
                Direction: DirectionIngress,
                Action:    ActionAllow,
                Protocol:  ProtocolTCP,
                Source:    NetworkEndpoint{Type: EndpointTypeCIDR, CIDR: "10.0.0.0/8"},
                Ports:     []PortRange{{Start: 80, End: 80}},
            },
        },
    }

    // Setup mocks
    mockStore.On("ListPolicies", mock.Anything).Return([]*SecurityPolicy{}, nil)
    mockFirewall.On("CreateChain", "filter", "BANYAN-POLICY-policy-123").Return(nil)
    mockFirewall.On("AppendRule", "filter", "BANYAN-POLICY-policy-123", mock.Anything).Return(nil)
    mockFirewall.On("InsertRule", "filter", "BANYAN-FORWARD", mock.Anything, mock.Anything).Return(nil)
    mockStore.On("SavePolicy", mock.Anything, mock.Anything).Return(nil)
    mockAudit.On("LogEvent", mock.Anything).Return(nil)

    // Execute
    err := uc.ApplyPolicy(context.Background(), policy)

    // Assert
    assert.NoError(t, err)
    mockFirewall.AssertExpectations(t)
    mockStore.AssertExpectations(t)
}

// Integration test with real iptables
func TestIPTablesAdapter_Integration(t *testing.T) {
    if os.Getuid() != 0 {
        t.Skip("Requires root")
    }

    adapter, err := NewIPTablesAdapter()
    require.NoError(t, err)

    testChain := "TEST-BANYAN-" + strconv.Itoa(os.Getpid())

    // Create chain
    err = adapter.CreateChain("filter", testChain)
    require.NoError(t, err)

    defer func() {
        _ = adapter.FlushChain("filter", testChain)
        _ = adapter.DeleteChain("filter", testChain)
    }()

    // Add rule
    rule := &FirewallRule{
        Source:   "10.0.0.0/8",
        Protocol: "tcp",
        DestPort: "80",
        Action:   "ACCEPT",
        Comment:  "test-rule",
    }
    err = adapter.AppendRule("filter", testChain, rule)
    require.NoError(t, err)

    // Verify rule exists
    rules, err := adapter.ListRules("filter", testChain)
    require.NoError(t, err)
    assert.Len(t, rules, 1)
}
```

## Related Documents

- [Network Node](./network-node.md) - Works with Network Node for enforcement
- [VPC Coordinator](../engine/vpc-coordinator.md) - Receives policies from VPC Coordinator
- [VPC Security Module](../../pkg/vpc/README.md) - Underlying security implementation
- [Task Executor](./task-executor.md) - Receives policy tasks from executor
