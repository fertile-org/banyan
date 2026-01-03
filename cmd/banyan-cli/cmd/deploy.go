package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fertile-org/banyan/pkg/vpc/storage"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Deploy configuration
var (
	deployFile         string
	deployEtcdEndpoint string
	deployDryRun       bool
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy applications from banyan.yaml",
	Long: `Deploy applications to Banyan using a banyan.yaml manifest.

Example banyan.yaml:
  name: my-app
  services:
    web:
      image: nginx:latest
      replicas: 3
      ports:
        - 80:80
      resources:
        memory: 256Mi
        cpu: 0.5
    api:
      image: my-api:v1
      replicas: 2
      ports:
        - 8080:8080
      env:
        - DATABASE_URL=postgres://db:5432/app

Usage:
  banyan-cli deploy --file banyan.yaml
  banyan-cli deploy --file banyan.yaml --dry-run`,
	RunE: runDeploy,
}

func init() {
	rootCmd.AddCommand(deployCmd)

	deployCmd.Flags().StringVarP(&deployFile, "file", "f", "banyan.yaml", "Path to banyan.yaml manifest")
	deployCmd.Flags().StringVar(&deployEtcdEndpoint, "etcd", "http://localhost:2379", "Engine etcd endpoint")
	deployCmd.Flags().BoolVar(&deployDryRun, "dry-run", false, "Validate manifest without deploying")
}

// BanyanManifest represents the banyan.yaml structure
type BanyanManifest struct {
	Name     string                   `yaml:"name"`
	Version  string                   `yaml:"version,omitempty"`
	Services map[string]ServiceConfig `yaml:"services"`
	Networks map[string]NetworkConfig `yaml:"networks,omitempty"`
}

// ServiceConfig represents a service in the manifest
type ServiceConfig struct {
	Image       string             `yaml:"image"`
	Replicas    int                `yaml:"replicas,omitempty"`
	Ports       []string           `yaml:"ports,omitempty"`
	Env         []string           `yaml:"env,omitempty"`
	Command     []string           `yaml:"command,omitempty"`
	Resources   ResourceConfig     `yaml:"resources,omitempty"`
	HealthCheck *HealthCheckConfig `yaml:"health_check,omitempty"`
	DependsOn   []string           `yaml:"depends_on,omitempty"`
}

// ResourceConfig represents resource limits
type ResourceConfig struct {
	Memory string `yaml:"memory,omitempty"`
	CPU    string `yaml:"cpu,omitempty"`
}

// HealthCheckConfig represents health check configuration
type HealthCheckConfig struct {
	HTTP     string `yaml:"http,omitempty"`
	TCP      string `yaml:"tcp,omitempty"`
	Interval string `yaml:"interval,omitempty"`
	Timeout  string `yaml:"timeout,omitempty"`
}

// NetworkConfig represents network configuration
type NetworkConfig struct {
	CIDR   string `yaml:"cidr,omitempty"`
	Driver string `yaml:"driver,omitempty"`
}

// Deployment represents a deployment in the system
type Deployment struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Services  []string  `json:"services"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DeployService represents a service in a deployment
type DeployService struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	DeploymentID string        `json:"deployment_id"`
	Image        string        `json:"image"`
	Replicas     int           `json:"replicas"`
	Status       string        `json:"status"`
	Ports        []PortMapping `json:"ports,omitempty"`
	Environment  []EnvVar      `json:"environment,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// PortMapping represents a port mapping
type PortMapping struct {
	HostPort      string `json:"host_port"`
	ContainerPort string `json:"container_port"`
	Protocol      string `json:"protocol"`
}

// EnvVar represents an environment variable
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func runDeploy(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan Deploy")
	fmt.Println("========================================")

	// Read manifest
	fmt.Printf("Reading manifest: %s\n", deployFile)
	data, err := os.ReadFile(deployFile)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest BanyanManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Validate manifest
	if manifest.Name == "" {
		return fmt.Errorf("manifest must have a name")
	}
	if len(manifest.Services) == 0 {
		return fmt.Errorf("manifest must define at least one service")
	}

	fmt.Printf("Application: %s\n", manifest.Name)
	fmt.Printf("Services: %d\n", len(manifest.Services))
	for name, svc := range manifest.Services {
		replicas := svc.Replicas
		if replicas == 0 {
			replicas = 1
		}
		fmt.Printf("  - %s: %s (replicas: %d)\n", name, svc.Image, replicas)
	}

	if deployDryRun {
		fmt.Println("\n[DRY-RUN] Manifest is valid. No changes made.")
		return nil
	}

	// Connect to Engine
	fmt.Printf("\nConnecting to Engine at %s...\n", deployEtcdEndpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := storage.NewEtcdStore([]string{deployEtcdEndpoint}, "/banyan")
	if err != nil {
		return fmt.Errorf("failed to connect to Engine: %w", err)
	}

	// Create deployment
	deployment := &Deployment{
		ID:        fmt.Sprintf("%s-%d", manifest.Name, time.Now().Unix()),
		Name:      manifest.Name,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Create services
	fmt.Println("\nCreating services...")
	for serviceName, svcConfig := range manifest.Services {
		replicas := svcConfig.Replicas
		if replicas == 0 {
			replicas = 1
		}

		service := &DeployService{
			ID:           fmt.Sprintf("%s-%s", manifest.Name, serviceName),
			Name:         serviceName,
			DeploymentID: deployment.ID,
			Image:        svcConfig.Image,
			Replicas:     replicas,
			Status:       "pending",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		// Parse ports
		for _, portStr := range svcConfig.Ports {
			parts := strings.Split(portStr, ":")
			if len(parts) == 2 {
				service.Ports = append(service.Ports, PortMapping{
					HostPort:      parts[0],
					ContainerPort: parts[1],
					Protocol:      "tcp",
				})
			}
		}

		// Parse environment variables
		for _, envStr := range svcConfig.Env {
			parts := strings.SplitN(envStr, "=", 2)
			if len(parts) == 2 {
				service.Environment = append(service.Environment, EnvVar{
					Name:  parts[0],
					Value: parts[1],
				})
			}
		}

		if err := store.Save(ctx, "/services/"+service.ID, service); err != nil {
			return fmt.Errorf("failed to save service %s: %w", serviceName, err)
		}
		fmt.Printf("  Created service: %s\n", serviceName)

		deployment.Services = append(deployment.Services, service.ID)
	}

	// Save deployment
	deployment.Status = "deploying"
	if err := store.Save(ctx, "/deployments/"+deployment.ID, deployment); err != nil {
		return fmt.Errorf("failed to save deployment: %w", err)
	}

	fmt.Println("\n========================================")
	fmt.Printf("Deployment '%s' created successfully!\n", manifest.Name)
	fmt.Printf("Deployment ID: %s\n", deployment.ID)
	fmt.Println("\nThe Engine will schedule containers to available agents.")
	fmt.Println("Use 'banyan-cli status' to check deployment status.")

	return nil
}
