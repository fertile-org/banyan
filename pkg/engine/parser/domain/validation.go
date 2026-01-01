// Package domain defines the core entities for the Compose Parser.
package domain

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// ValidationResult contains validation results.
type ValidationResult struct {
	Valid    bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

// ValidationError represents a validation error.
type ValidationError struct {
	Field   string
	Message string
	Path    string
}

// ValidationWarning represents a validation warning.
type ValidationWarning struct {
	Field   string
	Message string
	Path    string
}

// Validate validates the parsed compose configuration.
func (pc *ParsedCompose) Validate() ValidationResult {
	result := ValidationResult{Valid: true}

	// Validate services
	for name := range pc.Services {
		svc := pc.Services[name]
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
			if err := validateResources(&svc.Deploy.Resources); err != nil {
				result.AddError(path, "deploy.resources", err.Error())
			}
		}
	}

	// Validate networks
	for name, netCfg := range pc.Networks {
		path := fmt.Sprintf("networks.%s", name)

		if netCfg.IPAM != nil {
			for i, pool := range netCfg.IPAM.Config {
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

// AddError adds a validation error.
func (r *ValidationResult) AddError(path, field, message string) {
	r.Valid = false
	r.Errors = append(r.Errors, ValidationError{
		Path:    path,
		Field:   field,
		Message: message,
	})
}

// AddWarning adds a validation warning.
func (r *ValidationResult) AddWarning(path, field, message string) {
	r.Warnings = append(r.Warnings, ValidationWarning{
		Path:    path,
		Field:   field,
		Message: message,
	})
}

// detectCycles detects circular dependencies in service depends_on.
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
					cycle := make([]string, len(path[cycleStart:])+1)
					copy(cycle, path[cycleStart:])
					cycle[len(cycle)-1] = dep
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

func validateResources(res *ResourcesConfig) error {
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

// ValidateBanyan validates banyan extensions against the compose file.
func ValidateBanyan(compose *ParsedCompose, banyan *ParsedBanyan) ValidationResult {
	result := ValidationResult{Valid: true}

	if banyan == nil {
		return result
	}

	// Validate service extensions match compose services
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

	// Validate subnet CIDRs
	for i, subnet := range banyan.VPC.Subnets {
		if subnet.CIDR != "" {
			if _, _, err := net.ParseCIDR(subnet.CIDR); err != nil {
				result.AddError("banyan.vpc.subnets", fmt.Sprintf("[%d].cidr", i), "invalid subnet CIDR format")
			}
		}
	}

	return result
}
