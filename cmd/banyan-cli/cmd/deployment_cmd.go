package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/cmd/banyan-cli/dashboard"
)

var deploymentCmd = &cobra.Command{
	Use:   "deployment [ID]",
	Short: "List deployments or show deployment detail",
	Long: `List all deployments in the cluster, or show detailed info for a specific deployment.

The argument matches against deployment name or ID.

Examples:
  banyan-cli deployment
  banyan-cli deployment my-app
  banyan-cli deployment dep-001
  banyan-cli deployment -o json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeployment,
}

func init() {
	rootCmd.AddCommand(deploymentCmd)
	deploymentCmd.Flags().StringVarP(&outputFormat, "output", "o", "", "Output format (json)")
}

func runDeployment(_ *cobra.Command, args []string) error {
	data, err := fetchClusterData()
	if err != nil {
		return err
	}

	if len(args) == 1 {
		return showDeploymentDetail(data.Deployments, args[0])
	}
	return listDeployments(data.Deployments)
}

func listDeployments(deployments []dashboard.DeploymentData) error {
	if outputFormat == "json" {
		return printJSON(deployments)
	}

	if len(deployments) == 0 {
		fmt.Println("No deployments found")
		return nil
	}

	fmt.Printf("%-20s %-12s %9s %10s %-15s %s\n",
		"NAME", "STATUS", "HEALTHY", "SERVICES", "TAGS", "AGE")
	fmt.Println(strings.Repeat("-", 80))

	for i := range deployments {
		d := &deployments[i]
		tags := ""
		if len(d.Tags) > 0 {
			tags = strings.Join(d.Tags, ",")
		}
		healthy := fmt.Sprintf("%d/%d", d.Healthy, d.Total)
		age := humanDuration(time.Since(d.CreatedAt))
		fmt.Printf("%-20s %-12s %9s %10d %-15s %s\n",
			d.Name, d.Status, healthy, d.Services, tags, age)
	}

	return nil
}

func showDeploymentDetail(deployments []dashboard.DeploymentData, query string) error {
	var dep *dashboard.DeploymentData
	for i := range deployments {
		if deployments[i].Name == query || deployments[i].ID == query {
			dep = &deployments[i]
			break
		}
	}
	if dep == nil {
		return fmt.Errorf("deployment %q not found", query)
	}

	if outputFormat == "json" {
		return printJSON(dep)
	}

	fmt.Printf("Deployment: %s\n", dep.Name)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("  ID:       %s\n", dep.ID)
	fmt.Printf("  Status:   %s\n", dep.Status)
	fmt.Printf("  Healthy:  %d/%d\n", dep.Healthy, dep.Total)
	fmt.Printf("  Tags:     %s\n", strings.Join(dep.Tags, ", "))
	fmt.Printf("  Created:  %s ago\n", humanDuration(time.Since(dep.CreatedAt)))
	fmt.Printf("  Updated:  %s ago\n", humanDuration(time.Since(dep.UpdatedAt)))
	if dep.Error != "" {
		fmt.Printf("  Error:    %s\n", dep.Error)
	}

	if len(dep.ServiceDetails) > 0 {
		fmt.Println()
		fmt.Println("Services")
		fmt.Println(strings.Repeat("-", 60))
		for _, svc := range dep.ServiceDetails {
			fmt.Printf("  %s\n", svc.Name)
			fmt.Printf("    Image:     %s\n", svc.Image)
			fmt.Printf("    Replicas:  %d\n", svc.Replicas)
			if len(svc.Ports) > 0 {
				fmt.Printf("    Ports:     %s\n", strings.Join(svc.Ports, ", "))
			}
			if len(svc.DependsOn) > 0 {
				fmt.Printf("    Depends:   %s\n", strings.Join(svc.DependsOn, ", "))
			}
		}
	}

	if len(dep.Containers) > 0 {
		fmt.Println()
		fmt.Println("Containers")
		fmt.Println(strings.Repeat("-", 60))
		fmt.Printf("  %-25s %-12s %-15s %s\n", "NAME", "STATUS", "AGENT", "IMAGE")
		for j := range dep.Containers {
			c := &dep.Containers[j]
			status := c.ContainerStatus
			if status == "" {
				status = c.Status
			}
			fmt.Printf("  %-25s %-12s %-15s %s\n",
				c.Name, status, c.AgentName, c.Image)
		}
	}

	return nil
}
