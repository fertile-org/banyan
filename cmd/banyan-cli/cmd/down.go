package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
)

var (
	downName   string
	downFile   string
	downNoWait bool
	downTags   []string
)

var downCmd = &cobra.Command{
	Use:   "down [services...]",
	Short: "Stop and remove deployed services",
	Long: `Stop and remove services from a Banyan deployment.

By default, all services are stopped. Provide service names as arguments to stop only specific services.

Examples:
  banyan-cli down --name my-app
  banyan-cli down --name my-app web db
  banyan-cli down -f banyan.yaml web`,
	Args: cobra.ArbitraryArgs,
	RunE: runDown,
}

func init() {
	rootCmd.AddCommand(downCmd)

	downCmd.Flags().StringVar(&downName, "name", "", "Application name to stop")
	downCmd.Flags().StringVarP(&downFile, "file", "f", "", "Path to banyan.yaml manifest (to read app name)")
	downCmd.Flags().BoolVar(&downNoWait, "no-wait", false, "Don't wait for services to stop")
	downCmd.Flags().StringSliceVar(&downTags, "tags", nil, "Deployment tags for matching")
}

// resolveAppName determines the application name from explicit name or manifest file.
func resolveAppName(name, filePath string) (string, error) {
	if name != "" {
		return name, nil
	}
	if filePath == "" {
		return "", fmt.Errorf("either --name or --file must be provided")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read manifest: %w", err)
	}
	var manifest types.BanyanManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("failed to parse manifest: %w", err)
	}
	if manifest.Name == "" {
		return "", fmt.Errorf("manifest must have a name")
	}
	return manifest.Name, nil
}

func runDown(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan Down")
	fmt.Println("========================================")

	appName, err := resolveAppName(downName, downFile)
	if err != nil {
		return err
	}

	// Resolve engine endpoint from config
	engineAddr := types.GetCLIEngineEndpoint(configPath)
	if engineAddr == "" {
		return fmt.Errorf("engine endpoint not configured. Run 'banyan-cli init' to configure")
	}

	client, err := NewAutoEngineClient(engineAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to engine: %w", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Printf("Connecting to Engine at %s...\n", engineAddr)
	resp, err := client.Down(ctx, appName, args, downTags)
	if err != nil {
		return fmt.Errorf("failed to stop deployment: %w", err)
	}

	if resp.TaskCount == 0 {
		fmt.Println("No running services found to stop.")
		return nil
	}

	fmt.Printf("Created %d stop task(s) for deployment '%s'\n", resp.TaskCount, appName)

	if downNoWait {
		fmt.Println("Use 'banyan-cli status' to check progress.")
		return nil
	}

	fmt.Println("Waiting for services to stop...")
	return waitForDown(ctx, client, appName)
}

// findLatestDeployment returns the most recent deployment matching the given name+tags.
// If tags is nil, it matches deployments with any tags (backwards-compatible for status polling).
func findLatestDeployment(deployments []*banyanpb.DeploymentInfo, appName string, tags []string) *banyanpb.DeploymentInfo {
	var latest *banyanpb.DeploymentInfo
	for _, d := range deployments {
		if d.Name != appName {
			continue
		}
		if tags != nil && !types.TagsEqual(d.Tags, tags) {
			continue
		}
		if latest == nil || d.CreatedAtUnix > latest.CreatedAtUnix {
			latest = d
		}
	}
	return latest
}

func waitForDown(ctx context.Context, client *EngineClient, appName string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastStatus := ""
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nTimeout waiting for services to stop.")
			return fmt.Errorf("down timed out")
		case <-ticker.C:
			status, err := client.Status(ctx)
			if err != nil {
				continue
			}

			d := findLatestDeployment(status.Deployments, appName, downTags)
			if d == nil {
				continue
			}

			if d.Status != lastStatus {
				lastStatus = d.Status
				switch d.Status {
				case types.StatusStopped:
					fmt.Println("\n========================================")
					fmt.Printf("All services stopped for '%s'.\n", appName)
					return nil
				case types.StatusFailed:
					fmt.Printf("\n========================================\n")
					fmt.Printf("Down FAILED: %s\n", d.Error)
					return fmt.Errorf("down failed: %s", d.Error)
				}
			}
		}
	}
}
