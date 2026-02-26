package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// renderDeploymentList renders the deployment list view with cursor selection.
func renderDeploymentList(data *DashboardData, width, cursor int) string {
	if data == nil {
		return styleDim.Render("  Loading...")
	}
	if len(data.Deployments) == 0 {
		return renderBox("Deployments", styleDim.Render("No deployments"), width)
	}

	groups := groupDeployments(data.Deployments)

	const (
		colName    = 18
		colStatus  = 12
		colHealthy = 10
		colSvcs    = 10
		colTags    = 12
	)

	header := "    " +
		padRight(styleBold.Render("Name"), colName) +
		padRight(styleBold.Render("Status"), colStatus) +
		padRight(styleBold.Render("Healthy"), colHealthy) +
		padRight(styleBold.Render("Services"), colSvcs) +
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

		prefix := "  "
		nameStyle := lipgloss.NewStyle()
		if i == cursor {
			prefix = styleSelected.Render("> ")
			nameStyle = styleSelected
		}

		row := prefix +
			padRight(nameStyle.Render(truncate(d.Name, colName-2)), colName) +
			padRight(statusDot(d.Status)+" "+truncate(d.Status, 8), colStatus) +
			padRight(healthStr, colHealthy) +
			padRight(fmt.Sprintf("%d", d.Services), colSvcs) +
			padRight(tags, colTags) +
			age

		rows = append(rows, row)

		// Show summary of older deployments
		if len(g.Older) > 0 {
			stopped, running := 0, 0
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
			summary := "      " + styleDim.Render("└ ") + strings.Join(parts, ", ") + styleDim.Render(" previous")
			rows = append(rows, summary)
		}
	}

	activeCount := 0
	for i := range groups {
		if isActiveStatus(groups[i].Latest.Status) {
			activeCount++
		}
	}

	title := fmt.Sprintf("Deployments (%d active, %d total)", activeCount, len(data.Deployments))
	return renderBox(title, strings.Join(rows, "\n"), width)
}

// renderDeploymentDetail renders detailed info for a single deployment.
func renderDeploymentDetail(data *DashboardData, deployID string, width int) string {
	if data == nil {
		return styleDim.Render("  Loading...")
	}

	// Find deployment
	var deploy *DeploymentData
	for i := range data.Deployments {
		if data.Deployments[i].ID == deployID {
			deploy = &data.Deployments[i]
			break
		}
	}
	if deploy == nil {
		return renderBox("Deployment", styleDim.Render("Deployment not found"), width)
	}

	// Info card
	status := statusDot(deploy.Status) + " " + styleBold.Render(capitalize(deploy.Status))
	age := formatDuration(time.Since(deploy.CreatedAt))
	updated := formatDuration(time.Since(deploy.UpdatedAt))
	tags := strings.Join(deploy.Tags, ", ")
	if tags == "" {
		tags = "-"
	}

	healthRatio := float64(0)
	if deploy.Total > 0 {
		healthRatio = float64(deploy.Healthy) / float64(deploy.Total)
	}
	healthBar := healthBar(healthRatio, 20)
	healthPct := "0%"
	if deploy.Total > 0 {
		healthPct = fmt.Sprintf("%d%%", int(healthRatio*100))
	}

	info := fmt.Sprintf("%s  %s\n", status, styleDim.Render("ID: "+truncate(deploy.ID, 12))) +
		fmt.Sprintf("%-14s %s    %-14s %s\n", styleLabel.Render("Created"), age+" ago", styleLabel.Render("Updated"), updated+" ago") +
		fmt.Sprintf("%-14s %s\n", styleLabel.Render("Tags"), tags) +
		fmt.Sprintf("%-14s %d/%d %s %s", styleLabel.Render("Health"), deploy.Healthy, deploy.Total, healthBar, healthPct)

	if deploy.Error != "" {
		info += "\n" + styleError.Render("Error: "+deploy.Error)
	}

	infoBox := renderBox("Deployment: "+deploy.Name, info, width)

	// Services card
	var svcRows []string
	if len(deploy.ServiceDetails) == 0 {
		svcRows = append(svcRows, styleDim.Render("  No service details available"))
	} else {
		svcHeader := "  " +
			padRight(styleBold.Render("Service"), 16) +
			padRight(styleBold.Render("Image"), 28) +
			padRight(styleBold.Render("Replicas"), 10) +
			padRight(styleBold.Render("Ports"), 16) +
			styleBold.Render("Depends On")
		svcRows = append(svcRows, svcHeader)

		for i := range deploy.ServiceDetails {
			s := &deploy.ServiceDetails[i]
			ports := strings.Join(s.Ports, ", ")
			if ports == "" {
				ports = "-"
			}
			deps := strings.Join(s.DependsOn, ", ")
			if deps == "" {
				deps = "-"
			}
			row := "  " +
				padRight(truncate(s.Name, 14), 16) +
				padRight(truncate(s.Image, 26), 28) +
				padRight(fmt.Sprintf("%d", s.Replicas), 10) +
				padRight(truncate(ports, 14), 16) +
				truncate(deps, 20)
			svcRows = append(svcRows, row)
		}
	}
	svcBox := renderBox(fmt.Sprintf("Services (%d)", len(deploy.ServiceDetails)), strings.Join(svcRows, "\n"), width)

	// Containers card
	containerBox := renderContainerTable(deploy.Containers, width, fmt.Sprintf("Containers (%d)", len(deploy.Containers)), true)

	return lipgloss.JoinVertical(lipgloss.Left, infoBox, "", svcBox, "", containerBox)
}
