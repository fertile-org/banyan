package dashboard

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFilterPaletteActions_Empty(t *testing.T) {
	actions := filterPaletteActions("")
	if len(actions) != len(allPaletteActions) {
		t.Errorf("empty filter: got %d actions, want %d", len(actions), len(allPaletteActions))
	}
}

func TestFilterPaletteActions_MatchesName(t *testing.T) {
	actions := filterPaletteActions("agent")
	if len(actions) != 1 {
		t.Fatalf("filter 'agent': got %d actions, want 1", len(actions))
	}
	if actions[0].name != "Agents" {
		t.Errorf("expected Agents, got %q", actions[0].name)
	}
}

func TestFilterPaletteActions_MatchesDescription(t *testing.T) {
	actions := filterPaletteActions("cluster")
	if len(actions) != 1 {
		t.Fatalf("filter 'cluster': got %d actions, want 1", len(actions))
	}
	if actions[0].name != "Overview" {
		t.Errorf("expected Overview, got %q", actions[0].name)
	}
}

func TestFilterPaletteActions_CaseInsensitive(t *testing.T) {
	actions := filterPaletteActions("DEPLOY")
	if len(actions) != 1 {
		t.Fatalf("filter 'DEPLOY': got %d actions, want 1", len(actions))
	}
	if actions[0].name != "Deployments" {
		t.Errorf("expected Deployments, got %q", actions[0].name)
	}
}

func TestFilterPaletteActions_NoMatch(t *testing.T) {
	actions := filterPaletteActions("zzzzz")
	if len(actions) != 0 {
		t.Errorf("filter 'zzzzz': got %d actions, want 0", len(actions))
	}
}

func TestRenderPaletteBox_Content(t *testing.T) {
	got := renderPaletteBox("", 0, 50)

	if !strings.Contains(got, "Command Palette") {
		t.Error("palette missing title")
	}
	if !strings.Contains(got, ">") {
		t.Error("palette missing search prompt")
	}
	if !strings.Contains(got, "Overview") {
		t.Error("palette missing Overview action")
	}
	if !strings.Contains(got, "Agents") {
		t.Error("palette missing Agents action")
	}
	if !strings.Contains(got, "Esc Close") {
		t.Error("palette missing hint")
	}
}

func TestRenderPaletteBox_WithFilter(t *testing.T) {
	got := renderPaletteBox("eng", 0, 50)

	if !strings.Contains(got, "eng") {
		t.Error("palette missing filter text")
	}
	if !strings.Contains(got, "Engine") {
		t.Error("palette missing Engine action")
	}
}

func TestRenderPaletteBox(t *testing.T) {
	got := renderPaletteBox("", 0, 50)

	lines := strings.Split(got, "\n")
	// top + search + empty + 7 actions + empty + hint + bottom = 12
	if len(lines) < 10 {
		t.Errorf("palette box lines = %d, want at least 10", len(lines))
	}
}

func TestRenderPaletteBox_EmptyFilter(t *testing.T) {
	got := renderPaletteBox("zzz", 0, 50)
	if !strings.Contains(got, "No matching") {
		t.Error("palette with no matches should show 'No matching' message")
	}
}

func TestPaletteOpen(t *testing.T) {
	m := New(nil, 5)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	model := updated.(Model)

	if !model.paletteOpen {
		t.Error("pressing 'p' should open palette")
	}
	if model.paletteFilter != "" {
		t.Error("palette filter should be empty on open")
	}
	if model.paletteCursor != 0 {
		t.Error("palette cursor should be 0 on open")
	}
}

func TestPaletteClose(t *testing.T) {
	m := New(nil, 5)
	m.paletteOpen = true
	m.paletteFilter = "test"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model := updated.(Model)

	if model.paletteOpen {
		t.Error("Esc should close palette")
	}
}

func TestPaletteNavigation(t *testing.T) {
	m := New(nil, 5)
	m.paletteOpen = true

	// Down
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	model := updated.(Model)
	if model.paletteCursor != 1 {
		t.Errorf("down: cursor = %d, want 1", model.paletteCursor)
	}

	// Up
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(Model)
	if model.paletteCursor != 0 {
		t.Errorf("up: cursor = %d, want 0", model.paletteCursor)
	}

	// Up at top stays at 0
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(Model)
	if model.paletteCursor != 0 {
		t.Errorf("up at top: cursor = %d, want 0", model.paletteCursor)
	}
}

func TestPaletteTyping(t *testing.T) {
	m := New(nil, 5)
	m.paletteOpen = true

	// Type "ag"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model := updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	model = updated.(Model)

	if model.paletteFilter != "ag" {
		t.Errorf("filter = %q, want 'ag'", model.paletteFilter)
	}

	// Backspace
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model = updated.(Model)
	if model.paletteFilter != "a" {
		t.Errorf("after backspace: filter = %q, want 'a'", model.paletteFilter)
	}
}

func TestPaletteSelectAction(t *testing.T) {
	m := New(nil, 5)
	m.paletteOpen = true
	m.paletteCursor = 1 // Agents

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(Model)

	if model.paletteOpen {
		t.Error("palette should close after selection")
	}
	if model.activeView != ViewAgents {
		t.Errorf("view = %d, want ViewAgents (%d)", model.activeView, ViewAgents)
	}
}

func TestPaletteSelectQuit(t *testing.T) {
	m := New(nil, 5)
	m.paletteOpen = true

	// Find Quit action index
	for i, a := range allPaletteActions {
		if a.name == "Quit" {
			m.paletteCursor = i
			break
		}
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Error("selecting Quit should return a cmd")
	}
}

func TestPaletteSelectWithFilter(t *testing.T) {
	m := New(nil, 5)
	m.paletteOpen = true
	m.paletteFilter = "eng" // filters to just Engine

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(Model)

	if model.activeView != ViewEngine {
		t.Errorf("view = %d, want ViewEngine (%d)", model.activeView, ViewEngine)
	}
}

func TestPaletteSelectNoResults(t *testing.T) {
	m := New(nil, 5)
	m.paletteOpen = true
	m.paletteFilter = "zzzzz" // no matches

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(Model)

	// Should remain open since nothing to select
	if !model.paletteOpen {
		t.Error("palette should stay open when no matches")
	}
}

func TestPaletteRendersInView(t *testing.T) {
	m := New(nil, 5)
	m.width = 80
	m.height = 40
	m.paletteOpen = true

	got := m.View()
	if !strings.Contains(got, "Command Palette") {
		t.Error("view with palette open should show palette overlay")
	}
	// Background dashboard content should still be visible (overlay, not replace)
	if !strings.Contains(got, "Banyan Dashboard") {
		t.Error("palette overlay should show dashboard content in background")
	}
}
