package dashboard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestApplyOverlay_PreservesBackground(t *testing.T) {
	// Create a simple background with known content
	bgLines := []string{
		"HEADER LINE ONE",
		"line two here",
		"line three here",
		"line four here",
		"line five here",
		"line six here",
		"line seven here",
		"FOOTER LINE",
	}
	bg := strings.Join(bgLines, "\n")

	overlay := "OVERLAY"

	got := applyOverlay(bg, overlay, 20, 8)

	// Header (row 0) should be preserved (overlay starts at ~row 2 for 1/3 positioning)
	if !strings.Contains(got, "HEADER LINE ONE") {
		t.Error("overlay should preserve header in background")
	}
	// Footer (last row) should be preserved
	if !strings.Contains(got, "FOOTER LINE") {
		t.Error("overlay should preserve footer in background")
	}
	// Overlay content should be present
	if !strings.Contains(got, "OVERLAY") {
		t.Error("overlay content missing")
	}
}

func TestApplyOverlay_CentersHorizontally(t *testing.T) {
	bg := strings.Repeat(".", 40) // 40 dots on one line
	overlay := "BOX"

	// With height=1, overlay goes on row 0; centered in 40 chars
	got := applyOverlay(bg, overlay, 40, 1)

	// "BOX" is 3 chars, centered at col (40-3)/2 = 18
	// So we expect dots before and after
	lines := strings.Split(got, "\n")
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}

	line := lines[0]
	if !strings.Contains(line, "BOX") {
		t.Error("overlay content missing from composited line")
	}
	// Should have dots on both sides
	idx := strings.Index(line, "BOX")
	if idx < 1 {
		t.Error("overlay should not be at the left edge")
	}
}

func TestApplyOverlay_PadsShortBackground(t *testing.T) {
	bg := "short"
	overlay := "OVL"

	// Background has 1 line, but we claim height=5
	got := applyOverlay(bg, overlay, 20, 5)

	lines := strings.Split(got, "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d", len(lines))
	}
	if !strings.Contains(got, "OVL") {
		t.Error("overlay content missing")
	}
}

func TestCompositeLine_PreservesSides(t *testing.T) {
	bgLine := "AAABBBCCC" // 9 chars
	fgLine := "XXX"

	// Place "XXX" at col 3, width 3 → should get "AAAXXXCCC"
	got := compositeLine(bgLine, fgLine, 3, 3)

	// Strip ANSI resets for comparison
	stripped := strings.ReplaceAll(got, "\033[0m", "")
	if stripped != "AAAXXXCCC" {
		t.Errorf("compositeLine = %q, want AAAXXXCCC", stripped)
	}
}

func TestCompositeLine_ShortBackground(t *testing.T) {
	bgLine := "AB"  // only 2 chars
	fgLine := "XXX" // placed at col 5

	got := compositeLine(bgLine, fgLine, 5, 3)

	// Left should be padded to col 5: "AB   "
	stripped := strings.ReplaceAll(got, "\033[0m", "")
	if !strings.Contains(stripped, "XXX") {
		t.Error("overlay content missing")
	}
	// Should have padding before XXX
	if lipgloss.Width(stripped) < 8 { // 5 + 3
		t.Errorf("composited line too short: width=%d", lipgloss.Width(stripped))
	}
}

func TestCompositeLine_EmptyBackground(t *testing.T) {
	got := compositeLine("", "BOX", 10, 3)

	stripped := strings.ReplaceAll(got, "\033[0m", "")
	if !strings.Contains(stripped, "BOX") {
		t.Error("overlay content missing on empty background")
	}
}

func TestApplyOverlay_MultilineOverlay(t *testing.T) {
	bgLines := make([]string, 20)
	for i := range bgLines {
		bgLines[i] = strings.Repeat(".", 60)
	}
	bg := strings.Join(bgLines, "\n")

	overlayLines := []string{
		"╭────────╮",
		"│ HELLO  │",
		"╰────────╯",
	}
	overlay := strings.Join(overlayLines, "\n")

	got := applyOverlay(bg, overlay, 60, 20)

	if !strings.Contains(got, "HELLO") {
		t.Error("multiline overlay content missing")
	}
	if !strings.Contains(got, "╭") {
		t.Error("overlay top border missing")
	}
	if !strings.Contains(got, "╰") {
		t.Error("overlay bottom border missing")
	}

	// Background dots should still be visible on lines outside the overlay
	lines := strings.Split(got, "\n")
	// First line should have dots (overlay starts at row ~5 for height 20)
	if !strings.Contains(lines[0], "..") {
		t.Error("background should be visible on first line")
	}
	// Last line should have dots
	if !strings.Contains(lines[len(lines)-1], "..") {
		t.Error("background should be visible on last line")
	}
}
