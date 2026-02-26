package cmd

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/cmd/banyan-cli/dashboard"
	"github.com/fertile-org/banyan/pkg/types"
)

var dashboardRefreshInterval time.Duration

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Open live cluster dashboard",
	Long: `Open a live terminal dashboard showing cluster status, agents, deployments, and events.

The dashboard auto-refreshes and displays:
  - Engine health (CPU, memory, disk)
  - Connected agents and their resource usage
  - Deployment status and container health
  - Recent cluster events

Navigation:
  1-4    Switch views (Overview, Agents, Deploys, Containers)
  r      Force refresh
  q      Quit

Examples:
  banyan-cli dashboard
  banyan-cli dashboard --refresh 10s`,
	RunE: runDashboard,
}

func init() {
	dashboardCmd.Flags().DurationVar(&dashboardRefreshInterval, "refresh", 5*time.Second, "Auto-refresh interval")
	rootCmd.AddCommand(dashboardCmd)
}

func runDashboard(cmd *cobra.Command, args []string) error {
	engineAddr := types.GetCLIEngineEndpoint(configPath)
	if engineAddr == "" {
		return fmt.Errorf("engine endpoint not configured. Run 'banyan-cli init' to configure")
	}

	client, err := NewAutoEngineClient(engineAddr)
	if err != nil {
		return fmt.Errorf("cannot connect to engine: %w", err)
	}
	defer client.Close()

	model := dashboard.New(client.GRPCClient(), dashboardRefreshInterval)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("dashboard error: %w", err)
	}

	return nil
}
