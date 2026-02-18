package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/fertile-org/banyan/pkg/types"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show cluster status",
	Long: `Show the status of the Banyan cluster including agents, deployments, and container health.

Examples:
  banyan-cli status`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan Cluster - Status")
	fmt.Println("========================================")

	engineURL := types.GetCLIEngineEndpoint(configPath)
	if engineURL == "" {
		return fmt.Errorf("engine endpoint not configured. Run 'banyan-cli init' to configure")
	}

	password := types.GetConfigPassword(configPath)
	client := NewEngineClient(engineURL, password)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	status, err := client.Status(ctx)
	if err != nil {
		fmt.Printf("Engine: UNREACHABLE (%v)\n", err)
		fmt.Println("========================================")
		return nil
	}

	fmt.Println("Engine: RUNNING")
	fmt.Printf("Connection: %s\n", engineURL)

	// Print agents
	fmt.Printf("\nAgents: %d\n", len(status.Agents))
	for _, agent := range status.Agents {
		age := time.Since(agent.LastSeen).Truncate(time.Second)
		fmt.Printf("  - %s (status: %s, last seen: %s ago)\n", agent.Name, agent.Status, age)
	}

	// Print deployments
	fmt.Printf("\nDeployments: %d\n", len(status.Deployments))
	for _, d := range status.Deployments {
		fmt.Printf("  - %s (status: %s, containers: %d/%d healthy)\n",
			d.Name, d.Status, d.Healthy, d.Total)

		// Filter to create_and_start tasks
		var createTasks []types.TaskRecord
		for _, t := range d.Tasks {
			if t.Type == types.TaskTypeCreateAndStart {
				createTasks = append(createTasks, t)
			}
		}

		grouped := types.GroupTasksByService(createTasks)
		for _, svcName := range types.SortedServiceNames(grouped) {
			fmt.Printf("    %s:\n", svcName)
			for _, t := range grouped[svcName] {
				containerStatus := t.ContainerStatus
				if containerStatus == "" {
					containerStatus = "pending"
				}
				checkedInfo := ""
				if !t.ContainerCheckedAt.IsZero() {
					ago := time.Since(t.ContainerCheckedAt).Truncate(time.Second)
					checkedInfo = fmt.Sprintf(" (checked %s ago)", ago)
				}
				fmt.Printf("      %s on %s: %s%s\n", t.ContainerName, t.AgentID, containerStatus, checkedInfo)
			}
		}
	}

	fmt.Println("\n========================================")
	return nil
}

