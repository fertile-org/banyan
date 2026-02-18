package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fertile-org/banyan/pkg/vpc/storage"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	deployFile         string
	deployEtcdEndpoint string
	deployDryRun       bool
	deployNoWait       bool
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
    api:
      image: my-api:v1
      replicas: 2
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
	deployCmd.Flags().BoolVar(&deployNoWait, "no-wait", false, "Don't wait for deployment to complete")
}

// BanyanManifest represents the banyan.yaml structure
type BanyanManifest struct {
	Name     string                       `yaml:"name"`
	Version  string                       `yaml:"version,omitempty"`
	Services map[string]ManifestService   `yaml:"services"`
	Networks map[string]ManifestNetwork   `yaml:"networks,omitempty"`
}

// ManifestService represents a service in the manifest
type ManifestService struct {
	Image       string   `yaml:"image"`
	Replicas    int      `yaml:"replicas,omitempty"`
	Ports       []string `yaml:"ports,omitempty"`
	Env         []string `yaml:"env,omitempty"`
	Command     []string `yaml:"command,omitempty"`
	DependsOn   []string `yaml:"depends_on,omitempty"`
}

// ManifestNetwork represents network configuration
type ManifestNetwork struct {
	CIDR   string `yaml:"cidr,omitempty"`
	Driver string `yaml:"driver,omitempty"`
}

func runDeploy(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan Deploy")
	fmt.Println("========================================")

	// Read and parse manifest
	fmt.Printf("Reading manifest: %s\n", deployFile)
	data, err := os.ReadFile(deployFile)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest BanyanManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	if manifest.Name == "" {
		return fmt.Errorf("manifest must have a name")
	}
	if len(manifest.Services) == 0 {
		return fmt.Errorf("manifest must define at least one service")
	}

	// Build deployment record
	deploymentID := fmt.Sprintf("%s-%d", manifest.Name, time.Now().Unix())
	services := buildServiceRecords(manifest.Services)

	fmt.Printf("Application: %s\n", manifest.Name)
	fmt.Printf("Services: %d\n", len(manifest.Services))
	for name, svc := range services {
		fmt.Printf("  - %s: %s (replicas: %d)\n", name, svc.Image, svc.Replicas)
	}

	if deployDryRun {
		fmt.Println("\n[DRY-RUN] Manifest is valid. No changes made.")
		return nil
	}

	// Connect to etcd
	fmt.Printf("\nConnecting to Engine at %s...\n", deployEtcdEndpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store, err := storage.NewEtcdStore([]string{deployEtcdEndpoint}, "/banyan")
	if err != nil {
		return fmt.Errorf("failed to connect to Engine: %w", err)
	}

	// Save deployment record
	record := &DeploymentRecord{
		ID:        deploymentID,
		Name:      manifest.Name,
		Status:    statusPending,
		Services:  services,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := store.Save(ctx, keyDeployments+deploymentID, record); err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	fmt.Printf("\nDeployment '%s' created (ID: %s)\n", manifest.Name, deploymentID)

	if deployNoWait {
		fmt.Println("Use 'banyan-cli engine status' to check deployment status.")
		return nil
	}

	// Poll for status changes
	fmt.Println("Waiting for deployment to complete...")
	return waitForDeployment(ctx, store, deploymentID)
}

func waitForDeployment(ctx context.Context, store storage.StateStore, deploymentID string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastStatus := ""
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nTimeout waiting for deployment.")
			return fmt.Errorf("deployment timed out")
		case <-ticker.C:
			var record DeploymentRecord
			if err := store.Get(ctx, keyDeployments+deploymentID, &record); err != nil {
				continue // retry
			}

			if record.Status != lastStatus {
				lastStatus = record.Status
				switch record.Status {
				case statusDeploying:
					fmt.Println("  Status: deploying (tasks dispatched to agents)")
				case statusRunning:
					fmt.Println("  Status: running")
					fmt.Println("\n========================================")
					fmt.Printf("Deployment '%s' is RUNNING!\n", record.Name)
					return nil
				case statusFailed:
					fmt.Printf("  Status: FAILED (%s)\n", record.Error)
					fmt.Println("\n========================================")
					return fmt.Errorf("deployment failed: %s", record.Error)
				}
			}
		}
	}
}
