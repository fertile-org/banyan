package dashboard

import (
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// paletteAction represents a navigable action in the command palette.
type paletteAction struct {
	name        string
	desc        string
	section     string // section label for grouping (e.g. "Actions", "Navigate")
	availableIn []View // if non-nil, only show when activeView is in this list
	view        View   // target view, or sentinel for special actions
}

// Special action sentinel values (negative to avoid collision with View enum).
const (
	viewActionRefresh View = -1
	viewActionQuit    View = -2
	viewActionFilter  View = -3
	viewActionExport  View = -4
)

// listViews are views that support filtering and CSV export.
var listViews = []View{ViewAgents, ViewDeploys, ViewContainers, ViewEvents}

var allPaletteActions = []paletteAction{
	// Page actions (only on list views)
	{name: "Filter", desc: "Filter current list", section: "Actions", view: viewActionFilter, availableIn: listViews},
	{name: "Export CSV", desc: "Export current list to CSV", section: "Actions", view: viewActionExport, availableIn: listViews},
	// Navigation
	{name: "Overview", desc: "Cluster overview", section: "Navigate", view: ViewOverview},
	{name: "Agents", desc: "List all agents", section: "Navigate", view: ViewAgents},
	{name: "Deployments", desc: "List all deployments", section: "Navigate", view: ViewDeploys},
	{name: "Containers", desc: "List all containers", section: "Navigate", view: ViewContainers},
	{name: "Engine", desc: "Engine detail & metrics", section: "Navigate", view: ViewEngine},
	{name: "Events", desc: "Recent event log", section: "Navigate", view: ViewEvents},
	// Commands
	{name: "Refresh", desc: "Refresh dashboard data", section: "Commands", view: viewActionRefresh},
	{name: "Quit", desc: "Exit dashboard", section: "Commands", view: viewActionQuit},
}

// filterPaletteActions returns actions matching the filter (case-insensitive substring on name+desc),
// filtered by availability for the given active view.
func filterPaletteActions(filter string, activeView View) []paletteAction {
	lower := strings.ToLower(filter)
	var result []paletteAction
	for i := range allPaletteActions {
		if !isActionAvailable(&allPaletteActions[i], activeView) {
			continue
		}
		a := allPaletteActions[i]
		if filter == "" || strings.Contains(strings.ToLower(a.name+a.desc), lower) {
			result = append(result, a)
		}
	}
	return result
}

// isActionAvailable returns true if the action should be shown for the given view.
func isActionAvailable(a *paletteAction, activeView View) bool {
	if a.availableIn == nil {
		return true
	}
	return slices.Contains(a.availableIn, activeView)
}

// renderPaletteBox renders the palette content inside a bordered box.
func renderPaletteBox(filter string, cursor, width int, activeView View) string {
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

	// Filtered actions, grouped by section with dividers
	actions := filterPaletteActions(filter, activeView)
	if len(actions) == 0 {
		rows = append(rows, line(stActionDesc.Render("No matching actions")))
	} else {
		stSection := lipgloss.NewStyle().Foreground(colorDim).Bold(true)
		lastSection := ""
		for i, a := range actions {
			// Section divider when section changes
			if a.section != lastSection {
				if lastSection != "" {
					rows = append(rows, line(""))
				}
				rows = append(rows, line(stSection.Render(a.section)))
				lastSection = a.section
			}

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
