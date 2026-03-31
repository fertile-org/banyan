package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/cmd/banyan-cli/dashboard"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
)

var (
	dashboardRefreshInterval time.Duration
	dashboardSnapshot        bool
	dashboardSnapshotOutput  string
)

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

Snapshot mode:
  --snapshot    Fetch data once and export a self-contained HTML file

Examples:
  banyan-cli dashboard
  banyan-cli dashboard --refresh 10s
  banyan-cli dashboard --snapshot
  banyan-cli dashboard --snapshot -o cluster.html`,
	RunE: runDashboard,
}

func init() {
	dashboardCmd.Flags().DurationVar(&dashboardRefreshInterval, "refresh", 5*time.Second, "Auto-refresh interval")
	dashboardCmd.Flags().BoolVar(&dashboardSnapshot, "snapshot", false, "Export a self-contained HTML snapshot instead of opening the live dashboard")
	dashboardCmd.Flags().StringVarP(&dashboardSnapshotOutput, "output", "o", "", "Output file for snapshot (default: banyan-snapshot-<timestamp>.html)")
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

	if dashboardSnapshot {
		return runSnapshot(client.GRPCClient())
	}

	model := dashboard.New(client.GRPCClient(), dashboardRefreshInterval)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("dashboard error: %w", err)
	}

	return nil
}

func runSnapshot(grpcClient banyanpb.EngineServiceClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := grpcClient.GetDashboardData(ctx, &banyanpb.GetDashboardDataRequest{})
	if err != nil {
		return fmt.Errorf("failed to fetch dashboard data: %w", err)
	}

	data := dashboard.ConvertFromProto(resp)

	filename := dashboardSnapshotOutput
	if filename == "" {
		filename = fmt.Sprintf("banyan-snapshot-%s.html", time.Now().Format("20060102-150405"))
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("cannot create snapshot file: %w", err)
	}
	defer f.Close()

	if err := dashboard.RenderSnapshot(&data, f); err != nil {
		return fmt.Errorf("snapshot render failed: %w", err)
	}

	fmt.Printf("Snapshot saved to %s\n", filename)
	return nil
}
