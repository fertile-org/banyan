package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fertile-org/banyan/cmd/banyan-cli/dashboard"
	"github.com/fertile-org/banyan/pkg/types"
)

// outputFormat holds the --output flag value for resource commands.
var outputFormat string

// fetchClusterData connects to the engine and returns the full cluster snapshot.
func fetchClusterData() (*dashboard.DashboardData, error) {
	engineAddr := types.GetCLIEngineEndpoint(configPath)
	if engineAddr == "" {
		return nil, fmt.Errorf("engine endpoint not configured. Run 'banyan-cli init' to configure")
	}

	client, err := NewAutoEngineClient(engineAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to engine: %w", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.DashboardData(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cluster data: %w", err)
	}

	data := dashboard.ConvertFromProto(resp)
	return &data, nil
}

// printJSON marshals v to indented JSON and prints it.
func printJSON(v any) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

// humanDuration formats a duration into a human-readable age string.
func humanDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	}
	days := int(d.Hours()) / 24
	return fmt.Sprintf("%dd", days)
}

// formatPercent formats a 0.0–1.0 ratio as a percentage string.
func formatPercent(ratio float64) string {
	return fmt.Sprintf("%.1f%%", ratio*100)
}

// formatBytes formats bytes into a human-readable string.
func formatBytes(bytes uint64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// padRight pads s with spaces to the given width.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
