package adapters

import (
	"fmt"
	"strings"

	"github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
	"github.com/fertile-org/banyan/pkg/engine/orchestrator/ports/outbound"
	"gopkg.in/yaml.v3"
)

// MemoryBanyanParser is a simple banyan.yml parser.
type MemoryBanyanParser struct{}

// NewMemoryBanyanParser creates a new parser.
func NewMemoryBanyanParser() *MemoryBanyanParser {
	return &MemoryBanyanParser{}
}

// banyanConfig represents the structure of banyan.yml.
type banyanConfig struct {
	Services map[string]banyanService `yaml:"services"`
}

type banyanService struct {
	Image       string             `yaml:"image"`
	Deploy      *banyanDeploy      `yaml:"deploy"`
	Ports       []string           `yaml:"ports"`
	Environment map[string]string  `yaml:"environment"`
	DependsOn   []string           `yaml:"depends_on"`
	HealthCheck *banyanHealthCheck `yaml:"healthcheck"`
}

type banyanDeploy struct {
	Replicas int `yaml:"replicas"`
}

type banyanHealthCheck struct {
	Test     string `yaml:"test"`
	Interval string `yaml:"interval"`
	Timeout  string `yaml:"timeout"`
	Retries  int    `yaml:"retries"`
}

// Parse parses banyan.yml content into services.
func (p *MemoryBanyanParser) Parse(banyanContent string) ([]domain.Service, error) {
	var config banyanConfig
	if err := yaml.Unmarshal([]byte(banyanContent), &config); err != nil {
		return nil, fmt.Errorf("failed to parse banyan.yml: %w", err)
	}

	var services []domain.Service
	for name, svc := range config.Services {
		replicas := 0
		if svc.Deploy != nil {
			replicas = svc.Deploy.Replicas
		}
		if replicas == 0 {
			replicas = 1 // Default
		}

		service := domain.Service{
			Name:         name,
			Image:        svc.Image,
			Replicas:     replicas,
			Dependencies: svc.DependsOn,
			Environment:  svc.Environment,
			Ports:        parsePorts(svc.Ports),
		}

		if svc.HealthCheck != nil {
			service.HealthCheck = &domain.HealthCheckSpec{
				Endpoint: svc.HealthCheck.Test,
				Retries:  svc.HealthCheck.Retries,
			}
		}

		services = append(services, service)
	}

	return services, nil
}

// parsePorts parses port mappings like "8080:80" into PortMapping.
func parsePorts(ports []string) []domain.PortMapping {
	var result []domain.PortMapping
	for _, p := range ports {
		parts := strings.Split(p, ":")
		if len(parts) == 2 {
			var hostPort, containerPort int
			fmt.Sscanf(parts[0], "%d", &hostPort)
			fmt.Sscanf(parts[1], "%d", &containerPort)
			result = append(result, domain.PortMapping{
				HostPort:      hostPort,
				ContainerPort: containerPort,
				Protocol:      "tcp",
			})
		}
	}
	return result
}

// Ensure MemoryBanyanParser implements BanyanParser.
var _ outbound.BanyanParser = (*MemoryBanyanParser)(nil)
