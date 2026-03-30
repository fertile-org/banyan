package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Colors
var (
	colorGreen   = lipgloss.Color("42")
	colorYellow  = lipgloss.Color("214")
	colorRed     = lipgloss.Color("196")
	colorGray    = lipgloss.Color("241")
	colorBlue    = lipgloss.Color("39")
	colorPrimary = lipgloss.Color("#7CB342")
	colorDim     = lipgloss.Color("245")
	colorBorder  = lipgloss.Color("#558B2F")
)

// Text styles
var (
	styleBold     = lipgloss.NewStyle().Bold(true)
	styleDim      = lipgloss.NewStyle().Foreground(colorDim)
	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	styleLabel    = lipgloss.NewStyle().Foreground(colorDim)
	styleError    = lipgloss.NewStyle().Foreground(colorRed)
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	styleNone     = lipgloss.NewStyle()
)

// statusDot returns a colored dot for the given status.
func statusDot(status string) string {
	switch status {
	case "running", "ready", "up", "completed":
		return lipgloss.NewStyle().Foreground(colorGreen).Render("●")
	case "pending", "deploying":
		return lipgloss.NewStyle().Foreground(colorYellow).Render("●")
	case "failed", "error", "stale":
		return lipgloss.NewStyle().Foreground(colorRed).Render("●")
	default:
		return lipgloss.NewStyle().Foreground(colorGray).Render("●")
	}
}

// progressBar renders a colored progress bar of the given width.
// Color adapts to severity: green < 50%, yellow 50-80%, red > 80%.
func progressBar(ratio float64, width int) string {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := min(int(ratio*float64(width)), width)
	empty := width - filled

	var color lipgloss.Color
	switch {
	case ratio >= 0.8:
		color = colorRed
	case ratio >= 0.5:
		color = colorYellow
	default:
		color = colorGreen
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	return lipgloss.NewStyle().Foreground(color).Render(bar)
}

// healthBar renders a colored bar where green means fully healthy.
// Unlike progressBar (where high = bad), here high ratio = good.
func healthBar(ratio float64, width int) string {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := min(int(ratio*float64(width)), width)
	empty := width - filled

	var color lipgloss.Color
	switch {
	case ratio >= 1.0:
		color = colorGreen
	case ratio >= 0.5:
		color = colorYellow
	default:
		color = colorRed
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	return lipgloss.NewStyle().Foreground(color).Render(bar)
}

// formatDuration renders a human-friendly duration.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	hours := int(d.Hours())
	if hours < 24 {
		return fmt.Sprintf("%dh %dm", hours, int(d.Minutes())%60)
	}
	days := hours / 24
	return fmt.Sprintf("%dd %dh", days, hours%24)
}

// formatBytes renders bytes as a human-readable string.
func formatBytes(b uint64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.0f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.0f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// renderBox draws a bordered box with a title embedded in the top border.
func renderBox(title, content string, width int) string {
	stTitle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	stBorder := lipgloss.NewStyle().Foreground(colorBorder)

	renderedTitle := stTitle.Render(title)
	titleWidth := lipgloss.Width(renderedTitle)

	// Top border: ╭─ Title ─────╮ (5 chars for border decorations)
	topFill := max(width-5-titleWidth, 1)
	top := stBorder.Render("╭─ ") + renderedTitle + stBorder.Render(" "+strings.Repeat("─", topFill)+"╮")

	// Content lines with left/right border (4 chars for border padding)
	lines := strings.Split(content, "\n")
	innerWidth := width - 4
	var middle []string
	for _, line := range lines {
		lineWidth := lipgloss.Width(line)
		pad := max(innerWidth-lineWidth, 0)
		middle = append(middle, stBorder.Render("│ ")+line+strings.Repeat(" ", pad)+stBorder.Render(" │"))
	}

	// Bottom: ╰─────────────╯
	bottom := stBorder.Render("╰" + strings.Repeat("─", width-2) + "╯")

	parts := []string{top}
	parts = append(parts, middle...)
	parts = append(parts, bottom)
	return strings.Join(parts, "\n")
}

// eventIcon returns an icon for the event type.
func eventIcon(eventType, severity string) string {
	if severity == "error" {
		return lipgloss.NewStyle().Foreground(colorRed).Render("✗")
	}
	switch {
	case strings.Contains(eventType, "registered") || strings.Contains(eventType, "connected"):
		return lipgloss.NewStyle().Foreground(colorGreen).Render("+")
	case strings.Contains(eventType, "running"):
		return lipgloss.NewStyle().Foreground(colorGreen).Render("✓")
	case strings.Contains(eventType, "created") || strings.Contains(eventType, "deploy"):
		return lipgloss.NewStyle().Foreground(colorBlue).Render("↻")
	case strings.Contains(eventType, "stopped"):
		return lipgloss.NewStyle().Foreground(colorYellow).Render("■")
	default:
		return lipgloss.NewStyle().Foreground(colorDim).Render("·")
	}
}

// sparklineBlocks are the Unicode block characters used for sparklines, ordered low→high.
var sparklineBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// sparkline renders a mini sparkline from a slice of values (0.0–100.0).
// Returns a colored string of Unicode block characters.
func sparkline(values []float64, maxWidth int) string {
	if len(values) == 0 {
		return ""
	}

	// Use last maxWidth values
	start := 0
	if len(values) > maxWidth {
		start = len(values) - maxWidth
	}
	vals := values[start:]

	var sb strings.Builder
	for _, v := range vals {
		// Clamp to 0–100
		if v < 0 {
			v = 0
		}
		if v > 100 {
			v = 100
		}
		// Map to block index (0–7)
		idx := int(v / 100.0 * float64(len(sparklineBlocks)-1))
		if idx >= len(sparklineBlocks) {
			idx = len(sparklineBlocks) - 1
		}
		sb.WriteRune(sparklineBlocks[idx])
	}

	result := sb.String()

	// Color based on latest value
	latest := vals[len(vals)-1]
	var color lipgloss.Color
	switch {
	case latest >= 80:
		color = colorRed
	case latest >= 50:
		color = colorYellow
	default:
		color = colorGreen
	}
	return lipgloss.NewStyle().Foreground(color).Render(result)
}

// truncate truncates a string to maxLen, adding "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}

// padRight pads s to the given visual width using spaces.
// Unlike %-Ns, this accounts for ANSI escape sequences (colors, bold, etc.)
// that take bytes but render as zero width.
func padRight(s string, width int) string {
	visWidth := lipgloss.Width(s)
	if visWidth >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visWidth)
}
