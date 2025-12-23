# Compose Parser - Detailed Design

## Overview

The **Compose Parser** is responsible for parsing Docker Compose files (`docker-compose.yaml`) and Banyan extension files (`banyan.yaml`), validating configurations, and transforming them into domain entities that can be processed by the Orchestrator.

**Architecture Pattern**: Clean Architecture + Hexagonal Architecture (Ports and Adapters)

## Responsibilities

| Responsibility | Description |
|----------------|-------------|
| Compose Parsing | Parse docker-compose.yaml using compose-go library |
| Banyan Parsing | Parse banyan.yaml custom extensions |
| Config Merging | Merge compose and banyan configurations |
| Validation | Validate service definitions and configurations |
| Transformation | Transform parsed configs to domain Service entities |
| Schema Support | Support multiple compose file versions (v2, v3) |

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Compose Parser                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│  Inbound Ports                                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  ComposeParserService                                                │    │
│  │  - Parse(composeContent, banyanContent) -> []Service                │    │
│  │  - Validate(composeContent) -> ValidationResult                      │    │
│  │  - GetSchema(version) -> Schema                                      │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
├─────────────────────────────────────────────────────────────────────────────┤
│  Domain Layer                                                                │
│  ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────────────┐     │
│  │  ParsedCompose   │ │  ParsedBanyan    │ │  ValidationResult        │     │
│  │  - Version       │ │  - VPCConfig     │ │  - Valid                 │     │
│  │  - Services      │ │  - Placement     │ │  - Errors                │     │
│  │  - Networks      │ │  - Scaling       │ │  - Warnings              │     │
│  │  - Volumes       │ │  - Plugins       │ └──────────────────────────┘     │
│  └──────────────────┘ └──────────────────┘                                  │
│  ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────────────┐     │
│  │  ServiceConfig   │ │  NetworkConfig   │ │  VolumeConfig            │     │
│  │  - Name          │ │  - Name          │ │  - Name                  │     │
│  │  - Image         │ │  - Driver        │ │  - Driver                │     │
│  │  - Ports         │ │  - IPAM          │ │  - DriverOpts            │     │
│  │  - Environment   │ │  - Labels        │ │  - Labels                │     │
│  └──────────────────┘ └──────────────────┘ └──────────────────────────┘     │
├─────────────────────────────────────────────────────────────────────────────┤
│  Use Cases                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  ParseComposeUseCase     │  ValidateConfigUseCase                   │    │
│  │  MergeConfigUseCase      │  TransformServicesUseCase                │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
├─────────────────────────────────────────────────────────────────────────────┤
│  Outbound Ports                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  YAMLParser              │  SchemaValidator                         │    │
│  │  - ParseYAML()           │  - ValidateAgainstSchema()               │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
├─────────────────────────────────────────────────────────────────────────────┤
│  Driven Adapters                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  ComposeGoAdapter        │  BanyanYAMLAdapter                       │    │
│  │  (compose-go library)    │  (gopkg.in/yaml.v3)                      │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Domain Layer

### Entities

```go
// pkg/engine/parser/domain/compose.go

package domain

// ParsedCompose represents a parsed docker-compose.yaml
type ParsedCompose struct {
    Version  string
    Services map[string]ServiceConfig
    Networks map[string]NetworkConfig
    Volumes  map[string]VolumeConfig
    Configs  map[string]ConfigConfig
    Secrets  map[string]SecretConfig
}

// ServiceConfig represents a service definition from compose file
type ServiceConfig struct {
    Name          string
    Image         string
    Build         *BuildConfig
    Command       []string
    Entrypoint    []string
    Environment   map[string]string
    EnvFile       []string
    Ports         []PortConfig
    Volumes       []VolumeMount
    Networks      map[string]ServiceNetworkConfig
    DependsOn     map[string]DependsOnConfig
    Deploy        *DeployConfig
    HealthCheck   *HealthCheckConfig
    Restart       string
    Labels        map[string]string
    Logging       *LoggingConfig
    Ulimits       map[string]UlimitConfig
    Sysctls       map[string]string
    CapAdd        []string
    CapDrop       []string
    Privileged    bool
    ReadOnly      bool
    User          string
    WorkingDir    string
    Hostname      string
    DomainName    string
    ExtraHosts    []string
    DNS           []string
    DNSSearch     []string
}

// NetworkConfig represents a network definition
type NetworkConfig struct {
    Name       string
    Driver     string
    DriverOpts map[string]string
    IPAM       *IPAMConfig
    External   bool
    Internal   bool
    Labels     map[string]string
}

// VolumeConfig represents a volume definition
type VolumeConfig struct {
    Name       string
    Driver     string
    DriverOpts map[string]string
    External   bool
    Labels     map[string]string
}
```

```go
// pkg/engine/parser/domain/banyan.go

package domain

// ParsedBanyan represents a parsed banyan.yaml extension file
type ParsedBanyan struct {
    Version   string
    VPC       VPCExtension
    Services  map[string]ServiceExtension
    Placement PlacementConfig
    Scaling   ScalingConfig
    Plugins   []PluginConfig
}

// VPCExtension contains VPC-specific configurations
type VPCExtension struct {
    Name        string
    CIDR        string
    Subnets     []SubnetConfig
    DNS         DNSConfig
    Peering     []PeeringConfig
    NAT         *NATConfig
}

// SubnetConfig defines a subnet within the VPC
type SubnetConfig struct {
    Name       string
    CIDR       string
    Zone       string
    Public     bool
    NATGateway bool
}

// ServiceExtension contains Banyan-specific service configurations
type ServiceExtension struct {
    Placement     ServicePlacement
    Scaling       ServiceScaling
    SecurityGroup string
    Subnet        string
    GPU           *GPUConfig
    Storage       *StorageConfig
}

// ServicePlacement defines placement constraints
type ServicePlacement struct {
    Constraints []string
    Preferences []PlacementPreference
    NodeLabels  map[string]string
}

// ServiceScaling defines auto-scaling configuration
type ServiceScaling struct {
    Min        int
    Max        int
    TargetCPU  int
    TargetMem  int
    Cooldown   string
}

// PluginConfig defines a plugin to be applied
type PluginConfig struct {
    Name    string
    Version string
    Config  map[string]interface{}
    When    string // Condition for plugin execution
}
```

### Value Objects

```go
// pkg/engine/parser/domain/values.go

package domain

import (
    "fmt"
    "regexp"
    "strconv"
    "strings"
)

// PortConfig represents a port mapping
type PortConfig struct {
    Target    uint32
    Published string // Can be port number or range
    Protocol  string // tcp, udp
    Mode      string // host, ingress
}

// ParsePort parses a port string like "8080:80/tcp"
func ParsePort(portStr string) (PortConfig, error) {
    pc := PortConfig{Protocol: "tcp"}

    // Handle protocol suffix
    if idx := strings.LastIndex(portStr, "/"); idx != -1 {
        pc.Protocol = portStr[idx+1:]
        portStr = portStr[:idx]
    }

    parts := strings.Split(portStr, ":")
    switch len(parts) {
    case 1:
        port, err := strconv.ParseUint(parts[0], 10, 32)
        if err != nil {
            return pc, fmt.Errorf("invalid port: %s", parts[0])
        }
        pc.Target = uint32(port)
        pc.Published = parts[0]
    case 2:
        pc.Published = parts[0]
        port, err := strconv.ParseUint(parts[1], 10, 32)
        if err != nil {
            return pc, fmt.Errorf("invalid target port: %s", parts[1])
        }
        pc.Target = uint32(port)
    default:
        return pc, fmt.Errorf("invalid port format: %s", portStr)
    }

    return pc, nil
}

// VolumeMount represents a volume mount configuration
type VolumeMount struct {
    Type        string // volume, bind, tmpfs
    Source      string
    Target      string
    ReadOnly    bool
    Consistency string
    Bind        *BindOptions
    Volume      *VolumeOptions
    Tmpfs       *TmpfsOptions
}

// ServiceNetworkConfig represents service-specific network settings
type ServiceNetworkConfig struct {
    Aliases     []string
    IPV4Address string
    IPV6Address string
    Priority    int
}

// DependsOnConfig represents service dependency configuration
type DependsOnConfig struct {
    Condition string // service_started, service_healthy, service_completed_successfully
    Restart   bool
}

// DeployConfig represents deployment configuration
type DeployConfig struct {
    Replicas      *int
    Resources     ResourcesConfig
    RestartPolicy *RestartPolicy
    Placement     PlacementConfig
    UpdateConfig  *UpdateConfig
    RollbackConfig *UpdateConfig
    Labels        map[string]string
}

// ResourcesConfig represents resource limits and reservations
type ResourcesConfig struct {
    Limits       ResourceSpec
    Reservations ResourceSpec
}

// ResourceSpec defines CPU and memory specifications
type ResourceSpec struct {
    CPUs    string
    Memory  string
    Devices []DeviceConfig
}

// HealthCheckConfig represents health check configuration
type HealthCheckConfig struct {
    Test        []string
    Interval    string
    Timeout     string
    Retries     int
    StartPeriod string
    Disable     bool
}

// BuildConfig represents build configuration
type BuildConfig struct {
    Context    string
    Dockerfile string
    Args       map[string]string
    Target     string
    CacheFrom  []string
    Labels     map[string]string
    Network    string
    ShmSize    string
}

// IPAMConfig represents IP Address Management configuration
type IPAMConfig struct {
    Driver string
    Config []IPAMPoolConfig
}

// IPAMPoolConfig represents an IPAM pool
type IPAMPoolConfig struct {
    Subnet  string
    Gateway string
    IPRange string
}
```

### Domain Logic

```go
// pkg/engine/parser/domain/validation.go

package domain

import (
    "fmt"
    "net"
    "regexp"
    "strings"
)

// ValidationResult contains validation results
type ValidationResult struct {
    Valid    bool
    Errors   []ValidationError
    Warnings []ValidationWarning
}

// ValidationError represents a validation error
type ValidationError struct {
    Field   string
    Message string
    Path    string
}

// ValidationWarning represents a validation warning
type ValidationWarning struct {
    Field   string
    Message string
    Path    string
}

// Validate validates the parsed compose configuration
func (pc *ParsedCompose) Validate() ValidationResult {
    result := ValidationResult{Valid: true}

    // Validate services
    for name, svc := range pc.Services {
        path := fmt.Sprintf("services.%s", name)

        // Service must have image or build
        if svc.Image == "" && svc.Build == nil {
            result.AddError(path, "image", "service must have either 'image' or 'build'")
        }

        // Validate ports
        for i, port := range svc.Ports {
            if port.Target == 0 {
                result.AddError(path, fmt.Sprintf("ports[%d]", i), "target port cannot be 0")
            }
            if port.Target > 65535 {
                result.AddError(path, fmt.Sprintf("ports[%d]", i), "port must be <= 65535")
            }
        }

        // Validate depends_on references
        for dep := range svc.DependsOn {
            if _, exists := pc.Services[dep]; !exists {
                result.AddError(path, "depends_on", fmt.Sprintf("service '%s' not found", dep))
            }
        }

        // Validate network references
        for netName := range svc.Networks {
            if _, exists := pc.Networks[netName]; !exists {
                // Check if it's the default network
                if netName != "default" {
                    result.AddError(path, "networks", fmt.Sprintf("network '%s' not defined", netName))
                }
            }
        }

        // Validate volume references
        for _, vol := range svc.Volumes {
            if vol.Type == "volume" && vol.Source != "" {
                if _, exists := pc.Volumes[vol.Source]; !exists {
                    result.AddWarning(path, "volumes", fmt.Sprintf("volume '%s' not defined, will be created", vol.Source))
                }
            }
        }

        // Validate health check
        if svc.HealthCheck != nil && !svc.HealthCheck.Disable {
            if len(svc.HealthCheck.Test) == 0 {
                result.AddError(path, "healthcheck.test", "health check test cannot be empty")
            }
        }

        // Validate resource limits
        if svc.Deploy != nil {
            if err := validateResources(svc.Deploy.Resources); err != nil {
                result.AddError(path, "deploy.resources", err.Error())
            }
        }
    }

    // Validate networks
    for name, net := range pc.Networks {
        path := fmt.Sprintf("networks.%s", name)

        if net.IPAM != nil {
            for i, pool := range net.IPAM.Config {
                if pool.Subnet != "" {
                    if _, _, err := net.ParseCIDR(pool.Subnet); err != nil {
                        result.AddError(path, fmt.Sprintf("ipam.config[%d].subnet", i), "invalid CIDR")
                    }
                }
            }
        }
    }

    // Check for circular dependencies
    if cycles := pc.detectCycles(); len(cycles) > 0 {
        for _, cycle := range cycles {
            result.AddError("services", "depends_on", fmt.Sprintf("circular dependency: %s", strings.Join(cycle, " -> ")))
        }
    }

    return result
}

func (r *ValidationResult) AddError(path, field, message string) {
    r.Valid = false
    r.Errors = append(r.Errors, ValidationError{
        Path:    path,
        Field:   field,
        Message: message,
    })
}

func (r *ValidationResult) AddWarning(path, field, message string) {
    r.Warnings = append(r.Warnings, ValidationWarning{
        Path:    path,
        Field:   field,
        Message: message,
    })
}

// detectCycles detects circular dependencies in service depends_on
func (pc *ParsedCompose) detectCycles() [][]string {
    var cycles [][]string
    visited := make(map[string]bool)
    recStack := make(map[string]bool)

    var dfs func(service string, path []string) bool
    dfs = func(service string, path []string) bool {
        visited[service] = true
        recStack[service] = true
        path = append(path, service)

        if svc, exists := pc.Services[service]; exists {
            for dep := range svc.DependsOn {
                if !visited[dep] {
                    if dfs(dep, path) {
                        return true
                    }
                } else if recStack[dep] {
                    // Found cycle
                    cycleStart := 0
                    for i, s := range path {
                        if s == dep {
                            cycleStart = i
                            break
                        }
                    }
                    cycle := append(path[cycleStart:], dep)
                    cycles = append(cycles, cycle)
                    return true
                }
            }
        }

        recStack[service] = false
        return false
    }

    for service := range pc.Services {
        if !visited[service] {
            dfs(service, nil)
        }
    }

    return cycles
}

func validateResources(res ResourcesConfig) error {
    // Validate CPU format (e.g., "0.5", "2")
    cpuRegex := regexp.MustCompile(`^\d+(\.\d+)?$`)
    if res.Limits.CPUs != "" && !cpuRegex.MatchString(res.Limits.CPUs) {
        return fmt.Errorf("invalid CPU limit format: %s", res.Limits.CPUs)
    }

    // Validate memory format (e.g., "512M", "1G", "1024")
    memRegex := regexp.MustCompile(`^\d+[bBkKmMgG]?$`)
    if res.Limits.Memory != "" && !memRegex.MatchString(res.Limits.Memory) {
        return fmt.Errorf("invalid memory limit format: %s", res.Limits.Memory)
    }

    return nil
}
```

## Inbound Ports

```go
// pkg/engine/parser/ports/inbound.go

package ports

import (
    "github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
    parserdomain "github.com/fertile-org/banyan/pkg/engine/parser/domain"
)

// ComposeParserService defines the compose parsing service interface
type ComposeParserService interface {
    // Parse parses compose and banyan files, returning domain services
    Parse(composeContent, banyanContent string) ([]domain.Service, error)

    // ParseCompose parses only the compose file
    ParseCompose(content string) (*parserdomain.ParsedCompose, error)

    // ParseBanyan parses only the banyan extension file
    ParseBanyan(content string) (*parserdomain.ParsedBanyan, error)

    // Validate validates compose content without full parsing
    Validate(composeContent string) (*parserdomain.ValidationResult, error)

    // ValidateWithBanyan validates both compose and banyan content
    ValidateWithBanyan(composeContent, banyanContent string) (*parserdomain.ValidationResult, error)

    // GetSupportedVersions returns supported compose file versions
    GetSupportedVersions() []string
}

// ParseOptions contains options for parsing
type ParseOptions struct {
    // SkipValidation skips validation during parsing
    SkipValidation bool

    // SkipInterpolation skips environment variable interpolation
    SkipInterpolation bool

    // WorkingDir is the working directory for relative paths
    WorkingDir string

    // Environment variables for interpolation
    Environment map[string]string

    // ProjectName overrides the project name
    ProjectName string
}

// ParseResult contains the result of parsing
type ParseResult struct {
    Services   []domain.Service
    Networks   []domain.NetworkDefinition
    Volumes    []domain.VolumeDefinition
    Validation *parserdomain.ValidationResult
}
```

## Outbound Ports

```go
// pkg/engine/parser/ports/outbound.go

package ports

import (
    "github.com/fertile-org/banyan/pkg/engine/parser/domain"
)

// YAMLParser defines the interface for YAML parsing
type YAMLParser interface {
    // ParseYAML parses YAML content into a generic map
    ParseYAML(content string) (map[string]interface{}, error)

    // UnmarshalYAML unmarshals YAML into a specific type
    UnmarshalYAML(content string, v interface{}) error
}

// ComposeLoader defines the interface for loading compose files
type ComposeLoader interface {
    // Load loads a compose file from content
    Load(content string, opts LoadOptions) (*domain.ParsedCompose, error)

    // LoadFromFile loads a compose file from path
    LoadFromFile(path string, opts LoadOptions) (*domain.ParsedCompose, error)
}

// LoadOptions contains options for loading compose files
type LoadOptions struct {
    // SkipInterpolation skips environment variable interpolation
    SkipInterpolation bool

    // SkipValidation skips schema validation
    SkipValidation bool

    // WorkingDir is the working directory
    WorkingDir string

    // Environment variables
    Environment map[string]string
}

// SchemaValidator defines the interface for schema validation
type SchemaValidator interface {
    // ValidateAgainstSchema validates content against compose schema
    ValidateAgainstSchema(content string, version string) error

    // GetSchema returns the schema for a version
    GetSchema(version string) ([]byte, error)
}

// EnvironmentInterpolator defines the interface for env var interpolation
type EnvironmentInterpolator interface {
    // Interpolate replaces environment variables in content
    Interpolate(content string, env map[string]string) (string, error)
}
```

## Use Cases

```go
// pkg/engine/parser/usecases/parse.go

package usecases

import (
    "context"
    "fmt"
    "strings"

    "github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
    parserdomain "github.com/fertile-org/banyan/pkg/engine/parser/domain"
    "github.com/fertile-org/banyan/pkg/engine/parser/ports"
)

// ParseComposeUseCase implements compose file parsing
type ParseComposeUseCase struct {
    composeLoader ports.ComposeLoader
    yamlParser    ports.YAMLParser
    validator     ports.SchemaValidator
    interpolator  ports.EnvironmentInterpolator
}

func NewParseComposeUseCase(
    composeLoader ports.ComposeLoader,
    yamlParser ports.YAMLParser,
    validator ports.SchemaValidator,
    interpolator ports.EnvironmentInterpolator,
) *ParseComposeUseCase {
    return &ParseComposeUseCase{
        composeLoader: composeLoader,
        yamlParser:    yamlParser,
        validator:     validator,
        interpolator:  interpolator,
    }
}

// Parse parses compose and banyan files into domain services
func (uc *ParseComposeUseCase) Parse(ctx context.Context, composeContent, banyanContent string, opts ports.ParseOptions) (*ports.ParseResult, error) {
    // Interpolate environment variables
    if !opts.SkipInterpolation {
        var err error
        composeContent, err = uc.interpolator.Interpolate(composeContent, opts.Environment)
        if err != nil {
            return nil, fmt.Errorf("interpolation failed: %w", err)
        }
    }

    // Load compose file
    loadOpts := ports.LoadOptions{
        SkipInterpolation: true, // Already done
        SkipValidation:    opts.SkipValidation,
        WorkingDir:        opts.WorkingDir,
        Environment:       opts.Environment,
    }

    parsedCompose, err := uc.composeLoader.Load(composeContent, loadOpts)
    if err != nil {
        return nil, fmt.Errorf("failed to parse compose file: %w", err)
    }

    // Parse banyan extension if provided
    var parsedBanyan *parserdomain.ParsedBanyan
    if banyanContent != "" {
        parsedBanyan, err = uc.parseBanyan(banyanContent)
        if err != nil {
            return nil, fmt.Errorf("failed to parse banyan file: %w", err)
        }
    }

    // Validate
    var validation *parserdomain.ValidationResult
    if !opts.SkipValidation {
        validation = uc.validate(parsedCompose, parsedBanyan)
        if !validation.Valid {
            return &ports.ParseResult{Validation: validation}, fmt.Errorf("validation failed")
        }
    }

    // Transform to domain services
    services := uc.transformServices(parsedCompose, parsedBanyan, opts.ProjectName)
    networks := uc.transformNetworks(parsedCompose, parsedBanyan)
    volumes := uc.transformVolumes(parsedCompose)

    return &ports.ParseResult{
        Services:   services,
        Networks:   networks,
        Volumes:    volumes,
        Validation: validation,
    }, nil
}

func (uc *ParseComposeUseCase) parseBanyan(content string) (*parserdomain.ParsedBanyan, error) {
    var banyan parserdomain.ParsedBanyan
    if err := uc.yamlParser.UnmarshalYAML(content, &banyan); err != nil {
        return nil, err
    }
    return &banyan, nil
}

func (uc *ParseComposeUseCase) validate(compose *parserdomain.ParsedCompose, banyan *parserdomain.ParsedBanyan) *parserdomain.ValidationResult {
    result := compose.Validate()

    if banyan != nil {
        // Validate banyan references match compose services
        for svcName := range banyan.Services {
            if _, exists := compose.Services[svcName]; !exists {
                result.AddError("banyan.services", svcName, fmt.Sprintf("service '%s' not found in compose file", svcName))
            }
        }

        // Validate VPC configuration
        if banyan.VPC.CIDR != "" {
            if _, _, err := net.ParseCIDR(banyan.VPC.CIDR); err != nil {
                result.AddError("banyan.vpc", "cidr", "invalid CIDR format")
            }
        }
    }

    return &result
}

// transformServices transforms parsed configs to domain services
func (uc *ParseComposeUseCase) transformServices(compose *parserdomain.ParsedCompose, banyan *parserdomain.ParsedBanyan, projectName string) []domain.Service {
    services := make([]domain.Service, 0, len(compose.Services))

    for name, svcConfig := range compose.Services {
        svc := domain.Service{
            Name:  name,
            Image: svcConfig.Image,
        }

        // Set project-scoped name
        if projectName != "" {
            svc.Name = fmt.Sprintf("%s_%s", projectName, name)
        }

        // Transform ports
        for _, port := range svcConfig.Ports {
            svc.Ports = append(svc.Ports, domain.PortMapping{
                ContainerPort: port.Target,
                HostPort:      port.Published,
                Protocol:      port.Protocol,
            })
        }

        // Transform environment
        svc.Environment = svcConfig.Environment

        // Transform volumes
        for _, vol := range svcConfig.Volumes {
            svc.Volumes = append(svc.Volumes, domain.VolumeMount{
                Source:   vol.Source,
                Target:   vol.Target,
                ReadOnly: vol.ReadOnly,
                Type:     vol.Type,
            })
        }

        // Transform health check
        if svcConfig.HealthCheck != nil && !svcConfig.HealthCheck.Disable {
            svc.HealthCheck = &domain.HealthCheck{
                Test:        svcConfig.HealthCheck.Test,
                Interval:    svcConfig.HealthCheck.Interval,
                Timeout:     svcConfig.HealthCheck.Timeout,
                Retries:     svcConfig.HealthCheck.Retries,
                StartPeriod: svcConfig.HealthCheck.StartPeriod,
            }
        }

        // Transform resource limits
        if svcConfig.Deploy != nil {
            svc.Resources = &domain.Resources{
                Limits: domain.ResourceSpec{
                    CPUs:   svcConfig.Deploy.Resources.Limits.CPUs,
                    Memory: svcConfig.Deploy.Resources.Limits.Memory,
                },
                Reservations: domain.ResourceSpec{
                    CPUs:   svcConfig.Deploy.Resources.Reservations.CPUs,
                    Memory: svcConfig.Deploy.Resources.Reservations.Memory,
                },
            }

            if svcConfig.Deploy.Replicas != nil {
                svc.Replicas = *svcConfig.Deploy.Replicas
            }
        }

        // Transform dependencies
        for dep, cfg := range svcConfig.DependsOn {
            svc.DependsOn = append(svc.DependsOn, domain.ServiceDependency{
                Service:   dep,
                Condition: cfg.Condition,
            })
        }

        // Apply banyan extensions
        if banyan != nil {
            if ext, exists := banyan.Services[name]; exists {
                svc.Placement = domain.Placement{
                    Constraints: ext.Placement.Constraints,
                    NodeLabels:  ext.Placement.NodeLabels,
                }

                if ext.Scaling.Max > 0 {
                    svc.Scaling = &domain.ScalingConfig{
                        MinReplicas: ext.Scaling.Min,
                        MaxReplicas: ext.Scaling.Max,
                        TargetCPU:   ext.Scaling.TargetCPU,
                        TargetMem:   ext.Scaling.TargetMem,
                    }
                }

                svc.SecurityGroup = ext.SecurityGroup
                svc.Subnet = ext.Subnet
            }
        }

        // Transform labels
        svc.Labels = svcConfig.Labels

        // Transform networks
        for netName, netConfig := range svcConfig.Networks {
            svc.Networks = append(svc.Networks, domain.ServiceNetwork{
                Name:        netName,
                Aliases:     netConfig.Aliases,
                IPv4Address: netConfig.IPV4Address,
            })
        }

        services = append(services, svc)
    }

    return services
}

func (uc *ParseComposeUseCase) transformNetworks(compose *parserdomain.ParsedCompose, banyan *parserdomain.ParsedBanyan) []domain.NetworkDefinition {
    networks := make([]domain.NetworkDefinition, 0, len(compose.Networks))

    for name, netConfig := range compose.Networks {
        net := domain.NetworkDefinition{
            Name:     name,
            Driver:   netConfig.Driver,
            External: netConfig.External,
            Internal: netConfig.Internal,
            Labels:   netConfig.Labels,
        }

        if netConfig.IPAM != nil && len(netConfig.IPAM.Config) > 0 {
            net.Subnet = netConfig.IPAM.Config[0].Subnet
            net.Gateway = netConfig.IPAM.Config[0].Gateway
        }

        networks = append(networks, net)
    }

    return networks
}

func (uc *ParseComposeUseCase) transformVolumes(compose *parserdomain.ParsedCompose) []domain.VolumeDefinition {
    volumes := make([]domain.VolumeDefinition, 0, len(compose.Volumes))

    for name, volConfig := range compose.Volumes {
        vol := domain.VolumeDefinition{
            Name:     name,
            Driver:   volConfig.Driver,
            External: volConfig.External,
            Labels:   volConfig.Labels,
        }
        volumes = append(volumes, vol)
    }

    return volumes
}
```

## Driven Adapters

### ComposeGo Adapter

```go
// pkg/engine/parser/adapters/composego.go

package adapters

import (
    "fmt"
    "strings"

    "github.com/compose-spec/compose-go/v2/loader"
    "github.com/compose-spec/compose-go/v2/types"

    "github.com/fertile-org/banyan/pkg/engine/parser/domain"
    "github.com/fertile-org/banyan/pkg/engine/parser/ports"
)

// ComposeGoAdapter implements ComposeLoader using compose-go library
type ComposeGoAdapter struct{}

func NewComposeGoAdapter() *ComposeGoAdapter {
    return &ComposeGoAdapter{}
}

// Load loads a compose file from content
func (a *ComposeGoAdapter) Load(content string, opts ports.LoadOptions) (*domain.ParsedCompose, error) {
    // Create config details
    configDetails := types.ConfigDetails{
        WorkingDir: opts.WorkingDir,
        ConfigFiles: []types.ConfigFile{
            {
                Content: []byte(content),
            },
        },
        Environment: opts.Environment,
    }

    // Set loader options
    loaderOpts := []func(*loader.Options){
        loader.WithSkipValidation(opts.SkipValidation),
    }

    if opts.SkipInterpolation {
        loaderOpts = append(loaderOpts, loader.WithSkipInterpolation())
    }

    // Load the project
    project, err := loader.Load(configDetails, loaderOpts...)
    if err != nil {
        return nil, fmt.Errorf("compose-go loader failed: %w", err)
    }

    return a.convertProject(project), nil
}

// LoadFromFile loads a compose file from path
func (a *ComposeGoAdapter) LoadFromFile(path string, opts ports.LoadOptions) (*domain.ParsedCompose, error) {
    configDetails := types.ConfigDetails{
        WorkingDir: opts.WorkingDir,
        ConfigFiles: []types.ConfigFile{
            {Filename: path},
        },
        Environment: opts.Environment,
    }

    loaderOpts := []func(*loader.Options){
        loader.WithSkipValidation(opts.SkipValidation),
    }

    if opts.SkipInterpolation {
        loaderOpts = append(loaderOpts, loader.WithSkipInterpolation())
    }

    project, err := loader.Load(configDetails, loaderOpts...)
    if err != nil {
        return nil, fmt.Errorf("compose-go loader failed: %w", err)
    }

    return a.convertProject(project), nil
}

// convertProject converts compose-go Project to domain ParsedCompose
func (a *ComposeGoAdapter) convertProject(project *types.Project) *domain.ParsedCompose {
    parsed := &domain.ParsedCompose{
        Services: make(map[string]domain.ServiceConfig),
        Networks: make(map[string]domain.NetworkConfig),
        Volumes:  make(map[string]domain.VolumeConfig),
    }

    // Convert services
    for _, svc := range project.Services {
        parsed.Services[svc.Name] = a.convertService(svc)
    }

    // Convert networks
    for name, net := range project.Networks {
        parsed.Networks[name] = a.convertNetwork(net)
    }

    // Convert volumes
    for name, vol := range project.Volumes {
        parsed.Volumes[name] = a.convertVolume(vol)
    }

    return parsed
}

func (a *ComposeGoAdapter) convertService(svc types.ServiceConfig) domain.ServiceConfig {
    config := domain.ServiceConfig{
        Name:        svc.Name,
        Image:       svc.Image,
        Command:     svc.Command,
        Entrypoint:  svc.Entrypoint,
        Environment: a.convertEnvironment(svc.Environment),
        Labels:      svc.Labels,
        Restart:     svc.Restart,
        Privileged:  svc.Privileged,
        ReadOnly:    svc.ReadOnly,
        User:        svc.User,
        WorkingDir:  svc.WorkingDir,
        Hostname:    svc.Hostname,
        DomainName:  svc.DomainName,
        DNS:         svc.DNS,
        DNSSearch:   svc.DNSSearch,
        CapAdd:      svc.CapAdd,
        CapDrop:     svc.CapDrop,
        Sysctls:     svc.Sysctls,
    }

    // Convert ports
    for _, port := range svc.Ports {
        config.Ports = append(config.Ports, domain.PortConfig{
            Target:    port.Target,
            Published: port.Published,
            Protocol:  port.Protocol,
            Mode:      port.Mode,
        })
    }

    // Convert volumes
    for _, vol := range svc.Volumes {
        config.Volumes = append(config.Volumes, domain.VolumeMount{
            Type:     vol.Type,
            Source:   vol.Source,
            Target:   vol.Target,
            ReadOnly: vol.ReadOnly,
        })
    }

    // Convert networks
    config.Networks = make(map[string]domain.ServiceNetworkConfig)
    for name, net := range svc.Networks {
        if net != nil {
            config.Networks[name] = domain.ServiceNetworkConfig{
                Aliases:     net.Aliases,
                IPV4Address: net.Ipv4Address,
                IPV6Address: net.Ipv6Address,
                Priority:    net.Priority,
            }
        } else {
            config.Networks[name] = domain.ServiceNetworkConfig{}
        }
    }

    // Convert depends_on
    config.DependsOn = make(map[string]domain.DependsOnConfig)
    for name, dep := range svc.DependsOn {
        config.DependsOn[name] = domain.DependsOnConfig{
            Condition: string(dep.Condition),
            Restart:   dep.Restart,
        }
    }

    // Convert deploy
    if svc.Deploy != nil {
        config.Deploy = &domain.DeployConfig{
            Replicas: svc.Deploy.Replicas,
            Labels:   svc.Deploy.Labels,
        }

        if svc.Deploy.Resources.Limits != nil {
            config.Deploy.Resources.Limits = domain.ResourceSpec{
                CPUs:   string(svc.Deploy.Resources.Limits.NanoCPUs),
                Memory: string(svc.Deploy.Resources.Limits.MemoryBytes),
            }
        }

        if svc.Deploy.Resources.Reservations != nil {
            config.Deploy.Resources.Reservations = domain.ResourceSpec{
                CPUs:   string(svc.Deploy.Resources.Reservations.NanoCPUs),
                Memory: string(svc.Deploy.Resources.Reservations.MemoryBytes),
            }
        }
    }

    // Convert health check
    if svc.HealthCheck != nil {
        config.HealthCheck = &domain.HealthCheckConfig{
            Test:        svc.HealthCheck.Test,
            Interval:    svc.HealthCheck.Interval.String(),
            Timeout:     svc.HealthCheck.Timeout.String(),
            Retries:     int(svc.HealthCheck.Retries),
            StartPeriod: svc.HealthCheck.StartPeriod.String(),
            Disable:     svc.HealthCheck.Disable,
        }
    }

    // Convert build
    if svc.Build != nil {
        config.Build = &domain.BuildConfig{
            Context:    svc.Build.Context,
            Dockerfile: svc.Build.Dockerfile,
            Args:       a.convertBuildArgs(svc.Build.Args),
            Target:     svc.Build.Target,
            CacheFrom:  svc.Build.CacheFrom,
            Labels:     svc.Build.Labels,
            Network:    svc.Build.Network,
        }
    }

    // Convert extra hosts
    for _, host := range svc.ExtraHosts {
        config.ExtraHosts = append(config.ExtraHosts, host)
    }

    return config
}

func (a *ComposeGoAdapter) convertEnvironment(env types.MappingWithEquals) map[string]string {
    result := make(map[string]string)
    for k, v := range env {
        if v != nil {
            result[k] = *v
        } else {
            result[k] = ""
        }
    }
    return result
}

func (a *ComposeGoAdapter) convertBuildArgs(args types.MappingWithEquals) map[string]string {
    result := make(map[string]string)
    for k, v := range args {
        if v != nil {
            result[k] = *v
        }
    }
    return result
}

func (a *ComposeGoAdapter) convertNetwork(net types.NetworkConfig) domain.NetworkConfig {
    config := domain.NetworkConfig{
        Name:       net.Name,
        Driver:     net.Driver,
        External:   net.External.External,
        Internal:   net.Internal,
        Labels:     net.Labels,
        DriverOpts: net.DriverOpts,
    }

    if net.Ipam.Driver != "" || len(net.Ipam.Config) > 0 {
        config.IPAM = &domain.IPAMConfig{
            Driver: net.Ipam.Driver,
        }

        for _, ipamConfig := range net.Ipam.Config {
            config.IPAM.Config = append(config.IPAM.Config, domain.IPAMPoolConfig{
                Subnet:  ipamConfig.Subnet,
                Gateway: ipamConfig.Gateway,
                IPRange: ipamConfig.IPRange,
            })
        }
    }

    return config
}

func (a *ComposeGoAdapter) convertVolume(vol types.VolumeConfig) domain.VolumeConfig {
    return domain.VolumeConfig{
        Name:       vol.Name,
        Driver:     vol.Driver,
        DriverOpts: vol.DriverOpts,
        External:   vol.External.External,
        Labels:     vol.Labels,
    }
}
```

### YAML Parser Adapter

```go
// pkg/engine/parser/adapters/yaml.go

package adapters

import (
    "fmt"

    "gopkg.in/yaml.v3"
)

// YAMLParserAdapter implements YAMLParser using gopkg.in/yaml.v3
type YAMLParserAdapter struct{}

func NewYAMLParserAdapter() *YAMLParserAdapter {
    return &YAMLParserAdapter{}
}

// ParseYAML parses YAML content into a generic map
func (a *YAMLParserAdapter) ParseYAML(content string) (map[string]interface{}, error) {
    var result map[string]interface{}
    if err := yaml.Unmarshal([]byte(content), &result); err != nil {
        return nil, fmt.Errorf("yaml parse error: %w", err)
    }
    return result, nil
}

// UnmarshalYAML unmarshals YAML into a specific type
func (a *YAMLParserAdapter) UnmarshalYAML(content string, v interface{}) error {
    if err := yaml.Unmarshal([]byte(content), v); err != nil {
        return fmt.Errorf("yaml unmarshal error: %w", err)
    }
    return nil
}
```

### Environment Interpolator Adapter

```go
// pkg/engine/parser/adapters/interpolator.go

package adapters

import (
    "os"
    "regexp"
    "strings"
)

// EnvInterpolatorAdapter implements EnvironmentInterpolator
type EnvInterpolatorAdapter struct {
    envPattern *regexp.Regexp
}

func NewEnvInterpolatorAdapter() *EnvInterpolatorAdapter {
    // Match ${VAR}, ${VAR:-default}, ${VAR-default}, $VAR
    pattern := regexp.MustCompile(`\$\{([^}:]+)(?::?-([^}]*))?\}|\$([A-Za-z_][A-Za-z0-9_]*)`)
    return &EnvInterpolatorAdapter{
        envPattern: pattern,
    }
}

// Interpolate replaces environment variables in content
func (a *EnvInterpolatorAdapter) Interpolate(content string, env map[string]string) (string, error) {
    result := a.envPattern.ReplaceAllStringFunc(content, func(match string) string {
        // Handle ${VAR} or ${VAR:-default} format
        if strings.HasPrefix(match, "${") {
            inner := match[2 : len(match)-1]

            // Check for default value
            var varName, defaultVal string
            var hasDefault bool

            if idx := strings.Index(inner, ":-"); idx != -1 {
                varName = inner[:idx]
                defaultVal = inner[idx+2:]
                hasDefault = true
            } else if idx := strings.Index(inner, "-"); idx != -1 {
                varName = inner[:idx]
                defaultVal = inner[idx+1:]
                hasDefault = true
            } else {
                varName = inner
            }

            // Look up value
            if val, exists := env[varName]; exists {
                return val
            }
            if val, exists := os.LookupEnv(varName); exists {
                return val
            }
            if hasDefault {
                return defaultVal
            }
            return match // Keep original if not found
        }

        // Handle $VAR format
        varName := match[1:]
        if val, exists := env[varName]; exists {
            return val
        }
        if val, exists := os.LookupEnv(varName); exists {
            return val
        }
        return match
    })

    return result, nil
}
```

## Flow Diagrams

### Parse Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Parse Flow                                      │
└─────────────────────────────────────────────────────────────────────────────┘

  ┌─────────────┐      ┌─────────────────┐      ┌─────────────────┐
  │ Compose     │      │ Banyan          │      │ Environment     │
  │ Content     │      │ Content         │      │ Variables       │
  └──────┬──────┘      └────────┬────────┘      └────────┬────────┘
         │                      │                        │
         │                      │                        │
         ▼                      ▼                        │
  ┌─────────────────────────────────────────────────────┐│
  │              Environment Interpolation               ││
  │         (Replace ${VAR} with actual values)          │◄┘
  └──────────────────────────┬──────────────────────────┘
                             │
         ┌───────────────────┴───────────────────┐
         │                                       │
         ▼                                       ▼
  ┌─────────────────────┐              ┌─────────────────────┐
  │   compose-go        │              │   YAML Parser       │
  │   Loader            │              │   (gopkg.in/yaml)   │
  │                     │              │                     │
  │ - Parse compose     │              │ - Parse banyan.yaml │
  │ - Validate schema   │              │ - Map to struct     │
  │ - Build project     │              │                     │
  └──────────┬──────────┘              └──────────┬──────────┘
             │                                    │
             ▼                                    ▼
  ┌─────────────────────┐              ┌─────────────────────┐
  │   ParsedCompose     │              │   ParsedBanyan      │
  │   - Services        │              │   - VPC config      │
  │   - Networks        │              │   - Extensions      │
  │   - Volumes         │              │   - Plugins         │
  └──────────┬──────────┘              └──────────┬──────────┘
             │                                    │
             └────────────────┬───────────────────┘
                              │
                              ▼
                   ┌─────────────────────┐
                   │    Validation       │
                   │ - Schema validation │
                   │ - Reference checks  │
                   │ - Cycle detection   │
                   └──────────┬──────────┘
                              │
                              ▼
                   ┌─────────────────────┐
                   │   Transformation    │
                   │ - Merge configs     │
                   │ - Apply extensions  │
                   │ - Build domain objs │
                   └──────────┬──────────┘
                              │
                              ▼
                   ┌─────────────────────┐
                   │    ParseResult      │
                   │ - []Service         │
                   │ - []Network         │
                   │ - []Volume          │
                   │ - ValidationResult  │
                   └─────────────────────┘
```

### Validation Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            Validation Flow                                   │
└─────────────────────────────────────────────────────────────────────────────┘

  ┌─────────────────────┐
  │   ParsedCompose     │
  └──────────┬──────────┘
             │
             ▼
  ┌─────────────────────────────────────────────────────────────┐
  │                    Service Validation                        │
  │  ┌─────────────────────────────────────────────────────┐    │
  │  │ For each service:                                    │    │
  │  │  ✓ Has image or build                               │    │
  │  │  ✓ Ports are valid (0 < port <= 65535)              │    │
  │  │  ✓ depends_on references exist                      │    │
  │  │  ✓ Network references exist                         │    │
  │  │  ✓ Health check is valid                            │    │
  │  │  ✓ Resource limits format correct                   │    │
  │  └─────────────────────────────────────────────────────┘    │
  └──────────────────────────┬──────────────────────────────────┘
                             │
                             ▼
  ┌─────────────────────────────────────────────────────────────┐
  │                    Network Validation                        │
  │  ┌─────────────────────────────────────────────────────┐    │
  │  │ For each network:                                    │    │
  │  │  ✓ IPAM subnet is valid CIDR                        │    │
  │  │  ✓ Gateway is within subnet                         │    │
  │  │  ✓ Driver is supported                              │    │
  │  └─────────────────────────────────────────────────────┘    │
  └──────────────────────────┬──────────────────────────────────┘
                             │
                             ▼
  ┌─────────────────────────────────────────────────────────────┐
  │                   Dependency Validation                      │
  │  ┌─────────────────────────────────────────────────────┐    │
  │  │ Check for circular dependencies:                     │    │
  │  │  A -> B -> C -> A  ❌ Invalid                        │    │
  │  │  A -> B -> C       ✓ Valid                           │    │
  │  └─────────────────────────────────────────────────────┘    │
  └──────────────────────────┬──────────────────────────────────┘
                             │
                             ▼
  ┌─────────────────────────────────────────────────────────────┐
  │                    Banyan Validation                         │
  │  ┌─────────────────────────────────────────────────────┐    │
  │  │ If banyan file provided:                             │    │
  │  │  ✓ Service extensions match compose services        │    │
  │  │  ✓ VPC CIDR is valid                                │    │
  │  │  ✓ Subnet CIDRs are within VPC CIDR                 │    │
  │  │  ✓ Plugin references are valid                      │    │
  │  └─────────────────────────────────────────────────────┘    │
  └──────────────────────────┬──────────────────────────────────┘
                             │
                             ▼
                  ┌─────────────────────┐
                  │  ValidationResult   │
                  │  - Valid: bool      │
                  │  - Errors: []Error  │
                  │  - Warnings: []Warn │
                  └─────────────────────┘
```

## Error Handling

```go
// pkg/engine/parser/errors/errors.go

package errors

import (
    "fmt"
)

// ParseError represents a parsing error
type ParseError struct {
    File    string
    Line    int
    Column  int
    Message string
    Cause   error
}

func (e *ParseError) Error() string {
    if e.Line > 0 {
        return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Column, e.Message)
    }
    return fmt.Sprintf("%s: %s", e.File, e.Message)
}

func (e *ParseError) Unwrap() error {
    return e.Cause
}

// ValidationError represents a validation error
type ValidationError struct {
    Path    string
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("%s.%s: %s", e.Path, e.Field, e.Message)
}

// InterpolationError represents an environment interpolation error
type InterpolationError struct {
    Variable string
    Message  string
}

func (e *InterpolationError) Error() string {
    return fmt.Sprintf("interpolation error for ${%s}: %s", e.Variable, e.Message)
}

// SchemaError represents a schema validation error
type SchemaError struct {
    Version string
    Message string
}

func (e *SchemaError) Error() string {
    return fmt.Sprintf("schema error (version %s): %s", e.Version, e.Message)
}

// Error constructors
func NewParseError(file, message string, cause error) *ParseError {
    return &ParseError{File: file, Message: message, Cause: cause}
}

func NewParseErrorWithLocation(file string, line, col int, message string) *ParseError {
    return &ParseError{File: file, Line: line, Column: col, Message: message}
}

func NewValidationError(path, field, message string) *ValidationError {
    return &ValidationError{Path: path, Field: field, Message: message}
}
```

## Testing Strategy

### Unit Tests

```go
// pkg/engine/parser/usecases/parse_test.go

package usecases_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/fertile-org/banyan/pkg/engine/parser/adapters"
    "github.com/fertile-org/banyan/pkg/engine/parser/ports"
    "github.com/fertile-org/banyan/pkg/engine/parser/usecases"
)

func TestParseComposeUseCase_Parse(t *testing.T) {
    tests := []struct {
        name           string
        composeContent string
        banyanContent  string
        opts           ports.ParseOptions
        wantServices   int
        wantErr        bool
    }{
        {
            name: "simple service",
            composeContent: `
version: "3.8"
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
`,
            wantServices: 1,
            wantErr:      false,
        },
        {
            name: "multiple services with dependencies",
            composeContent: `
version: "3.8"
services:
  web:
    image: nginx:latest
    depends_on:
      - api
  api:
    image: myapi:latest
    depends_on:
      - db
  db:
    image: postgres:15
`,
            wantServices: 3,
            wantErr:      false,
        },
        {
            name: "with banyan extensions",
            composeContent: `
version: "3.8"
services:
  web:
    image: nginx:latest
`,
            banyanContent: `
version: "1"
vpc:
  cidr: "10.0.0.0/16"
services:
  web:
    placement:
      constraints:
        - node.role == worker
    scaling:
      min: 2
      max: 10
`,
            wantServices: 1,
            wantErr:      false,
        },
        {
            name: "circular dependency",
            composeContent: `
version: "3.8"
services:
  a:
    image: a:latest
    depends_on:
      - b
  b:
    image: b:latest
    depends_on:
      - c
  c:
    image: c:latest
    depends_on:
      - a
`,
            wantServices: 0,
            wantErr:      true,
        },
        {
            name: "missing image and build",
            composeContent: `
version: "3.8"
services:
  web:
    ports:
      - "80:80"
`,
            wantServices: 0,
            wantErr:      true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            uc := usecases.NewParseComposeUseCase(
                adapters.NewComposeGoAdapter(),
                adapters.NewYAMLParserAdapter(),
                nil, // schema validator
                adapters.NewEnvInterpolatorAdapter(),
            )

            result, err := uc.Parse(context.Background(), tt.composeContent, tt.banyanContent, tt.opts)

            if tt.wantErr {
                assert.Error(t, err)
                return
            }

            require.NoError(t, err)
            assert.Len(t, result.Services, tt.wantServices)
        })
    }
}

func TestParseComposeUseCase_Interpolation(t *testing.T) {
    composeContent := `
version: "3.8"
services:
  web:
    image: ${IMAGE_NAME}:${IMAGE_TAG:-latest}
    environment:
      DB_HOST: ${DB_HOST:-localhost}
`

    uc := usecases.NewParseComposeUseCase(
        adapters.NewComposeGoAdapter(),
        adapters.NewYAMLParserAdapter(),
        nil,
        adapters.NewEnvInterpolatorAdapter(),
    )

    opts := ports.ParseOptions{
        Environment: map[string]string{
            "IMAGE_NAME": "myapp",
            "IMAGE_TAG":  "v1.0",
        },
    }

    result, err := uc.Parse(context.Background(), composeContent, "", opts)
    require.NoError(t, err)
    require.Len(t, result.Services, 1)

    assert.Equal(t, "myapp:v1.0", result.Services[0].Image)
    assert.Equal(t, "localhost", result.Services[0].Environment["DB_HOST"])
}
```

### Integration Tests

```go
// pkg/engine/parser/adapters/composego_test.go

package adapters_test

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/fertile-org/banyan/pkg/engine/parser/adapters"
    "github.com/fertile-org/banyan/pkg/engine/parser/ports"
)

func TestComposeGoAdapter_LoadFromFile(t *testing.T) {
    // Create temp compose file
    tmpDir := t.TempDir()
    composePath := filepath.Join(tmpDir, "docker-compose.yaml")

    content := `
version: "3.8"
services:
  web:
    image: nginx:latest
    ports:
      - "8080:80"
    networks:
      - frontend
    deploy:
      replicas: 3
      resources:
        limits:
          cpus: "0.5"
          memory: 512M

networks:
  frontend:
    driver: bridge
    ipam:
      config:
        - subnet: 172.28.0.0/16

volumes:
  data:
    driver: local
`

    err := os.WriteFile(composePath, []byte(content), 0644)
    require.NoError(t, err)

    adapter := adapters.NewComposeGoAdapter()

    result, err := adapter.LoadFromFile(composePath, ports.LoadOptions{
        WorkingDir: tmpDir,
    })

    require.NoError(t, err)
    assert.Len(t, result.Services, 1)
    assert.Len(t, result.Networks, 1)
    assert.Len(t, result.Volumes, 1)

    web := result.Services["web"]
    assert.Equal(t, "nginx:latest", web.Image)
    assert.Len(t, web.Ports, 1)
    assert.Equal(t, uint32(80), web.Ports[0].Target)
}
```

## Directory Structure

```
pkg/engine/parser/
├── domain/
│   ├── compose.go       # Compose entities
│   ├── banyan.go        # Banyan extension entities
│   ├── values.go        # Value objects
│   └── validation.go    # Validation logic
├── ports/
│   ├── inbound.go       # Service interfaces
│   └── outbound.go      # Adapter interfaces
├── usecases/
│   └── parse.go         # Parse use case
├── adapters/
│   ├── composego.go     # compose-go adapter
│   ├── yaml.go          # YAML parser adapter
│   └── interpolator.go  # Env interpolation adapter
├── errors/
│   └── errors.go        # Error types
└── parser.go            # Package entry point
```
