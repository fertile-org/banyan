package dashboard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// confirmState holds the state for a confirmation dialog.
type confirmState struct {
	message  string
	typeName string // if non-empty, type-to-confirm mode
	input    string // typed input for type-to-confirm
}

// renderConfirmDialog renders a confirmation dialog overlay.
func renderConfirmDialog(state confirmState, width int) string {
	stTitle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	stBorder := lipgloss.NewStyle().Foreground(colorBorder)
	stHint := lipgloss.NewStyle().Foreground(colorDim)
	stInput := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)

	innerWidth := width - 4

	title := stTitle.Render("Confirm")
	titleWidth := lipgloss.Width(title)
	topFill := max(width-5-titleWidth, 1)
	top := stBorder.Render("╭─ ") + title + stBorder.Render(" "+strings.Repeat("─", topFill)+"╮")

	line := func(content string) string {
		lineWidth := lipgloss.Width(content)
		pad := max(innerWidth-lineWidth, 0)
		return stBorder.Render("│ ") + content + strings.Repeat(" ", pad) + stBorder.Render(" │")
	}

	rows := []string{top, line("")}

	if state.typeName != "" {
		// Type-to-confirm mode
		rows = append(rows,
			line(state.message),
			line(""),
			line("Type '"+state.typeName+"' to confirm:"),
			line(stInput.Render("> "+state.input+styleBold.Render("_"))),
			line(""),
			line(stHint.Render("↵ Confirm  Esc Cancel")),
		)
	} else {
		// Simple y/n mode
		rows = append(rows,
			line(state.message),
			line(""),
			line(stHint.Render("[y] Yes  [n/Esc] No")),
		)
	}

	rows = append(rows,
		stBorder.Render("╰"+strings.Repeat("─", width-2)+"╯"),
	)

	return strings.Join(rows, "\n")
}
