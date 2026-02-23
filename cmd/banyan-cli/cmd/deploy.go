package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/fertile-org/banyan/pkg/types"
)

var (
	deployFile   string
	deployDryRun bool
	deployNoWait bool
)

// Function variables for external commands, enabling test mocking.
var (
	buildImageFunc = buildImage
	tagImageFunc   = tagImage
	pushImageFunc  = pushImage
)

var deployCmd = &cobra.Command{
	Use:     "up",
	Aliases: []string{"deploy"},
	Short:   "Deploy applications from banyan.yaml",
	Long: `Deploy applications to Banyan using a banyan.yaml manifest.

If the application is already running, it will be stopped and redeployed
automatically (idempotent). Old containers are removed before new ones start.

Example banyan.yaml:
  name: my-app
  services:
    web:
      build: ./web
      ports:
        - "80:80"
    api:
      build: ./api
      deploy:
        replicas: 3
      ports:
        - "8080:8080"

Usage:
  banyan-cli up --file banyan.yaml
  banyan-cli up --file banyan.yaml --dry-run`,
	RunE: runDeploy,
}

func init() {
	rootCmd.AddCommand(deployCmd)

	deployCmd.Flags().StringVarP(&deployFile, "file", "f", "banyan.yaml", "Path to banyan.yaml manifest")
	deployCmd.Flags().BoolVar(&deployDryRun, "dry-run", false, "Validate manifest without deploying")
	deployCmd.Flags().BoolVar(&deployNoWait, "no-wait", false, "Don't wait for deployment to complete")
}

// validateManifest checks that a manifest has a name, services, and each service has an image or build config.
func validateManifest(manifest types.BanyanManifest) error {
	if manifest.Name == "" {
		return fmt.Errorf("manifest must have a name")
	}
	if len(manifest.Services) == 0 {
		return fmt.Errorf("manifest must define at least one service")
	}
	for name, svc := range manifest.Services { //nolint:gocritic // map iteration
		if svc.Image == "" && svc.Build == nil {
			return fmt.Errorf("service %q must have either 'image' or 'build'", name)
		}
	}
	return nil
}

// buildImageArgs constructs nerdctl build arguments.
func buildImageArgs(imageName, contextPath, dockerfile string) []string {
	args := []string{"build", "-t", imageName}
	if dockerfile != "" {
		args = append(args, "-f", filepath.Join(contextPath, dockerfile))
	}
	args = append(args, contextPath)
	return args
}

func runDeploy(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan Up")
	fmt.Println("========================================")

	// Read and parse manifest
	fmt.Printf("Reading manifest: %s\n", deployFile)
	data, err := os.ReadFile(deployFile)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest types.BanyanManifest
	if parseErr := yaml.Unmarshal(data, &manifest); parseErr != nil {
		return fmt.Errorf("failed to parse manifest: %w", parseErr)
	}

	err = validateManifest(manifest)
	if err != nil {
		return err
	}

	// Build images for services with build config
	manifestDir := filepath.Dir(deployFile)
	if buildErr := buildServiceImages(manifestDir, manifest.Name, manifest.Services); buildErr != nil {
		return buildErr
	}

	if deployDryRun {
		services := types.BuildServiceRecords(manifest.Services)
		fmt.Printf("Application: %s\n", manifest.Name)
		fmt.Printf("Services: %d\n", len(manifest.Services))
		for name, svc := range services {
			fmt.Printf("  - %s: %s (replicas: %d)\n", name, svc.Image, svc.Replicas)
		}
		fmt.Println("\n[DRY-RUN] Manifest is valid. No changes made.")
		return nil
	}

	// Resolve engine endpoint from config
	engineAddr := types.GetCLIEngineEndpoint(configPath)
	if engineAddr == "" {
		return fmt.Errorf("engine endpoint not configured. Run 'banyan-cli init' to configure")
	}

	token := types.GetCLIAuthToken(configPath)
	client, err := NewEngineClient(engineAddr, token)
	if err != nil {
		return fmt.Errorf("failed to connect to engine: %w", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Get registry URL from engine
	info, err := client.Info(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to engine: %w", err)
	}

	// Push built images to registry
	if pushErr := pushServiceImages(info.RegistryUrl, manifest.Services); pushErr != nil {
		return pushErr
	}

	// Print deployment info
	services := types.BuildServiceRecords(manifest.Services)
	fmt.Printf("\nApplication: %s\n", manifest.Name)
	fmt.Printf("Services: %d\n", len(manifest.Services))
	for name, svc := range services {
		fmt.Printf("  - %s: %s (replicas: %d)\n", name, svc.Image, svc.Replicas)
	}

	// Deploy via engine gRPC
	fmt.Printf("\nConnecting to Engine at %s...\n", engineAddr)
	resp, err := client.Deploy(ctx, manifest)
	if err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	fmt.Printf("Deployment '%s' created (ID: %s)\n", manifest.Name, resp.DeploymentId)

	if deployNoWait {
		fmt.Println("Use 'banyan-cli status' to check deployment status.")
		return nil
	}

	// Poll for status changes
	fmt.Println("Waiting for deployment to complete...")
	return waitForDeployment(ctx, client, manifest.Name)
}

func waitForDeployment(ctx context.Context, client *EngineClient, appName string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastStatus := ""
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nTimeout waiting for deployment.")
			return fmt.Errorf("deployment timed out")
		case <-ticker.C:
			status, err := client.Status(ctx)
			if err != nil {
				continue
			}

			d := findLatestDeployment(status.Deployments, appName)
			if d == nil {
				continue
			}

			if d.Status != lastStatus {
				lastStatus = d.Status
				switch d.Status {
				case types.StatusDeploying:
					fmt.Println("  Status: deploying (tasks dispatched to agents)")
				case types.StatusRunning:
					fmt.Println("  Status: running")
					fmt.Println("\n========================================")
					fmt.Printf("Deployment '%s' is RUNNING!\n", d.Name)
					return nil
				case types.StatusFailed:
					fmt.Printf("  Status: FAILED (%s)\n", d.Error)
					fmt.Println("\n========================================")
					return fmt.Errorf("deployment failed: %s", d.Error)
				}
			}
		}
	}
}

// buildServiceImages builds Docker images for services that have a build config.
func buildServiceImages(manifestDir, appName string, services map[string]types.ManifestService) error {
	needsBuild := false
	for _, svc := range services { //nolint:gocritic // map iteration
		if svc.Build != nil {
			needsBuild = true
			break
		}
	}
	if !needsBuild {
		return nil
	}

	fmt.Println("\nBuilding images...")

	for name, svc := range services { //nolint:gocritic // map iteration
		if svc.Build == nil {
			continue
		}

		imageName := svc.Image
		if imageName == "" {
			imageName = fmt.Sprintf("%s-%s:latest", appName, name)
			svc.Image = imageName
			services[name] = svc
		}

		contextPath := svc.Build.Context
		if !filepath.IsAbs(contextPath) {
			contextPath = filepath.Join(manifestDir, contextPath)
		}

		fmt.Printf("  Building %s → %s\n", name, imageName)
		if err := buildImageFunc(imageName, contextPath, svc.Build.Dockerfile); err != nil {
			return fmt.Errorf("failed to build image for service %q: %w", name, err)
		}
	}

	fmt.Println("Images built successfully.")
	return nil
}

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

// pushServiceImages pushes locally-built images to the registry.
func pushServiceImages(registryURL string, services map[string]types.ManifestService) error {
	hasBuild := false
	for _, svc := range services { //nolint:gocritic // map iteration
		if svc.Build != nil {
			hasBuild = true
			break
		}
	}
	if !hasBuild {
		return nil
	}

	if registryURL == "" {
		fmt.Println("Warning: No registry URL available. Skipping image push.")
		return nil
	}

	fmt.Printf("\nPushing images to registry %s...\n", registryURL)

	for name, svc := range services { //nolint:gocritic // map iteration
		if svc.Build == nil {
			continue
		}

		localImage := svc.Image
		registryImage := fmt.Sprintf("%s/%s", registryURL, localImage)

		fmt.Printf("  Tagging %s → %s\n", localImage, registryImage)
		if err := tagImageFunc(localImage, registryImage); err != nil {
			return fmt.Errorf("failed to tag image for service %q: %w", name, err)
		}

		fmt.Printf("  Pushing %s...\n", registryImage)
		if err := pushImageFunc(registryImage); err != nil {
			return fmt.Errorf("failed to push image for service %q: %w", name, err)
		}

		// Update the service image to the registry-prefixed name
		svc.Image = registryImage
		services[name] = svc
	}

	fmt.Println("Images pushed successfully.")
	return nil
}

func tagImage(src, dst string) error {
	cmd := exec.Command("nerdctl", "tag", src, dst)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nerdctl tag failed: %w", err)
	}
	return nil
}

func pushImage(image string) error {
	cmd := exec.Command("nerdctl", "push", "--insecure-registry", image)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nerdctl push failed: %w", err)
	}
	return nil
}
