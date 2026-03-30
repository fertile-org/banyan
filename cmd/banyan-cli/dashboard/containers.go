package dashboard

import (
	"fmt"
	"strings"
)

// renderContainerList renders the flat container list view with cursor.
// cpuHistory provides per-container CPU history for sparkline rendering.
func renderContainerList(data *DashboardData, width, cursor int, cpuHistory map[string][]float64) string {
	if data == nil {
		return styleDim.Render("  Loading...")
	}
	if len(data.Containers) == 0 {
		return renderBox("Containers", styleDim.Render("No containers"), width)
	}

	const (
		colName    = 26
		colService = 12
		colAgent   = 14
		colDeploy  = 14
		colStatus  = 12
		colCPU     = 10 // sparkline column
		colPorts   = 12
	)

	header := "    " +
		padRight(styleBold.Render("Container"), colName) +
		padRight(styleBold.Render("Service"), colService) +
		padRight(styleBold.Render("Agent"), colAgent) +
		padRight(styleBold.Render("Deploy"), colDeploy) +
		padRight(styleBold.Render("Status"), colStatus) +
		padRight(styleBold.Render("CPU"), colCPU) +
		styleBold.Render("Ports")

	var rows []string
	rows = append(rows, header)

	for i := range data.Containers {
		c := &data.Containers[i]
		displayStatus := c.ContainerStatus
		if displayStatus == "" {
			displayStatus = c.Status
		}

		ports := strings.Join(c.Ports, ", ")
		if ports == "" {
			ports = "-"
		}

		// Build sparkline from CPU history
		cpuSpark := styleDim.Render("-")
		if cpuHistory != nil {
			if history := cpuHistory[c.Name]; len(history) > 0 {
				cpuSpark = sparkline(history, 8)
			}
		}

		prefix := "  "
		nameStyle := styleNone
		if i == cursor {
			prefix = styleSelected.Render("> ")
			nameStyle = styleSelected
		}

		row := prefix +
			padRight(nameStyle.Render(truncate(c.Name, colName-2)), colName) +
			padRight(truncate(c.ServiceName, colService-2), colService) +
			padRight(truncate(c.AgentName, colAgent-2), colAgent) +
			padRight(truncate(c.DeploymentName, colDeploy-2), colDeploy) +
			padRight(statusDot(displayStatus)+" "+truncate(displayStatus, 8), colStatus) +
			padRight(cpuSpark, colCPU) +
			truncate(ports, colPorts)

		rows = append(rows, row)
	}

	title := fmt.Sprintf("All Containers (%d)", len(data.Containers))
	return renderBox(title, strings.Join(rows, "\n"), width)
}
