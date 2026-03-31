package dashboard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// actionMenuItem represents an action in the context menu.
type actionMenuItem struct {
	name string
	desc string
	key  string // shortcut hint (e.g., "kill", "restart")
}

// containerActions returns actions available for a container.
func containerActions() []actionMenuItem {
	return []actionMenuItem{
		{name: "Kill", desc: "Stop and remove container", key: "kill"},
		{name: "Restart", desc: "Stop then recreate container", key: "restart"},
		{name: "Stream Logs", desc: "Stream container logs", key: "logs"},
		{name: "Cancel", desc: "Close this menu", key: "cancel"},
	}
}

// deploymentActions returns actions for a deployment.
func deploymentActions() []actionMenuItem {
	return []actionMenuItem{
		{name: "Teardown", desc: "Stop all containers", key: "teardown"},
		{name: "Scale", desc: "Adjust replica count", key: "scale"},
		{name: "Cancel", desc: "Close this menu", key: "cancel"},
	}
}

// renderActionMenu renders the action menu overlay box.
func renderActionMenu(target string, items []actionMenuItem, cursor, width int) string {
	stTitle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	stBorder := lipgloss.NewStyle().Foreground(colorBorder)
	stCursor := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	stActionName := lipgloss.NewStyle().Bold(true)
	stActionDesc := lipgloss.NewStyle().Foreground(colorDim)
	stHint := lipgloss.NewStyle().Foreground(colorDim)

	innerWidth := width - 4

	// Top border
	title := stTitle.Render("Actions: " + truncate(target, width-20))
	titleWidth := lipgloss.Width(title)
	topFill := max(width-5-titleWidth, 1)
	top := stBorder.Render("╭─ ") + title + stBorder.Render(" "+strings.Repeat("─", topFill)+"╮")

	line := func(content string) string {
		lineWidth := lipgloss.Width(content)
		pad := max(innerWidth-lineWidth, 0)
		return stBorder.Render("│ ") + content + strings.Repeat(" ", pad) + stBorder.Render(" │")
	}

	rows := []string{top, line("")}

	for i, item := range items {
		marker := "  "
		nameStyle := stActionName
		if i == cursor {
			marker = stCursor.Render("> ")
			nameStyle = lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
		}

		entry := marker + nameStyle.Render(padRight(item.name, 16)) + stActionDesc.Render(item.desc)
		rows = append(rows, line(entry))
	}

	rows = append(rows,
		line(""),
		line(stHint.Render("↑↓ Navigate  ↵ Select  Esc Close")),
		stBorder.Render("╰"+strings.Repeat("─", width-2)+"╯"),
	)

	return strings.Join(rows, "\n")
}
