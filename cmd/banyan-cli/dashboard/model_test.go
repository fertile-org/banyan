package dashboard

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNew(t *testing.T) {
	m := New(nil, 5*time.Second)
	if m.activeView != ViewOverview {
		t.Errorf("initial view = %d, want ViewOverview", m.activeView)
	}
	if m.refreshInterval != 5*time.Second {
		t.Errorf("refresh interval = %v, want 5s", m.refreshInterval)
	}
}

func TestUpdate_WindowSize(t *testing.T) {
	m := New(nil, 5*time.Second)
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := updated.(Model)

	if model.width != 120 {
		t.Errorf("width = %d, want 120", model.width)
	}
	if model.height != 40 {
		t.Errorf("height = %d, want 40", model.height)
	}
	if cmd != nil {
		t.Error("expected nil cmd for window size")
	}
}

func TestUpdate_KeyNavigation(t *testing.T) {
	tests := []struct {
		key      string
		wantView View
	}{
		{"1", ViewOverview},
		{"2", ViewAgents},
		{"3", ViewDeploys},
		{"4", ViewContainers},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			m := New(nil, 5*time.Second)
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
			model := updated.(Model)
			if model.activeView != tt.wantView {
				t.Errorf("key %q → view %d, want %d", tt.key, model.activeView, tt.wantView)
			}
		})
	}
}

func TestUpdate_DataMsg(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.err = errors.New("old error")

	data := DashboardData{
		Engine: EngineData{Status: "running"},
	}

	updated, cmd := m.Update(dataMsg{data: data})
	model := updated.(Model)

	if model.data == nil {
		t.Fatal("data should not be nil after dataMsg")
	}
	if model.data.Engine.Status != "running" {
		t.Errorf("engine status = %q, want running", model.data.Engine.Status)
	}
	if model.err != nil {
		t.Error("error should be cleared after successful data")
	}
	if cmd != nil {
		t.Error("expected nil cmd for dataMsg")
	}
}

func TestUpdate_ErrMsg(t *testing.T) {
	m := New(nil, 5*time.Second)
	testErr := errors.New("connection refused")

	updated, _ := m.Update(errMsg{err: testErr})
	model := updated.(Model)

	if model.err == nil || model.err.Error() != "connection refused" {
		t.Errorf("err = %v, want connection refused", model.err)
	}
}

func TestView_Initializing(t *testing.T) {
	m := New(nil, 5*time.Second)
	// width=0 means not yet sized
	got := m.View()
	if got != "Initializing..." {
		t.Errorf("view = %q, want Initializing...", got)
	}
}

func TestView_OverviewWithData(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.width = 100
	m.height = 40
	m.data = &DashboardData{
		Engine: EngineData{
			Status:    "running",
			StartedAt: time.Now().Add(-time.Hour),
			CPU:       0.25,
			MemUsed:   512 * 1024 * 1024,
			MemTotal:  1024 * 1024 * 1024,
		},
		Summary: ClusterSummary{
			TotalAgents:     2,
			ConnectedAgents: 2,
		},
	}

	got := m.View()
	if !strings.Contains(got, "Banyan Dashboard") {
		t.Error("view missing dashboard title")
	}
	if !strings.Contains(got, "Engine") {
		t.Error("view missing Engine card")
	}
	if !strings.Contains(got, "Overview") {
		t.Error("view missing Overview tab")
	}
}

func TestView_ErrorWithNoData(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.width = 80
	m.height = 30
	m.err = errors.New("engine unreachable")

	got := m.View()
	if !strings.Contains(got, "Error") {
		t.Error("view should show error box when no data")
	}
}

func TestView_ErrorWithStaleData(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.width = 80
	m.height = 30
	m.err = errors.New("timeout")
	m.data = &DashboardData{
		Engine: EngineData{Status: "running"},
	}

	got := m.View()
	// Should show warning but also the stale data
	if !strings.Contains(got, "Engine") {
		t.Error("view should show stale data when error with existing data")
	}
}

func TestView_PlaceholderViews(t *testing.T) {
	tests := []struct {
		view View
		want string
	}{
		{ViewAgents, "coming soon"},
		{ViewDeploys, "coming soon"},
		{ViewContainers, "coming soon"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			m := New(nil, 5*time.Second)
			m.width = 80
			m.height = 30
			m.activeView = tt.view

			got := m.View()
			if !strings.Contains(got, tt.want) {
				t.Errorf("view %d missing %q", tt.view, tt.want)
			}
		})
	}
}

func TestRenderHeader(t *testing.T) {
	got := renderHeader(80, 5*time.Second)
	if !strings.Contains(got, "Banyan Dashboard") {
		t.Error("header missing title")
	}
	if !strings.Contains(got, "5s") {
		t.Error("header missing refresh interval")
	}
}

func TestRenderFooter(t *testing.T) {
	got := renderFooter(80, ViewOverview)
	if !strings.Contains(got, "Overview") {
		t.Error("footer missing Overview tab")
	}
	if !strings.Contains(got, "Quit") {
		t.Error("footer missing quit hint")
	}
}

func TestRenderPlaceholder(t *testing.T) {
	got := renderPlaceholder("Test", "A placeholder")
	if !strings.Contains(got, "Test") {
		t.Error("placeholder missing title")
	}
	if !strings.Contains(got, "A placeholder") {
		t.Error("placeholder missing message")
	}
}

func TestRenderErrorView(t *testing.T) {
	got := renderErrorView(errors.New("test error"), 60)
	if !strings.Contains(got, "test error") {
		t.Error("error view missing error message")
	}
	if !strings.Contains(got, "Retrying") {
		t.Error("error view missing retry message")
	}
}

func TestSplitJoinLines(t *testing.T) {
	input := "line1\nline2\nline3"
	lines := splitLines(input)
	if len(lines) != 3 {
		t.Errorf("splitLines count = %d, want 3", len(lines))
	}

	rejoined := joinLines(lines)
	if rejoined != input {
		t.Errorf("joinLines = %q, want %q", rejoined, input)
	}
}
