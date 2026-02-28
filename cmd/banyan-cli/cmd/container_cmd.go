package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/cmd/banyan-cli/dashboard"
)

var containerCmd = &cobra.Command{
	Use:   "container [NAME]",
	Short: "List containers or show container detail",
	Long: `List all containers in the cluster, or show detailed info for a specific container.

Examples:
  banyan-cli container
  banyan-cli container my-app-web-0
  banyan-cli container -o json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runContainer,
}

func init() {
	rootCmd.AddCommand(containerCmd)
	containerCmd.Flags().StringVarP(&outputFormat, "output", "o", "", "Output format (json)")
}

func runContainer(_ *cobra.Command, args []string) error {
	data, err := fetchClusterData()
	if err != nil {
		return err
	}

	if len(args) == 1 {
		return showContainerDetail(data.Containers, args[0])
	}
	return listContainers(data.Containers)
}

func listContainers(containers []dashboard.ContainerData) error {
	if outputFormat == "json" {
		return printJSON(containers)
	}

	if len(containers) == 0 {
		fmt.Println("No containers found")
		return nil
	}

	fmt.Printf("%-25s %-12s %-15s %-15s %s\n",
		"NAME", "SERVICE", "AGENT", "DEPLOYMENT", "STATUS")
	fmt.Println(strings.Repeat("-", 80))

	for i := range containers {
		c := &containers[i]
		status := c.ContainerStatus
		if status == "" {
			status = c.Status
		}
		fmt.Printf("%-25s %-12s %-15s %-15s %s\n",
			c.Name, c.ServiceName, c.AgentName, c.DeploymentName, status)
	}

	return nil
}

func showContainerDetail(containers []dashboard.ContainerData, name string) error {
	var container *dashboard.ContainerData
	for i := range containers {
		if containers[i].Name == name {
			container = &containers[i]
			break
		}
	}
	if container == nil {
		return fmt.Errorf("container %q not found", name)
	}

	if outputFormat == "json" {
		return printJSON(container)
	}

	fmt.Printf("Container: %s\n", container.Name)
	fmt.Println(strings.Repeat("=", 50))
	status := container.ContainerStatus
	if status == "" {
		status = container.Status
	}
	fmt.Printf("  Status:      %s\n", status)
	fmt.Printf("  Service:     %s\n", container.ServiceName)
	fmt.Printf("  Agent:       %s\n", container.AgentName)
	fmt.Printf("  Deployment:  %s\n", container.DeploymentName)
	fmt.Printf("  Image:       %s\n", container.Image)
	if len(container.Ports) > 0 {
		fmt.Printf("  Ports:       %s\n", strings.Join(container.Ports, ", "))
	}
	fmt.Printf("  Replica:     %d\n", container.ReplicaIndex)
	fmt.Printf("  Created:     %s ago\n", humanDuration(time.Since(container.CreatedAt)))
	fmt.Printf("  Updated:     %s ago\n", humanDuration(time.Since(container.UpdatedAt)))

	return nil
}
