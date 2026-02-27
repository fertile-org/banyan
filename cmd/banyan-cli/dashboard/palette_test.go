package dashboard

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFilterPaletteActions_Empty(t *testing.T) {
	// On a list view, all actions (including Filter/Export) are shown.
	actions := filterPaletteActions("", ViewAgents)
	if len(actions) != len(allPaletteActions) {
		t.Errorf("empty filter on list view: got %d actions, want %d", len(actions), len(allPaletteActions))
	}
}

func TestFilterPaletteActions_MatchesName(t *testing.T) {
	actions := filterPaletteActions("agent", ViewOverview)
	if len(actions) != 1 {
		t.Fatalf("filter 'agent': got %d actions, want 1", len(actions))
	}
	if actions[0].name != "Agents" {
		t.Errorf("expected Agents, got %q", actions[0].name)
	}
}

func TestFilterPaletteActions_MatchesDescription(t *testing.T) {
	actions := filterPaletteActions("cluster", ViewOverview)
	if len(actions) != 1 {
		t.Fatalf("filter 'cluster': got %d actions, want 1", len(actions))
	}
	if actions[0].name != "Overview" {
		t.Errorf("expected Overview, got %q", actions[0].name)
	}
}

func TestFilterPaletteActions_CaseInsensitive(t *testing.T) {
	actions := filterPaletteActions("DEPLOY", ViewOverview)
	if len(actions) != 1 {
		t.Fatalf("filter 'DEPLOY': got %d actions, want 1", len(actions))
	}
	if actions[0].name != "Deployments" {
		t.Errorf("expected Deployments, got %q", actions[0].name)
	}
}

func TestFilterPaletteActions_NoMatch(t *testing.T) {
	actions := filterPaletteActions("zzzzz", ViewOverview)
	if len(actions) != 0 {
		t.Errorf("filter 'zzzzz': got %d actions, want 0", len(actions))
	}
}

func TestFilterPaletteActions_ContextAware_ListView(t *testing.T) {
	actions := filterPaletteActions("", ViewContainers)
	var found bool
	for _, a := range actions {
		if a.name == "Filter" || a.name == "Export CSV" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Filter/Export should be available on list views")
	}
}

func TestFilterPaletteActions_ContextAware_NonListView(t *testing.T) {
	actions := filterPaletteActions("", ViewOverview)
	for _, a := range actions {
		if a.name == "Filter" || a.name == "Export CSV" {
			t.Errorf("action %q should NOT be available on Overview", a.name)
		}
	}
}

func TestRenderPaletteBox_Content(t *testing.T) {
	got := renderPaletteBox("", 0, 50, ViewOverview)

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
	got := renderPaletteBox("eng", 0, 50, ViewOverview)

	if !strings.Contains(got, "eng") {
		t.Error("palette missing filter text")
	}
	if !strings.Contains(got, "Engine") {
		t.Error("palette missing Engine action")
	}
}

func TestRenderPaletteBox(t *testing.T) {
	got := renderPaletteBox("", 0, 50, ViewAgents)

	lines := strings.Split(got, "\n")
	// With sections: top + search + empty + actions/headers/gaps + empty + hint + bottom
	if len(lines) < 10 {
		t.Errorf("palette box lines = %d, want at least 10", len(lines))
	}
}

func TestRenderPaletteBox_SectionDividers(t *testing.T) {
	// On a list view, all three sections should be visible
	got := renderPaletteBox("", 0, 60, ViewContainers)

	if !strings.Contains(got, "Actions") {
		t.Error("palette should show Actions section on list views")
	}
	if !strings.Contains(got, "Navigate") {
		t.Error("palette should show Navigate section")
	}
	if !strings.Contains(got, "Commands") {
		t.Error("palette should show Commands section")
	}
}

func TestRenderPaletteBox_NoActionsSectionOnOverview(t *testing.T) {
	got := renderPaletteBox("", 0, 60, ViewOverview)

	// Actions section should not appear on non-list views
	if strings.Contains(got, "Actions") {
		t.Error("palette should NOT show Actions section on Overview")
	}
	if !strings.Contains(got, "Navigate") {
		t.Error("palette should show Navigate section on Overview")
	}
}

func TestRenderPaletteBox_EmptyFilter(t *testing.T) {
	got := renderPaletteBox("zzz", 0, 50, ViewOverview)
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

	// Find Quit action index in the filtered list for the active view
	actions := filterPaletteActions("", m.activeView)
	for i, a := range actions {
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

func TestPaletteSelectFilter(t *testing.T) {
	m := New(nil, 5)
	m.activeView = ViewContainers
	m.paletteOpen = true
	m.paletteFilter = "filter" // filters to just Filter action

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(Model)

	if model.paletteOpen {
		t.Error("palette should close after selecting Filter")
	}
	if !model.filterEditing {
		t.Error("selecting Filter should activate filter editing")
	}
}

func TestPaletteSelectExport(t *testing.T) {
	orig := exportViewCSV
	defer func() { exportViewCSV = orig }()
	exportViewCSV = func(_ *DashboardData, _ View) (string, error) {
		return "test-export.csv", nil
	}

	m := New(nil, 5)
	m.activeView = ViewAgents
	m.data = &DashboardData{}
	m.paletteOpen = true
	m.paletteFilter = "export" // filters to just Export CSV action

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(Model)

	if model.paletteOpen {
		t.Error("palette should close after selecting Export")
	}
	if model.exportStatus != "test-export.csv" {
		t.Errorf("exportStatus = %q, want 'test-export.csv'", model.exportStatus)
	}
}

func TestPaletteFilterNotOnOverview(t *testing.T) {
	actions := filterPaletteActions("filter", ViewOverview)
	if len(actions) != 0 {
		t.Errorf("filter 'filter' on overview: got %d actions, want 0", len(actions))
	}
}

func TestPaletteExportPreservesFilter(t *testing.T) {
	orig := exportViewCSV
	defer func() { exportViewCSV = orig }()

	var exportedData *DashboardData
	exportViewCSV = func(data *DashboardData, _ View) (string, error) {
		exportedData = data
		return "out.csv", nil
	}

	m := New(nil, 5)
	m.activeView = ViewAgents
	m.filterText = "worker"
	m.data = &DashboardData{
		Agents: []AgentData{
			{Name: "worker-1", Status: "ready"},
			{Name: "gateway", Status: "ready"},
		},
	}
	m.paletteOpen = true
	m.paletteFilter = "export"

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if exportedData == nil {
		t.Fatal("export was not called")
	}
	if len(exportedData.Agents) != 1 {
		t.Errorf("export should use filtered data: got %d agents, want 1", len(exportedData.Agents))
	}
}

func TestIsActionAvailable(t *testing.T) {
	tests := []struct {
		name   string
		action paletteAction
		view   View
		want   bool
	}{
		{"global on overview", paletteAction{name: "Refresh", view: viewActionRefresh}, ViewOverview, true},
		{"global on agents", paletteAction{name: "Quit", view: viewActionQuit}, ViewAgents, true},
		{"filter on list view", paletteAction{name: "Filter", availableIn: listViews}, ViewContainers, true},
		{"filter on overview", paletteAction{name: "Filter", availableIn: listViews}, ViewOverview, false},
		{"export on events", paletteAction{name: "Export", availableIn: listViews}, ViewEvents, true},
		{"export on engine", paletteAction{name: "Export", availableIn: listViews}, ViewEngine, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isActionAvailable(&tt.action, tt.view)
			if got != tt.want {
				t.Errorf("isActionAvailable(%q, %d) = %v, want %v", tt.action.name, tt.view, got, tt.want)
			}
		})
	}
}
