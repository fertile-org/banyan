package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// renderAgentList renders the agent list view with cursor selection.
func renderAgentList(data *DashboardData, width, cursor int) string {
	if data == nil {
		return styleDim.Render("  Loading...")
	}
	if len(data.Agents) == 0 {
		return renderBox("Agents", styleDim.Render("No agents connected"), width)
	}

	const (
		colName       = 18
		colStatus     = 10
		colTags       = 14
		colSubnet     = 16
		colCPU        = 18
		colMem        = 18
		colContainers = 12
	)

	header := "    " +
		padRight(styleBold.Render("Name"), colName) +
		padRight(styleBold.Render("Status"), colStatus) +
		padRight(styleBold.Render("Tags"), colTags) +
		padRight(styleBold.Render("VPC Subnet"), colSubnet) +
		padRight(styleBold.Render("CPU"), colCPU) +
		padRight(styleBold.Render("Mem"), colMem) +
		styleBold.Render("Containers")

	var rows []string
	rows = append(rows, header)

	for i := range data.Agents {
		a := &data.Agents[i]
		tags := truncate(strings.Join(a.Tags, ", "), colTags-2)
		subnet := truncate(a.VPCSubnet, colSubnet-2)
		if subnet == "" {
			subnet = styleDim.Render("-")
		}

		memRatio := float64(0)
		if a.MemTotal > 0 {
			memRatio = float64(a.MemUsed) / float64(a.MemTotal)
		}

		cpuStr := progressBar(a.CPU, 10) + fmt.Sprintf(" %2d%%", int(a.CPU*100))
		memStr := progressBar(memRatio, 10) + fmt.Sprintf(" %2d%%", int(memRatio*100))

		prefix := "  "
		nameStyle := lipgloss.NewStyle()
		if i == cursor {
			prefix = styleSelected.Render("> ")
			nameStyle = styleSelected
		}

		row := prefix +
			padRight(nameStyle.Render(truncate(a.Name, colName-2)), colName) +
			padRight(statusDot(a.Status)+" "+truncate(a.Status, 6), colStatus) +
			padRight(tags, colTags) +
			padRight(subnet, colSubnet) +
			padRight(cpuStr, colCPU) +
			padRight(memStr, colMem) +
			fmt.Sprintf("%4d", a.ContainerCount)

		rows = append(rows, row)
	}

	title := fmt.Sprintf("Agents (%d)", len(data.Agents))
	return renderBox(title, strings.Join(rows, "\n"), width)
}

// renderAgentDetail renders detailed info for a single agent.
func renderAgentDetail(data *DashboardData, agentName string, width int) string {
	if data == nil {
		return styleDim.Render("  Loading...")
	}

	// Find agent
	var agent *AgentData
	for i := range data.Agents {
		if data.Agents[i].Name == agentName {
			agent = &data.Agents[i]
			break
		}
	}
	if agent == nil {
		return renderBox("Agent", styleDim.Render("Agent not found: "+agentName), width)
	}

	// Info card
	barWidth := min(max(width-20, 10), 30)

	status := statusDot(agent.Status) + " " + styleBold.Render(capitalize(agent.Status))
	uptime := formatDuration(time.Since(agent.CreatedAt))
	tags := strings.Join(agent.Tags, ", ")
	if tags == "" {
		tags = "-"
	}
	subnet := agent.VPCSubnet
	if subnet == "" {
		subnet = "-"
	}

	cpuPct := fmt.Sprintf("%d%%", int(agent.CPU*100))
	memRatio := float64(0)
	if agent.MemTotal > 0 {
		memRatio = float64(agent.MemUsed) / float64(agent.MemTotal)
	}
	memPct := fmt.Sprintf("%d%%", int(memRatio*100))
	memDetail := fmt.Sprintf("%s / %s", formatBytes(agent.MemUsed), formatBytes(agent.MemTotal))

	diskRatio := float64(0)
	if agent.DiskTotal > 0 {
		diskRatio = float64(agent.DiskUsed) / float64(agent.DiskTotal)
	}
	diskPct := fmt.Sprintf("%d%%", int(diskRatio*100))
	diskDetail := fmt.Sprintf("%s / %s", formatBytes(agent.DiskUsed), formatBytes(agent.DiskTotal))

	coresStr := ""
	if agent.CPUCores > 0 {
		coresStr = fmt.Sprintf("  (%d cores)", agent.CPUCores)
	}

	info := fmt.Sprintf("%s  %s\n", status, styleDim.Render(uptime)) +
		fmt.Sprintf("%-14s %s\n", styleLabel.Render("Address"), agent.APIAddress) +
		fmt.Sprintf("%-14s %s\n", styleLabel.Render("Tags"), tags) +
		fmt.Sprintf("%-14s %s\n", styleLabel.Render("VPC Subnet"), subnet) +
		"\n" +
		fmt.Sprintf("%s  %s %s%s\n", styleLabel.Render("CPU "), progressBar(agent.CPU, barWidth), cpuPct, coresStr) +
		fmt.Sprintf("%s  %s %s  %s\n", styleLabel.Render("Mem "), progressBar(memRatio, barWidth), memPct, styleDim.Render(memDetail)) +
		fmt.Sprintf("%s  %s %s  %s", styleLabel.Render("Disk"), progressBar(diskRatio, barWidth), diskPct, styleDim.Render(diskDetail))

	infoBox := renderBox("Agent: "+agentName, info, width)

	// Containers on this agent
	var agentContainers []ContainerData
	for i := range data.Containers {
		if data.Containers[i].AgentName == agentName {
			agentContainers = append(agentContainers, data.Containers[i])
		}
	}

	containerBox := renderContainerTable(agentContainers, width, fmt.Sprintf("Containers (%d)", len(agentContainers)), false)

	return lipgloss.JoinVertical(lipgloss.Left, infoBox, "", containerBox)
}

// renderContainerTable renders a table of containers inside a box.
// showAgent controls whether the Agent column is displayed.
func renderContainerTable(containers []ContainerData, width int, title string, showAgent bool) string {
	if len(containers) == 0 {
		return renderBox(title, styleDim.Render("No containers"), width)
	}

	const (
		colName    = 28
		colService = 14
		colAgent   = 16
		colStatus  = 12
		colImage   = 20
		colPorts   = 12
	)

	header := "  "
	header += padRight(styleBold.Render("Container"), colName)
	header += padRight(styleBold.Render("Service"), colService)
	if showAgent {
		header += padRight(styleBold.Render("Agent"), colAgent)
	}
	header += padRight(styleBold.Render("Status"), colStatus)
	header += padRight(styleBold.Render("Image"), colImage)
	header += styleBold.Render("Ports")

	var rows []string
	rows = append(rows, header)

	for i := range containers {
		c := &containers[i]
		displayStatus := c.ContainerStatus
		if displayStatus == "" {
			displayStatus = c.Status
		}

		ports := strings.Join(c.Ports, ", ")
		if ports == "" {
			ports = "-"
		}

		row := "  "
		row += padRight(truncate(c.Name, colName-2), colName)
		row += padRight(truncate(c.ServiceName, colService-2), colService)
		if showAgent {
			row += padRight(truncate(c.AgentName, colAgent-2), colAgent)
		}
		row += padRight(statusDot(displayStatus)+" "+truncate(displayStatus, 8), colStatus)
		row += padRight(truncate(c.Image, colImage-2), colImage)
		row += truncate(ports, colPorts)

		rows = append(rows, row)
	}

	return renderBox(title, strings.Join(rows, "\n"), width)
}
