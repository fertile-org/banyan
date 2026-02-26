package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// renderEngineDetail renders the expanded engine detail view.
func renderEngineDetail(data *DashboardData, width int) string {
	if data == nil {
		return styleDim.Render("  Loading...")
	}

	e := &data.Engine

	// Engine status card (expanded)
	barWidth := min(max(width-20, 10), 40)

	uptime := formatDuration(time.Since(e.StartedAt))
	status := statusDot(e.Status) + " " + styleBold.Render(capitalize(e.Status))

	cpuPct := fmt.Sprintf("%d%%", int(e.CPU*100))
	coresStr := ""
	if e.CPUCores > 0 {
		coresStr = fmt.Sprintf("  (%d cores)", e.CPUCores)
	}

	memRatio := float64(0)
	if e.MemTotal > 0 {
		memRatio = float64(e.MemUsed) / float64(e.MemTotal)
	}
	memPct := fmt.Sprintf("%d%%", int(memRatio*100))
	memDetail := fmt.Sprintf("%s / %s", formatBytes(e.MemUsed), formatBytes(e.MemTotal))

	diskRatio := float64(0)
	if e.DiskTotal > 0 {
		diskRatio = float64(e.DiskUsed) / float64(e.DiskTotal)
	}
	diskPct := fmt.Sprintf("%d%%", int(diskRatio*100))
	diskDetail := fmt.Sprintf("%s / %s", formatBytes(e.DiskUsed), formatBytes(e.DiskTotal))

	info := fmt.Sprintf("%s  %s\n", status, styleDim.Render("uptime "+uptime)) +
		"\n" +
		fmt.Sprintf("%s  %s %s%s\n", styleLabel.Render("CPU "), progressBar(e.CPU, barWidth), cpuPct, coresStr) +
		fmt.Sprintf("%s  %s %s  %s\n", styleLabel.Render("Mem "), progressBar(memRatio, barWidth), memPct, styleDim.Render(memDetail)) +
		fmt.Sprintf("%s  %s %s  %s", styleLabel.Render("Disk"), progressBar(diskRatio, barWidth), diskPct, styleDim.Render(diskDetail))

	engineBox := renderBox("Engine", info, width)

	// Cluster summary card (expanded)
	s := data.Summary
	agentDots := ""
	for range s.ConnectedAgents {
		agentDots += lipgloss.NewStyle().Foreground(colorGreen).Render("●")
	}
	for range s.TotalAgents - s.ConnectedAgents {
		agentDots += lipgloss.NewStyle().Foreground(colorRed).Render("●")
	}

	containerHealth := fmt.Sprintf("%d/%d healthy", s.HealthyContainers, s.TotalContainers)
	if s.TotalContainers > 0 && s.HealthyContainers == s.TotalContainers {
		containerHealth = lipgloss.NewStyle().Foreground(colorGreen).Render(containerHealth)
	}

	summary := fmt.Sprintf("%-18s %d/%d connected %s\n", styleLabel.Render("Agents"), s.ConnectedAgents, s.TotalAgents, agentDots) +
		fmt.Sprintf("%-18s %d running / %d total\n", styleLabel.Render("Deployments"), s.RunningDeployments, s.TotalDeployments) +
		fmt.Sprintf("%-18s %s\n", styleLabel.Render("Containers"), containerHealth) +
		fmt.Sprintf("%-18s %d completed, %d failed", styleLabel.Render("Tasks"), s.CompletedTasks, s.FailedTasks)

	summaryBox := renderBox("Cluster Summary", summary, width)

	// Events (show more in detail view)
	eventsBox := renderEventsBoxExpanded(data.Events, width)

	return lipgloss.JoinVertical(lipgloss.Left, engineBox, "", summaryBox, "", eventsBox)
}

// renderEventsBoxExpanded renders recent events with more entries than the overview.
func renderEventsBoxExpanded(events []EventData, width int) string {
	maxEvents := 15
	if len(events) == 0 {
		return renderBox("Recent Events", styleDim.Render("No events yet"), width)
	}

	shown := events
	if len(shown) > maxEvents {
		shown = shown[:maxEvents]
	}

	var rows []string
	for _, e := range shown {
		ts := e.Timestamp.Format("15:04:05")
		icon := eventIcon(e.Type, e.Severity)
		msgWidth := max(width-16, 10)
		msg := truncate(e.Message, msgWidth)
		rows = append(rows, fmt.Sprintf("  %s  %s %s", styleDim.Render(ts), icon, msg))
	}

	return renderBox(fmt.Sprintf("Recent Events (%d)", len(events)), strings.Join(rows, "\n"), width)
}
