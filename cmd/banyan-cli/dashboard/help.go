package dashboard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderHelpBox renders the help content inside a bordered box.
func renderHelpBox(width int) string {
	stTitle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	stBorder := lipgloss.NewStyle().Foreground(colorBorder)
	stKey := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	stSection := lipgloss.NewStyle().Bold(true)
	stHint := lipgloss.NewStyle().Foreground(colorDim)

	innerWidth := width - 4

	line := func(content string) string {
		lineWidth := lipgloss.Width(content)
		pad := max(innerWidth-lineWidth, 0)
		return stBorder.Render("│ ") + content + strings.Repeat(" ", pad) + stBorder.Render(" │")
	}

	// Top border
	title := stTitle.Render("Keyboard Shortcuts")
	titleWidth := lipgloss.Width(title)
	topFill := max(width-5-titleWidth, 1)
	top := stBorder.Render("╭─ ") + title + stBorder.Render(" "+strings.Repeat("─", topFill)+"╮")

	rows := []string{
		top,
		line(""),
		// Navigation section
		line(stSection.Render("Navigation")),
		line(stKey.Render(padRight("1-6", 12)) + "Switch views"),
		line(stKey.Render(padRight("p", 12)) + "Command palette"),
		line(stKey.Render(padRight("Esc", 12)) + "Back / Close overlay"),
		line(""),
		// List views section
		line(stSection.Render("List Views")),
		line(stKey.Render(padRight("↑ / k", 12)) + "Move up"),
		line(stKey.Render(padRight("↓ / j", 12)) + "Move down"),
		line(stKey.Render(padRight("Enter", 12)) + "Drill into detail"),
		line(stKey.Render(padRight("/", 12)) + "Filter list"),
		line(stKey.Render(padRight("e", 12)) + "Export to CSV"),
		line(""),
		// General section
		line(stSection.Render("General")),
		line(stKey.Render(padRight("r", 12)) + "Refresh data"),
		line(stKey.Render(padRight("?", 12)) + "Toggle this help"),
		line(stKey.Render(padRight("q", 12)) + "Quit dashboard"),
		line(""),
		// Hint + bottom border
		line(stHint.Render("Press ? or Esc to close")),
		stBorder.Render("╰" + strings.Repeat("─", width-2) + "╯"),
	}

	return strings.Join(rows, "\n")
}
