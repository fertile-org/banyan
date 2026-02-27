package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// filteredData returns a shallow copy of data with only items matching the filter for the given view.
// Returns the original data if filterText is empty.
func filteredData(data *DashboardData, view View, filterText string) *DashboardData {
	if filterText == "" || data == nil {
		return data
	}
	lower := strings.ToLower(filterText)
	filtered := *data

	switch view {
	case ViewAgents:
		var agents []AgentData
		for i := range data.Agents {
			a := &data.Agents[i]
			if matchesFilter(lower, a.Name, a.Status, strings.Join(a.Tags, " ")) {
				agents = append(agents, *a)
			}
		}
		filtered.Agents = agents
	case ViewDeploys:
		var deploys []DeploymentData
		for i := range data.Deployments {
			d := &data.Deployments[i]
			if matchesFilter(lower, d.Name, d.Status, strings.Join(d.Tags, " ")) {
				deploys = append(deploys, *d)
			}
		}
		filtered.Deployments = deploys
	case ViewContainers:
		var containers []ContainerData
		for i := range data.Containers {
			c := &data.Containers[i]
			status := c.ContainerStatus
			if status == "" {
				status = c.Status
			}
			if matchesFilter(lower, c.Name, c.ServiceName, c.AgentName, c.DeploymentName, status) {
				containers = append(containers, *c)
			}
		}
		filtered.Containers = containers
	case ViewEvents:
		var events []EventData
		for _, e := range data.Events {
			if matchesFilter(lower, e.Type, e.Message, e.Severity) {
				events = append(events, e)
			}
		}
		filtered.Events = events
	}

	return &filtered
}

// matchesFilter returns true if any field contains the filter substring (case-insensitive).
func matchesFilter(lower string, fields ...string) bool {
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), lower) {
			return true
		}
	}
	return false
}

// renderFilterBar renders the filter input bar shown above list content.
func renderFilterBar(filterText string, editing bool, width int) string {
	if editing {
		prompt := styleSelected.Render("/ ") + filterText + styleBold.Render("█")
		hints := styleDim.Render("  Enter apply  Esc cancel  ↑↓ navigate")
		gap := max(width-lipgloss.Width(prompt)-lipgloss.Width(hints)-2, 1)
		return " " + prompt + fmt.Sprintf("%*s", gap, "") + hints
	}

	label := styleDim.Render("Filter: ") + styleBold.Render(filterText)
	hints := styleDim.Render("  / edit  Esc clear  e export")
	gap := max(width-lipgloss.Width(label)-lipgloss.Width(hints)-2, 1)
	return " " + label + fmt.Sprintf("%*s", gap, "") + hints
}
