package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

var (
	downName         string
	downFile         string
	downEtcdEndpoint string
	downNoWait       bool
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy applications from banyan.yaml",
	Long: `Deploy applications to Banyan using a banyan.yaml manifest.

Example banyan.yaml:
  name: my-app
  services:
    web:
      build: ./web
      ports:
        - "80:80"
      depends_on:
        - api
    api:
      build: ./api
      replicas: 3
      ports:
        - "8080:8080"
      env:
        - DB_HOST=my-app-db-0
        - DB_PORT=5432
      depends_on:
        - db
    db:
      image: postgres:15-alpine
      ports:
        - "5432:5432"
      env:
        - POSTGRES_USER=banyan
        - POSTGRES_PASSWORD=secret
        - POSTGRES_DB=app

Usage:
  banyan-cli deploy --file banyan.yaml
  banyan-cli deploy --file banyan.yaml --dry-run`,
	RunE: runDeploy,
}

var downCmd = &cobra.Command{
	Use:   "down [services...]",
	Short: "Stop and remove deployed services",
	Long: `Stop and remove services from a Banyan deployment.

By default, all services are stopped. Provide service names as arguments to stop only specific services.

Examples:
  banyan-cli down --name my-app           # stops all services
  banyan-cli down --name my-app web db    # stops only web and db
  banyan-cli down -f banyan.yaml web      # stops only web (reads name from file)`,
	Args: cobra.ArbitraryArgs,
	RunE: runDown,
}

func init() {
	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(downCmd)

	deployCmd.Flags().StringVarP(&deployFile, "file", "f", "banyan.yaml", "Path to banyan.yaml manifest")
	deployCmd.Flags().StringVar(&deployEtcdEndpoint, "etcd", "http://localhost:2379", "Engine etcd endpoint")
	deployCmd.Flags().BoolVar(&deployDryRun, "dry-run", false, "Validate manifest without deploying")
	deployCmd.Flags().BoolVar(&deployNoWait, "no-wait", false, "Don't wait for deployment to complete")

	downCmd.Flags().StringVar(&downName, "name", "", "Application name to stop")
	downCmd.Flags().StringVarP(&downFile, "file", "f", "", "Path to banyan.yaml manifest (to read app name)")
	downCmd.Flags().StringVar(&downEtcdEndpoint, "etcd", "http://localhost:2379", "Engine etcd endpoint")
	downCmd.Flags().BoolVar(&downNoWait, "no-wait", false, "Don't wait for services to stop")
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
	Image       string         `yaml:"image"`
	Build       *ManifestBuild `yaml:"build,omitempty"`
	Replicas    int            `yaml:"replicas,omitempty"`
	Ports       []string       `yaml:"ports,omitempty"`
	Env         []string       `yaml:"env,omitempty"`
	Command     []string       `yaml:"command,omitempty"`
	DependsOn   []string       `yaml:"depends_on,omitempty"`
}

// ManifestBuild represents build configuration for a service.
// Supports both string form (build: ./path) and object form (build: {context: ./path}).
type ManifestBuild struct {
	Context    string `yaml:"context"`
	Dockerfile string `yaml:"dockerfile,omitempty"`
}

func (b *ManifestBuild) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		b.Context = value.Value
		return nil
	}
	// Decode as struct for object form
	type raw ManifestBuild
	return value.Decode((*raw)(b))
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

	// Validate: each service needs either image or build
	for name, svc := range manifest.Services {
		if svc.Image == "" && svc.Build == nil {
			return fmt.Errorf("service %q must have either 'image' or 'build'", name)
		}
	}

	// Build images for services with build config
	manifestDir := filepath.Dir(deployFile)
	if err := buildServiceImages(manifestDir, manifest.Name, manifest.Services); err != nil {
		return err
	}

	// Connect to etcd (needed for registry URL lookup before building service records)
	fmt.Printf("\nConnecting to Engine at %s...\n", deployEtcdEndpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store, err := storage.NewEtcdStore([]string{deployEtcdEndpoint}, "/banyan")
	if err != nil {
		return fmt.Errorf("failed to connect to Engine: %w", err)
	}

	// Push built images to the engine's OCI registry
	if !deployDryRun {
		if err := pushServiceImages(ctx, store, manifest.Services); err != nil {
			return err
		}
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

// buildServiceImages builds Docker images for services that have a build config.
// Tries nerdctl build first (images go directly into containerd).
// Falls back to docker build + import into containerd via docker save | nerdctl load.
func buildServiceImages(manifestDir, appName string, services map[string]ManifestService) error {
	needsBuild := false
	for _, svc := range services {
		if svc.Build != nil {
			needsBuild = true
			break
		}
	}
	if !needsBuild {
		return nil
	}

	fmt.Println("\nBuilding images...")

	for name, svc := range services {
		if svc.Build == nil {
			continue
		}

		// Determine image name
		imageName := svc.Image
		if imageName == "" {
			imageName = fmt.Sprintf("%s-%s:latest", appName, name)
			svc.Image = imageName
			services[name] = svc
		}

		// Resolve context path relative to manifest directory
		contextPath := svc.Build.Context
		if !filepath.IsAbs(contextPath) {
			contextPath = filepath.Join(manifestDir, contextPath)
		}

		fmt.Printf("  Building %s → %s\n", name, imageName)
		if err := buildImage(imageName, contextPath, svc.Build.Dockerfile); err != nil {
			return fmt.Errorf("failed to build image for service %q: %w", name, err)
		}
	}

	fmt.Println("Images built successfully.")
	return nil
}

// buildImage builds a container image using nerdctl build.
// Requires BuildKit (buildkitd) to be running.
func buildImage(imageName, contextPath, dockerfile string) error {
	args := []string{"build", "-t", imageName}
	if dockerfile != "" {
		args = append(args, "-f", filepath.Join(contextPath, dockerfile))
	}
	args = append(args, contextPath)

	buildCmd := exec.Command("nerdctl", args...)
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("nerdctl build failed (is buildkitd running?): %w", err)
	}
	return nil
}

// pushServiceImages reads the registry URL from etcd and pushes locally-built images
// to the engine's embedded OCI registry. It updates svc.Image to the registry-prefixed name
// so downstream service records use the correct image reference.
func pushServiceImages(ctx context.Context, store *storage.EtcdStore, services map[string]ManifestService) error {
	// Read registry URL from etcd
	var registryURL string
	if err := store.Get(ctx, keyRegistry, &registryURL); err != nil {
		fmt.Println("Warning: No registry URL found in etcd. Skipping image push.")
		fmt.Println("  (Engine may not have a registry enabled, or hasn't started yet)")
		return nil
	}

	hasBuild := false
	for _, svc := range services {
		if svc.Build != nil {
			hasBuild = true
			break
		}
	}
	if !hasBuild {
		return nil
	}

	fmt.Printf("\nPushing images to registry %s...\n", registryURL)

	for name, svc := range services {
		if svc.Build == nil {
			continue
		}

		localImage := svc.Image
		registryImage := fmt.Sprintf("%s/%s", registryURL, localImage)

		fmt.Printf("  Tagging %s → %s\n", localImage, registryImage)
		if err := tagImage(localImage, registryImage); err != nil {
			return fmt.Errorf("failed to tag image for service %q: %w", name, err)
		}

		fmt.Printf("  Pushing %s...\n", registryImage)
		if err := pushImage(registryImage); err != nil {
			return fmt.Errorf("failed to push image for service %q: %w", name, err)
		}

		// Update the service image to the registry-prefixed name
		svc.Image = registryImage
		services[name] = svc
	}

	fmt.Println("Images pushed successfully.")
	return nil
}

// tagImage runs nerdctl tag to create a registry-prefixed alias for a local image.
func tagImage(src, dst string) error {
	cmd := exec.Command("nerdctl", "tag", src, dst)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nerdctl tag failed: %w", err)
	}
	return nil
}

// pushImage runs nerdctl push with --insecure-registry to push an image to the registry.
func pushImage(image string) error {
	cmd := exec.Command("nerdctl", "push", "--insecure-registry", image)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nerdctl push failed: %w", err)
	}
	return nil
}

func runDown(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan Down")
	fmt.Println("========================================")

	// Determine app name from --name or --file
	appName := downName
	if appName == "" && downFile == "" {
		return fmt.Errorf("either --name or --file must be provided")
	}
	if appName == "" {
		data, err := os.ReadFile(downFile)
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
		appName = manifest.Name
	}

	// Connect to etcd
	fmt.Printf("Connecting to Engine at %s...\n", downEtcdEndpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store, err := storage.NewEtcdStore([]string{downEtcdEndpoint}, "/banyan")
	if err != nil {
		return fmt.Errorf("failed to connect to Engine: %w", err)
	}

	// Find deployment by name
	deployment, deploymentKey, err := findDeploymentByName(ctx, store, appName)
	if err != nil {
		return err
	}

	// Determine which services to stop
	serviceNames := args
	if len(serviceNames) > 0 {
		for _, name := range serviceNames {
			if _, ok := deployment.Services[name]; !ok {
				return fmt.Errorf("service %q not found in deployment %q", name, appName)
			}
		}
	}

	// Collect all tasks for this deployment
	tasks := collectDeploymentTasks(ctx, store, deployment.ID)

	// Filter to completed create_and_start tasks
	var targetTasks []TaskRecord
	for _, task := range tasks {
		if task.Type == taskTypeCreateAndStart && task.Status == statusCompleted {
			targetTasks = append(targetTasks, task)
		}
	}

	// If service names provided, filter further
	if len(serviceNames) > 0 {
		serviceSet := make(map[string]bool, len(serviceNames))
		for _, name := range serviceNames {
			serviceSet[name] = true
		}
		var filtered []TaskRecord
		for _, task := range targetTasks {
			if serviceSet[task.ServiceName] {
				filtered = append(filtered, task)
			}
		}
		targetTasks = filtered
	}

	if len(targetTasks) == 0 {
		fmt.Println("No running services found to stop.")
		return nil
	}

	// Create stop_and_remove tasks
	stoppingAll := len(serviceNames) == 0
	var stopTaskIDs []string
	now := time.Now()
	for _, task := range targetTasks {
		stopTask := &TaskRecord{
			ID:            task.ID + "-stop",
			DeploymentID:  task.DeploymentID,
			ServiceName:   task.ServiceName,
			AgentID:       task.AgentID,
			Type:          taskTypeStopAndRemove,
			Status:        statusPending,
			ContainerName: task.ContainerName,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		taskKey := keyTasks + task.AgentID + "/" + stopTask.ID
		if err := store.Save(ctx, taskKey, stopTask); err != nil {
			return fmt.Errorf("failed to create stop task for %s: %w", task.ContainerName, err)
		}
		stopTaskIDs = append(stopTaskIDs, stopTask.ID)
		fmt.Printf("  Stopping: %s on %s\n", task.ContainerName, task.AgentID)
	}

	// Update deployment status if stopping all services
	if stoppingAll {
		deployment.Status = statusStopping
		deployment.UpdatedAt = time.Now()
		if err := store.Save(ctx, deploymentKey, deployment); err != nil {
			return fmt.Errorf("failed to update deployment status: %w", err)
		}
	}

	fmt.Printf("\nCreated %d stop task(s) for deployment '%s'\n", len(stopTaskIDs), appName)

	if downNoWait {
		fmt.Println("Use 'banyan-cli engine status' to check progress.")
		return nil
	}

	fmt.Println("Waiting for services to stop...")
	return waitForDown(ctx, store, deployment.ID, stopTaskIDs)
}

// findDeploymentByName scans all deployments and returns the most recent one
// matching the given name. Returns the deployment, its etcd key, and any error.
func findDeploymentByName(ctx context.Context, store *storage.EtcdStore, name string) (*DeploymentRecord, string, error) {
	keys, err := store.List(ctx, keyDeployments)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list deployments: %w", err)
	}

	var best *DeploymentRecord
	var bestKey string
	for _, key := range keys {
		var record DeploymentRecord
		if err := store.Get(ctx, key, &record); err != nil {
			continue
		}
		if record.Name != name {
			continue
		}
		if best == nil || record.CreatedAt.After(best.CreatedAt) {
			r := record // copy
			best = &r
			bestKey = key
		}
	}

	if best == nil {
		return nil, "", fmt.Errorf("no deployment found with name %q", name)
	}
	return best, bestKey, nil
}

// waitForDown polls until all stop_and_remove tasks complete or times out.
func waitForDown(ctx context.Context, store *storage.EtcdStore, deploymentID string, stopTaskIDs []string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	wantIDs := make(map[string]bool, len(stopTaskIDs))
	for _, id := range stopTaskIDs {
		wantIDs[id] = true
	}

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nTimeout waiting for services to stop.")
			return fmt.Errorf("down timed out")
		case <-ticker.C:
			tasks := collectDeploymentTasks(ctx, store, deploymentID)

			completed := 0
			failed := 0
			var firstError string
			for _, task := range tasks {
				if !wantIDs[task.ID] {
					continue
				}
				switch task.Status {
				case statusCompleted:
					completed++
				case statusFailed:
					failed++
					if firstError == "" {
						firstError = task.Error
					}
				}
			}

			total := len(stopTaskIDs)
			if failed > 0 {
				fmt.Printf("\n========================================\n")
				fmt.Printf("Down FAILED: %d/%d tasks failed: %s\n", failed, total, firstError)
				return fmt.Errorf("down failed: %s", firstError)
			}
			if completed == total {
				fmt.Println("\n========================================")
				fmt.Printf("All %d service(s) stopped successfully.\n", total)
				return nil
			}
		}
	}
}
