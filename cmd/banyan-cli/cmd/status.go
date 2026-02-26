package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/types"
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

	engineAddr := types.GetCLIEngineEndpoint(configPath)
	if engineAddr == "" {
		return fmt.Errorf("engine endpoint not configured. Run 'banyan-cli init' to configure")
	}

	client, err := NewAutoEngineClient(engineAddr)
	if err != nil {
		fmt.Printf("Engine: UNREACHABLE (%v)\n", err)
		fmt.Println("========================================")
		return nil
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	status, err := client.Status(ctx)
	if err != nil {
		fmt.Printf("Engine: UNREACHABLE (%v)\n", err)
		fmt.Println("========================================")
		return nil
	}

	fmt.Println("Engine: RUNNING")
	fmt.Printf("Connection: %s\n", engineAddr)

	// Print agents
	fmt.Printf("\nAgents: %d\n", len(status.Agents))
	for _, agent := range status.Agents {
		lastSeen := time.Unix(agent.LastSeenUnix, 0)
		age := time.Since(lastSeen).Truncate(time.Second)
		fmt.Printf("  - %s (status: %s, last seen: %s ago)\n", agent.Name, agent.Status, age)
	}

	// Print deployments
	fmt.Printf("\nDeployments: %d\n", len(status.Deployments))
	for _, d := range status.Deployments {
		fmt.Printf("  - %s (status: %s, containers: %d/%d healthy)\n",
			d.Name, d.Status, d.Healthy, d.Total)

		// Filter to create_and_start tasks
		var createTasks []*taskInfoForDisplay
		for _, t := range d.Tasks {
			if t.Type == types.TaskTypeCreateAndStart {
				createTasks = append(createTasks, &taskInfoForDisplay{
					ServiceName:            t.ServiceName,
					ContainerName:          t.ContainerName,
					AgentID:                t.AgentId,
					ContainerStatus:        t.ContainerStatus,
					ContainerCheckedAtUnix: t.ContainerCheckedAtUnix,
				})
			}
		}

		grouped := groupTaskInfoByService(createTasks)
		for _, svcName := range sortedServiceNamesFromInfo(grouped) {
			fmt.Printf("    %s:\n", svcName)
			for _, t := range grouped[svcName] {
				containerStatus := t.ContainerStatus
				if containerStatus == "" {
					containerStatus = "pending"
				}
				checkedInfo := ""
				if t.ContainerCheckedAtUnix > 0 {
					checkedAt := time.Unix(t.ContainerCheckedAtUnix, 0)
					ago := time.Since(checkedAt).Truncate(time.Second)
					checkedInfo = fmt.Sprintf(" (checked %s ago)", ago)
				}
				fmt.Printf("      %s on %s: %s%s\n", t.ContainerName, t.AgentID, containerStatus, checkedInfo)
			}
		}
	}

	fmt.Println("\n========================================")
	return nil
}

// taskInfoForDisplay is a lightweight struct for status display.
type taskInfoForDisplay struct {
	ServiceName            string
	ContainerName          string
	AgentID                string
	ContainerStatus        string
	ContainerCheckedAtUnix int64
}

func groupTaskInfoByService(tasks []*taskInfoForDisplay) map[string][]*taskInfoForDisplay {
	grouped := make(map[string][]*taskInfoForDisplay)
	for _, t := range tasks {
		grouped[t.ServiceName] = append(grouped[t.ServiceName], t)
	}
	return grouped
}

func sortedServiceNamesFromInfo(grouped map[string][]*taskInfoForDisplay) []string {
	names := make([]string, 0, len(grouped))
	for name := range grouped {
		names = append(names, name)
	}
	// Simple sort
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[i] > names[j] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return names
}
