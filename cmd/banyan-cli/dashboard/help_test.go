package dashboard

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRenderHelpBox_Content(t *testing.T) {
	got := renderHelpBox(50)

	if !strings.Contains(got, "Keyboard Shortcuts") {
		t.Error("help missing title")
	}
	if !strings.Contains(got, "Navigation") {
		t.Error("help missing Navigation section")
	}
	if !strings.Contains(got, "List Views") {
		t.Error("help missing List Views section")
	}
	if !strings.Contains(got, "General") {
		t.Error("help missing General section")
	}
	if !strings.Contains(got, "Command palette") {
		t.Error("help missing palette shortcut")
	}
	if !strings.Contains(got, "Esc") {
		t.Error("help missing Esc hint")
	}
}

func TestRenderHelpBox(t *testing.T) {
	got := renderHelpBox(50)

	lines := strings.Split(got, "\n")
	// Should have borders + content
	if len(lines) < 10 {
		t.Errorf("help box lines = %d, want at least 10", len(lines))
	}
	if !strings.Contains(lines[0], "╭") {
		t.Error("help box missing top border")
	}
	if !strings.Contains(lines[len(lines)-1], "╯") {
		t.Error("help box missing bottom border")
	}
}

func TestHelpOpen(t *testing.T) {
	m := New(nil, 5*time.Second)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	model := updated.(Model)

	if !model.helpOpen {
		t.Error("pressing '?' should open help")
	}
}

func TestHelpClose_QuestionMark(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.helpOpen = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	model := updated.(Model)

	if model.helpOpen {
		t.Error("pressing '?' again should close help")
	}
}

func TestHelpClose_Esc(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.helpOpen = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model := updated.(Model)

	if model.helpOpen {
		t.Error("Esc should close help")
	}
}

func TestHelpRendersInView(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.width = 80
	m.height = 40
	m.helpOpen = true

	got := m.View()
	if !strings.Contains(got, "Keyboard Shortcuts") {
		t.Error("view with help open should show help overlay")
	}
	// Background dashboard content should still be visible (overlay, not replace)
	if !strings.Contains(got, "Banyan Dashboard") {
		t.Error("help overlay should show dashboard content in background")
	}
}

func TestHelpBlocksOtherKeys(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.helpOpen = true
	m.activeView = ViewOverview

	// Pressing 'p' while help is open should NOT open palette
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	model := updated.(Model)

	if model.paletteOpen {
		t.Error("help overlay should block other key handlers")
	}
	if !model.helpOpen {
		t.Error("help should remain open for unhandled keys")
	}
}

func TestFooterHasHelpHint(t *testing.T) {
	got := renderFooter(120, ViewOverview)
	if !strings.Contains(got, "Help") {
		t.Error("footer missing Help hint")
	}
}
