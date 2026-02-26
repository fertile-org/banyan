package dashboard

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// renderOverview renders the full overview screen.
func renderOverview(data *DashboardData, width int) string {
	if data == nil {
		return styleDim.Render("  Loading cluster data...")
	}

	// Top cards: Engine (left) + Cluster (right)
	cardWidth := (width - 1) / 2
	if cardWidth < 25 {
		cardWidth = width // stack vertically if too narrow
	}
	engineCard := renderEngineCard(&data.Engine, cardWidth)
	clusterCard := renderClusterCard(data.Summary, cardWidth)

	var topRow string
	if width >= 52 {
		topRow = lipgloss.JoinHorizontal(lipgloss.Top, engineCard, " ", clusterCard)
	} else {
		topRow = lipgloss.JoinVertical(lipgloss.Left, engineCard, clusterCard)
	}

	// Agents table
	agentsBox := renderAgentsTable(data.Agents, width)

	// Deployments table
	deploysBox := renderDeploymentsTable(data.Deployments, width)

	// Events
	eventsBox := renderEventsBox(data.Events, width)

	return lipgloss.JoinVertical(lipgloss.Left,
		topRow, "",
		agentsBox, "",
		deploysBox, "",
		eventsBox,
	)
}

// renderEngineCard renders the engine status card.
func renderEngineCard(e *EngineData, width int) string {
	uptime := formatDuration(time.Since(e.StartedAt))
	status := statusDot(e.Status) + " " + styleBold.Render(capitalize(e.Status))

	barWidth := min(max(width-15, 10), 30)
	cpuPct := fmt.Sprintf("%d%%", int(e.CPU*100))
	memPct := "0%"
	diskPct := "0%"
	if e.MemTotal > 0 {
		memPct = fmt.Sprintf("%d%%", int(float64(e.MemUsed)/float64(e.MemTotal)*100))
	}
	if e.DiskTotal > 0 {
		diskPct = fmt.Sprintf("%d%%", int(float64(e.DiskUsed)/float64(e.DiskTotal)*100))
	}

	memRatio := float64(0)
	if e.MemTotal > 0 {
		memRatio = float64(e.MemUsed) / float64(e.MemTotal)
	}
	diskRatio := float64(0)
	if e.DiskTotal > 0 {
		diskRatio = float64(e.DiskUsed) / float64(e.DiskTotal)
	}

	content := fmt.Sprintf("%s  %s\n", status, styleDim.Render(uptime)) +
		fmt.Sprintf("%s  %s %s\n", styleLabel.Render("CPU "), progressBar(e.CPU, barWidth), cpuPct) +
		fmt.Sprintf("%s  %s %s\n", styleLabel.Render("Mem "), progressBar(memRatio, barWidth), memPct) +
		fmt.Sprintf("%s  %s %s", styleLabel.Render("Disk"), progressBar(diskRatio, barWidth), diskPct)

	return renderBox("Engine", content, width)
}

// renderClusterCard renders the cluster summary card.
func renderClusterCard(s ClusterSummary, width int) string {
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

	content := fmt.Sprintf("%-13s %d/%d %s\n", styleLabel.Render("Agents"), s.ConnectedAgents, s.TotalAgents, agentDots) +
		fmt.Sprintf("%-13s %d running\n", styleLabel.Render("Deployments"), s.RunningDeployments) +
		fmt.Sprintf("%-13s %s", styleLabel.Render("Containers"), containerHealth)

	return renderBox("Cluster", content, width)
}

// renderAgentsTable renders the agents section.
func renderAgentsTable(agents []AgentData, width int) string {
	if len(agents) == 0 {
		return renderBox("Agents", styleDim.Render("No agents connected"), width)
	}

	// Column widths
	const (
		colName       = 18
		colStatus     = 10
		colTags       = 14
		colCPU        = 18
		colMem        = 18
		colDisk       = 18
		colContainers = 12
	)

	header := "  " +
		padRight(styleBold.Render("Name"), colName) +
		padRight(styleBold.Render("Status"), colStatus) +
		padRight(styleBold.Render("Tags"), colTags) +
		padRight(styleBold.Render("CPU"), colCPU) +
		padRight(styleBold.Render("Mem"), colMem) +
		padRight(styleBold.Render("Disk"), colDisk) +
		styleBold.Render("Containers")

	var rows []string
	rows = append(rows, header)

	for i := range agents {
		a := &agents[i]
		tags := truncate(strings.Join(a.Tags, ", "), colTags-2)

		memRatio := float64(0)
		if a.MemTotal > 0 {
			memRatio = float64(a.MemUsed) / float64(a.MemTotal)
		}
		diskRatio := float64(0)
		if a.DiskTotal > 0 {
			diskRatio = float64(a.DiskUsed) / float64(a.DiskTotal)
		}

		cpuStr := progressBar(a.CPU, 10) + fmt.Sprintf(" %2d%%", int(a.CPU*100))
		memStr := progressBar(memRatio, 10) + fmt.Sprintf(" %2d%%", int(memRatio*100))
		diskStr := progressBar(diskRatio, 10) + fmt.Sprintf(" %2d%%", int(diskRatio*100))

		row := "  " +
			padRight(truncate(a.Name, colName-2), colName) +
			padRight(statusDot(a.Status)+" "+truncate(a.Status, 6), colStatus) +
			padRight(tags, colTags) +
			padRight(cpuStr, colCPU) +
			padRight(memStr, colMem) +
			padRight(diskStr, colDisk) +
			fmt.Sprintf("%4d", a.ContainerCount)

		rows = append(rows, row)
	}

	return renderBox(fmt.Sprintf("Agents (%d)", len(agents)), strings.Join(rows, "\n"), width)
}

// deployGroup groups deployments by name, with the latest first.
type deployGroup struct {
	Name   string
	Older  []DeploymentData
	Latest DeploymentData
}

// groupDeployments groups deployments by name, sorting each group by CreatedAt descending.
// The most recently created deployment in each group becomes Latest.
func groupDeployments(deploys []DeploymentData) []deployGroup {
	byName := make(map[string][]DeploymentData)
	var nameOrder []string
	for i := range deploys {
		name := deploys[i].Name
		if _, exists := byName[name]; !exists {
			nameOrder = append(nameOrder, name)
		}
		byName[name] = append(byName[name], deploys[i])
	}

	var groups []deployGroup
	for _, name := range nameOrder {
		list := byName[name]
		// Sort by CreatedAt descending (newest first)
		slices.SortFunc(list, func(a, b DeploymentData) int {
			return cmp.Compare(b.CreatedAt.UnixNano(), a.CreatedAt.UnixNano())
		})
		groups = append(groups, deployGroup{
			Name:   name,
			Latest: list[0],
			Older:  list[1:],
		})
	}

	// Sort groups: active (running/pending) first, then by latest CreatedAt descending
	slices.SortFunc(groups, func(a, b deployGroup) int {
		aActive := isActiveStatus(a.Latest.Status)
		bActive := isActiveStatus(b.Latest.Status)
		if aActive != bActive {
			if aActive {
				return -1
			}
			return 1
		}
		return cmp.Compare(b.Latest.CreatedAt.UnixNano(), a.Latest.CreatedAt.UnixNano())
	})

	return groups
}

// isActiveStatus returns true if the deployment status is running or pending.
func isActiveStatus(status string) bool {
	return status == "running" || status == "pending" || status == "deploying"
}

// renderDeploymentsTable renders the deployments section grouped by name.
func renderDeploymentsTable(deploys []DeploymentData, width int) string {
	if len(deploys) == 0 {
		return renderBox("Deployments", styleDim.Render("No deployments"), width)
	}

	groups := groupDeployments(deploys)

	// Column widths
	const (
		colName     = 18
		colStatus   = 12
		colHealthy  = 10
		colServices = 10
		colTags     = 12
	)

	header := "  " +
		padRight(styleBold.Render("Name"), colName) +
		padRight(styleBold.Render("Status"), colStatus) +
		padRight(styleBold.Render("Healthy"), colHealthy) +
		padRight(styleBold.Render("Services"), colServices) +
		padRight(styleBold.Render("Tags"), colTags) +
		styleBold.Render("Age")

	var rows []string
	rows = append(rows, header)

	for i := range groups {
		g := &groups[i]
		d := &g.Latest
		tags := truncate(strings.Join(d.Tags, ", "), colTags-2)
		age := formatDuration(time.Since(d.CreatedAt))

		healthStr := fmt.Sprintf("%d/%d", d.Healthy, d.Total)
		if d.Total > 0 && d.Healthy == d.Total {
			healthStr = lipgloss.NewStyle().Foreground(colorGreen).Render(healthStr)
		} else if d.Total > 0 && d.Healthy < d.Total {
			healthStr = lipgloss.NewStyle().Foreground(colorYellow).Render(healthStr)
		}

		row := "  " +
			padRight(truncate(d.Name, colName-2), colName) +
			padRight(statusDot(d.Status)+" "+truncate(d.Status, 8), colStatus) +
			padRight(healthStr, colHealthy) +
			padRight(fmt.Sprintf("%d", d.Services), colServices) +
			padRight(tags, colTags) +
			age

		rows = append(rows, row)

		// Show summary of older deployments if any
		if len(g.Older) > 0 {
			stopped := 0
			running := 0
			for j := range g.Older {
				if isActiveStatus(g.Older[j].Status) {
					running++
				} else {
					stopped++
				}
			}
			var parts []string
			if running > 0 {
				parts = append(parts, lipgloss.NewStyle().Foreground(colorYellow).Render(fmt.Sprintf("%d running", running)))
			}
			if stopped > 0 {
				parts = append(parts, styleDim.Render(fmt.Sprintf("%d stopped", stopped)))
			}
			summary := "    " + styleDim.Render("└ ") + strings.Join(parts, ", ") + styleDim.Render(" previous")
			rows = append(rows, summary)
		}
	}

	activeCount := 0
	for i := range groups {
		if isActiveStatus(groups[i].Latest.Status) {
			activeCount++
		}
	}

	title := fmt.Sprintf("Deployments (%d active, %d total)", activeCount, len(deploys))
	return renderBox(title, strings.Join(rows, "\n"), width)
}

// renderEventsBox renders the recent events section.
func renderEventsBox(events []EventData, width int) string {
	maxEvents := 5
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
		msgWidth := max(width-16, 10) // account for timestamp + icon + padding
		msg := truncate(e.Message, msgWidth)
		rows = append(rows, fmt.Sprintf("  %s  %s %s", styleDim.Render(ts), icon, msg))
	}

	return renderBox("Recent Events", strings.Join(rows, "\n"), width)
}

// capitalize returns s with the first letter uppercased.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
