package dashboard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// applyOverlay composites an overlay box on top of a background view.
// The overlay is centered horizontally and placed 1/3 from the top.
// Background content remains visible on all sides of the overlay box.
func applyOverlay(bg, overlay string, width, height int) string {
	bgLines := strings.Split(bg, "\n")

	// Pad background to fill the screen
	for len(bgLines) < height {
		bgLines = append(bgLines, "")
	}
	if len(bgLines) > height {
		bgLines = bgLines[:height]
	}

	overlayLines := strings.Split(overlay, "\n")
	overlayHeight := len(overlayLines)

	// Measure the widest overlay line
	overlayWidth := 0
	for _, l := range overlayLines {
		if w := lipgloss.Width(l); w > overlayWidth {
			overlayWidth = w
		}
	}

	// Position: 1/3 from top, centered horizontally
	startRow := max((height-overlayHeight)/3, 0)
	startCol := max((width-overlayWidth)/2, 0)

	for i, ol := range overlayLines {
		row := startRow + i
		if row >= len(bgLines) {
			break
		}
		bgLines[row] = compositeLine(bgLines[row], ol, startCol, overlayWidth)
	}

	return strings.Join(bgLines, "\n")
}

// compositeLine places an overlay line segment on top of a background line
// at the given column position. Uses ANSI-aware string operations to preserve
// styled content on both sides of the overlay.
func compositeLine(bgLine, fgLine string, col, fgWidth int) string {
	// Left portion of background (before the overlay)
	left := ansi.Truncate(bgLine, col, "")
	leftW := lipgloss.Width(left)
	if leftW < col {
		left += strings.Repeat(" ", col-leftW)
	}

	// Right portion of background (after the overlay)
	right := ansi.TruncateLeft(bgLine, col+fgWidth, "")

	// Reset ANSI state between segments to prevent color bleed
	return left + "\033[0m" + fgLine + "\033[0m" + right
}
