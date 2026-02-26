package dashboard

import (
	"strings"
	"testing"
	"time"
)

func TestStatusDot(t *testing.T) {
	tests := []struct {
		status string
		want   string // check the dot character is present
	}{
		{"running", "●"},
		{"ready", "●"},
		{"up", "●"},
		{"completed", "●"},
		{"pending", "●"},
		{"deploying", "●"},
		{"failed", "●"},
		{"error", "●"},
		{"stale", "●"},
		{"unknown", "●"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := statusDot(tt.status)
			if !strings.Contains(got, tt.want) {
				t.Errorf("statusDot(%q) = %q, does not contain %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestProgressBar(t *testing.T) {
	tests := []struct {
		name  string
		ratio float64
		width int
	}{
		{"zero", 0, 10},
		{"half", 0.5, 10},
		{"full", 1.0, 10},
		{"over", 1.5, 10},
		{"negative", -0.1, 10},
		{"narrow", 0.3, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := progressBar(tt.ratio, tt.width)
			if got == "" {
				t.Error("progressBar returned empty string")
			}
			// Should contain block characters
			if !strings.Contains(got, "█") && !strings.Contains(got, "░") {
				t.Errorf("progressBar does not contain expected block characters: %q", got)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		want string
		dur  time.Duration
	}{
		{"0s", 0},
		{"30s", 30 * time.Second},
		{"1m 30s", time.Minute + 30*time.Second},
		{"2h 15m", 2*time.Hour + 15*time.Minute},
		{"1d 1h", 25 * time.Hour},
		{"2d 1h", 49 * time.Hour},
		{"0s", -5 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDuration(tt.dur)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.dur, got, tt.want)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		want  string
		bytes uint64
	}{
		{"0 B", 0},
		{"512 B", 512},
		{"1 KB", 1024},
		{"1 MB", 1024 * 1024},
		{"1.0 GB", 1024 * 1024 * 1024},
		{"5.5 GB", 1024*1024*1024*5 + 1024*1024*512},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		want   string
		maxLen int
	}{
		{"hello", "hello", 10},
		{"hello world", "hell…", 5},
		{"hi", "hi", 2},
		{"hello", "…", 1},
		{"", "", 5},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestEventIcon(t *testing.T) {
	tests := []struct {
		name     string
		evType   string
		severity string
	}{
		{"error severity", "any", "error"},
		{"registered", "agent.registered", "info"},
		{"running", "container.running", "info"},
		{"deploy", "deployment.created", "info"},
		{"stopped", "container.stopped", "info"},
		{"default", "some.event", "info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eventIcon(tt.evType, tt.severity)
			if got == "" {
				t.Error("eventIcon returned empty string")
			}
		})
	}
}

func TestRenderBox(t *testing.T) {
	got := renderBox("Test", "Hello", 30)

	if !strings.Contains(got, "╭") {
		t.Error("renderBox missing top-left corner")
	}
	if !strings.Contains(got, "╯") {
		t.Error("renderBox missing bottom-right corner")
	}
	if !strings.Contains(got, "Test") {
		t.Error("renderBox missing title")
	}
	if !strings.Contains(got, "Hello") {
		t.Error("renderBox missing content")
	}

	lines := strings.Split(got, "\n")
	if len(lines) != 3 { // top + 1 content line + bottom
		t.Errorf("renderBox lines = %d, want 3", len(lines))
	}
}

func TestRenderBox_MultilineContent(t *testing.T) {
	got := renderBox("Multi", "line1\nline2\nline3", 40)
	lines := strings.Split(got, "\n")
	if len(lines) != 5 { // top + 3 content lines + bottom
		t.Errorf("renderBox multiline lines = %d, want 5", len(lines))
	}
}

func TestHealthBar(t *testing.T) {
	tests := []struct {
		name  string
		ratio float64
		width int
	}{
		{"zero", 0, 10},
		{"half", 0.5, 10},
		{"full", 1.0, 10},
		{"over", 1.5, 10},
		{"negative", -0.1, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := healthBar(tt.ratio, tt.width)
			if got == "" {
				t.Error("healthBar returned empty string")
			}
			if !strings.Contains(got, "█") && !strings.Contains(got, "░") {
				t.Errorf("healthBar does not contain expected block characters: %q", got)
			}
		})
	}
}

func TestHealthBar_ColorInversion(t *testing.T) {
	// Full health (1.0) should contain green (color 42), not red
	full := healthBar(1.0, 10)
	if strings.Contains(full, "196") {
		t.Error("healthBar(1.0) should not use red color")
	}

	// Empty health (0.0) should contain red, not green
	empty := healthBar(0.0, 10)
	if strings.Contains(empty, "42") {
		t.Error("healthBar(0.0) should not use green color")
	}
}

func TestCapitalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"running", "Running"},
		{"", ""},
		{"R", "R"},
		{"hello world", "Hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := capitalize(tt.input)
			if got != tt.want {
				t.Errorf("capitalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
