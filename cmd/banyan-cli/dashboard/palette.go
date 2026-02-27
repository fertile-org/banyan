package dashboard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// paletteAction represents a navigable action in the command palette.
type paletteAction struct {
	name string
	desc string
	view View // target view, or viewActionRefresh/viewActionQuit for special actions
}

// Special action sentinel values (negative to avoid collision with View enum).
const (
	viewActionRefresh View = -1
	viewActionQuit    View = -2
)

var allPaletteActions = []paletteAction{
	{name: "Overview", desc: "Cluster overview", view: ViewOverview},
	{name: "Agents", desc: "List all agents", view: ViewAgents},
	{name: "Deployments", desc: "List all deployments", view: ViewDeploys},
	{name: "Containers", desc: "List all containers", view: ViewContainers},
	{name: "Engine", desc: "Engine detail & metrics", view: ViewEngine},
	{name: "Events", desc: "Recent event log", view: ViewEvents},
	{name: "Refresh", desc: "Refresh dashboard data", view: viewActionRefresh},
	{name: "Quit", desc: "Exit dashboard", view: viewActionQuit},
}

// filterPaletteActions returns actions matching the filter (case-insensitive substring on name+desc).
func filterPaletteActions(filter string) []paletteAction {
	if filter == "" {
		return allPaletteActions
	}
	lower := strings.ToLower(filter)
	var result []paletteAction
	for _, a := range allPaletteActions {
		if strings.Contains(strings.ToLower(a.name+a.desc), lower) {
			result = append(result, a)
		}
	}
	return result
}

// renderPaletteBox renders the palette content inside a bordered box.
func renderPaletteBox(filter string, cursor, width int) string {
	stTitle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	stBorder := lipgloss.NewStyle().Foreground(colorBorder)
	stCursor := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	stActionName := lipgloss.NewStyle().Bold(true)
	stActionDesc := lipgloss.NewStyle().Foreground(colorDim)
	stHint := lipgloss.NewStyle().Foreground(colorDim)

	innerWidth := width - 4 // border + padding

	// Top border
	title := stTitle.Render("Command Palette")
	titleWidth := lipgloss.Width(title)
	topFill := max(width-5-titleWidth, 1)
	top := stBorder.Render("╭─ ") + title + stBorder.Render(" "+strings.Repeat("─", topFill)+"╮")

	// Helper to make a content line
	line := func(content string) string {
		lineWidth := lipgloss.Width(content)
		pad := max(innerWidth-lineWidth, 0)
		return stBorder.Render("│ ") + content + strings.Repeat(" ", pad) + stBorder.Render(" │")
	}

	// Search input
	prompt := stCursor.Render("> ") + filter + styleBold.Render("_")
	rows := []string{top, line(prompt), line("")}

	// Filtered actions
	actions := filterPaletteActions(filter)
	if len(actions) == 0 {
		rows = append(rows, line(stActionDesc.Render("No matching actions")))
	} else {
		for i, a := range actions {
			marker := "  "
			nameStyle := stActionName
			if i == cursor {
				marker = stCursor.Render("> ")
				nameStyle = lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
			}

			nameStr := nameStyle.Render(padRight(a.name, 16))
			descStr := stActionDesc.Render(a.desc)
			entry := marker + nameStr + descStr
			rows = append(rows, line(entry))
		}
	}

	// Hint + bottom border
	rows = append(rows,
		line(""),
		line(stHint.Render("↑↓ Navigate  ↵ Select  Esc Close")),
		stBorder.Render("╰"+strings.Repeat("─", width-2)+"╯"),
	)

	return strings.Join(rows, "\n")
}
