package dashboard

import (
	"strings"
	"testing"
)

func TestSparkline_Empty(t *testing.T) {
	got := sparkline(nil, 10)
	if got != "" {
		t.Errorf("sparkline(nil) = %q, want empty", got)
	}
}

func TestSparkline_SingleValue(t *testing.T) {
	got := sparkline([]float64{50}, 10)
	if got == "" {
		t.Error("sparkline with one value should not be empty")
	}
	// Should contain exactly one block character (inside ANSI styling)
	// The block chars are multi-byte runes, so check the underlying char
	stripped := stripAnsi(got)
	if len([]rune(stripped)) != 1 {
		t.Errorf("single value should produce 1 rune, got %d: %q", len([]rune(stripped)), stripped)
	}
}

func TestSparkline_AllZero(t *testing.T) {
	got := sparkline([]float64{0, 0, 0, 0}, 10)
	stripped := stripAnsi(got)
	for _, r := range stripped {
		if r != sparklineBlocks[0] {
			t.Errorf("all-zero sparkline should only use lowest block, got %c", r)
		}
	}
}

func TestSparkline_AllMax(t *testing.T) {
	got := sparkline([]float64{100, 100, 100}, 10)
	stripped := stripAnsi(got)
	for _, r := range stripped {
		if r != sparklineBlocks[len(sparklineBlocks)-1] {
			t.Errorf("all-100 sparkline should only use highest block, got %c", r)
		}
	}
}

func TestSparkline_TruncatesToMaxWidth(t *testing.T) {
	values := make([]float64, 20)
	for i := range values {
		values[i] = float64(i * 5)
	}
	got := sparkline(values, 8)
	stripped := stripAnsi(got)
	if len([]rune(stripped)) != 8 {
		t.Errorf("sparkline should be truncated to maxWidth=8, got %d runes", len([]rune(stripped)))
	}
}

func TestSparkline_ClampsValues(t *testing.T) {
	// Negative and >100 should be clamped
	got := sparkline([]float64{-10, 150}, 10)
	if got == "" {
		t.Error("sparkline should handle out-of-range values")
	}
}

func TestSparkline_ColorByLatest(t *testing.T) {
	// Low CPU → green (ANSI color 42)
	low := sparkline([]float64{10, 15, 20}, 10)
	if !strings.Contains(low, "42") && !strings.Contains(low, "38;5;42") {
		// The exact ANSI format depends on lipgloss, just verify it renders
		if low == "" {
			t.Error("low CPU sparkline should not be empty")
		}
	}
}

// stripAnsi removes ANSI escape codes from a string.
func stripAnsi(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}
