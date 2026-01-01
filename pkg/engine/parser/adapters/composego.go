// Package adapters provides implementations of the outbound ports for the Compose Parser.
package adapters

import (
	"context"
	"fmt"
	"strconv"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"

	"github.com/fertile-org/banyan/pkg/engine/parser/domain"
	"github.com/fertile-org/banyan/pkg/engine/parser/ports/outbound"
)

// ComposeGoAdapter implements ComposeLoader using compose-go library.
type ComposeGoAdapter struct{}

// NewComposeGoAdapter creates a new ComposeGoAdapter.
func NewComposeGoAdapter() *ComposeGoAdapter {
	return &ComposeGoAdapter{}
}

// Load loads a compose file from content.
func (a *ComposeGoAdapter) Load(content string, opts outbound.LoadOptions) (*domain.ParsedCompose, error) {
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
	loaderOpts := func(o *loader.Options) {
		o.SkipValidation = opts.SkipValidation
		o.SkipInterpolation = opts.SkipInterpolation
		o.SetProjectName("project", false) // Default project name
	}

	// Load the project
	project, err := loader.LoadWithContext(context.Background(), configDetails, loaderOpts)
	if err != nil {
		return nil, fmt.Errorf("compose-go loader failed: %w", err)
	}

	return a.convertProject(project), nil
}

// LoadFromFile loads a compose file from path.
func (a *ComposeGoAdapter) LoadFromFile(path string, opts outbound.LoadOptions) (*domain.ParsedCompose, error) {
	configDetails := types.ConfigDetails{
		WorkingDir: opts.WorkingDir,
		ConfigFiles: []types.ConfigFile{
			{Filename: path},
		},
		Environment: opts.Environment,
	}

	loaderOpts := func(o *loader.Options) {
		o.SkipValidation = opts.SkipValidation
		o.SkipInterpolation = opts.SkipInterpolation
		o.SetProjectName("project", false) // Default project name
	}

	project, err := loader.LoadWithContext(context.Background(), configDetails, loaderOpts)
	if err != nil {
		return nil, fmt.Errorf("compose-go loader failed: %w", err)
	}

	return a.convertProject(project), nil
}

// convertProject converts compose-go Project to domain ParsedCompose.
func (a *ComposeGoAdapter) convertProject(project *types.Project) *domain.ParsedCompose {
	parsed := &domain.ParsedCompose{
		Services: make(map[string]domain.ServiceConfig),
		Networks: make(map[string]domain.NetworkConfig),
		Volumes:  make(map[string]domain.VolumeConfig),
	}

	// Convert services
	for name := range project.Services {
		svc := project.Services[name]
		parsed.Services[svc.Name] = a.convertService(&svc)
	}

	// Convert networks
	for name := range project.Networks {
		net := project.Networks[name]
		parsed.Networks[name] = a.convertNetwork(&net)
	}

	// Convert volumes
	for name, vol := range project.Volumes {
		parsed.Volumes[name] = a.convertVolume(vol)
	}

	return parsed
}

func (a *ComposeGoAdapter) convertService(svc *types.ServiceConfig) domain.ServiceConfig {
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
			Condition: dep.Condition,
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
				CPUs:   formatNanoCPUs(svc.Deploy.Resources.Limits.NanoCPUs),
				Memory: formatMemoryBytes(svc.Deploy.Resources.Limits.MemoryBytes),
			}
		}

		if svc.Deploy.Resources.Reservations != nil {
			config.Deploy.Resources.Reservations = domain.ResourceSpec{
				CPUs:   formatNanoCPUs(svc.Deploy.Resources.Reservations.NanoCPUs),
				Memory: formatMemoryBytes(svc.Deploy.Resources.Reservations.MemoryBytes),
			}
		}
	}

	// Convert health check
	if svc.HealthCheck != nil {
		var retries int
		if svc.HealthCheck.Retries != nil {
			// #nosec G115 - safe conversion as retries is always small
			retries = int(*svc.HealthCheck.Retries)
		}
		config.HealthCheck = &domain.HealthCheckConfig{
			Test:        svc.HealthCheck.Test,
			Interval:    svc.HealthCheck.Interval.String(),
			Timeout:     svc.HealthCheck.Timeout.String(),
			Retries:     retries,
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
	if svc.ExtraHosts != nil {
		config.ExtraHosts = svc.ExtraHosts.AsList(":")
	}

	return config
}

func formatNanoCPUs(nanoCPUs types.NanoCPUs) string {
	if nanoCPUs == 0 {
		return ""
	}
	return strconv.FormatFloat(float64(nanoCPUs), 'f', -1, 32)
}

func formatMemoryBytes(memBytes types.UnitBytes) string {
	if memBytes == 0 {
		return ""
	}
	return strconv.FormatInt(int64(memBytes), 10)
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

func (a *ComposeGoAdapter) convertNetwork(net *types.NetworkConfig) domain.NetworkConfig {
	config := domain.NetworkConfig{
		Name:       net.Name,
		Driver:     net.Driver,
		External:   bool(net.External),
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
		External:   bool(vol.External),
		Labels:     vol.Labels,
	}
}
