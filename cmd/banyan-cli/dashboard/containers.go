package dashboard

import (
	"fmt"
	"strings"
)

// renderContainerList renders the flat container list view with cursor.
func renderContainerList(data *DashboardData, width, cursor int) string {
	if data == nil {
		return styleDim.Render("  Loading...")
	}
	if len(data.Containers) == 0 {
		return renderBox("Containers", styleDim.Render("No containers"), width)
	}

	const (
		colName    = 28
		colService = 14
		colAgent   = 16
		colDeploy  = 16
		colStatus  = 12
		colPorts   = 12
	)

	header := "    " +
		padRight(styleBold.Render("Container"), colName) +
		padRight(styleBold.Render("Service"), colService) +
		padRight(styleBold.Render("Agent"), colAgent) +
		padRight(styleBold.Render("Deployment"), colDeploy) +
		padRight(styleBold.Render("Status"), colStatus) +
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
			truncate(ports, colPorts)

		rows = append(rows, row)
	}

	title := fmt.Sprintf("All Containers (%d)", len(data.Containers))
	return renderBox(title, strings.Join(rows, "\n"), width)
}
