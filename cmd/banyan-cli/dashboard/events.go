package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderEventList renders the event log view with cursor, newest events first.
func renderEventList(data *DashboardData, width, cursor int) string {
	if data == nil {
		return styleDim.Render("  Loading...")
	}
	if len(data.Events) == 0 {
		return renderBox("Events", styleDim.Render("No events yet"), width)
	}

	const (
		colTime     = 12
		colIcon     = 4
		colType     = 22
		colSeverity = 12
	)

	header := "    " +
		padRight(styleBold.Render("Time"), colTime) +
		padRight("", colIcon) +
		padRight(styleBold.Render("Type"), colType) +
		padRight(styleBold.Render("Severity"), colSeverity) +
		styleBold.Render("Message")

	var rows []string
	rows = append(rows, header)

	// Subtract column widths and border/prefix padding (8 chars) from total width.
	msgWidth := max(width-colTime-colIcon-colType-colSeverity-8, 10)

	for i := range data.Events {
		e := &data.Events[i]

		ts := e.Timestamp.Format("15:04:05")
		icon := eventIcon(e.Type, e.Severity)
		severity := severityLabel(e.Severity)

		prefix := "  "
		nameStyle := styleNone
		if i == cursor {
			prefix = styleSelected.Render("> ")
			nameStyle = styleSelected
		}

		row := prefix +
			padRight(nameStyle.Render(ts), colTime) +
			padRight(icon, colIcon) +
			padRight(truncate(e.Type, colType-2), colType) +
			padRight(severity, colSeverity) +
			truncate(e.Message, msgWidth)

		rows = append(rows, row)
	}

	title := fmt.Sprintf("Events (%d)", len(data.Events))
	return renderBox(title, strings.Join(rows, "\n"), width)
}

// severityLabel returns a colored severity label.
func severityLabel(severity string) string {
	switch severity {
	case "error":
		return styleError.Render("error")
	case "warning":
		return lipgloss.NewStyle().Foreground(colorYellow).Render("warning")
	default:
		return styleDim.Render(severity)
	}
}
