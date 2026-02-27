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

func TestView_ListViews(t *testing.T) {
	tests := []struct {
		want string
		view View
	}{
		{"Agents", ViewAgents},
		{"Deployments", ViewDeploys},
		{"Containers", ViewContainers},
		{"Engine", ViewEngine},
		{"Events", ViewEvents},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			m := New(nil, 5*time.Second)
			m.width = 120
			m.height = 40
			m.activeView = tt.view
			m.data = &DashboardData{
				Engine:  EngineData{Status: "running", StartedAt: time.Now()},
				Summary: ClusterSummary{TotalAgents: 1, ConnectedAgents: 1},
			}

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

func TestEscBack_AgentDetailToList(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.activeView = ViewAgentDetail
	m.selectedAgent = "worker-1"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model := updated.(Model)

	if model.activeView != ViewAgents {
		t.Errorf("Esc from AgentDetail: view = %d, want ViewAgents", model.activeView)
	}
}

func TestEscBack_DeployDetailToList(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.activeView = ViewDeploymentDetail
	m.selectedDeploy = "dep-001"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model := updated.(Model)

	if model.activeView != ViewDeploys {
		t.Errorf("Esc from DeploymentDetail: view = %d, want ViewDeploys", model.activeView)
	}
}

func TestEscBack_ListToOverview(t *testing.T) {
	for _, view := range []View{ViewAgents, ViewDeploys, ViewContainers, ViewEngine, ViewEvents} {
		m := New(nil, 5*time.Second)
		m.activeView = view

		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
		model := updated.(Model)

		if model.activeView != ViewOverview {
			t.Errorf("Esc from view %d: got %d, want ViewOverview", view, model.activeView)
		}
	}
}

func TestEnterKey_AgentList(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.activeView = ViewAgents
	m.listCursor = 0
	m.data = &DashboardData{
		Agents: []AgentData{{Name: "worker-1"}},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(Model)

	if model.activeView != ViewAgentDetail {
		t.Errorf("Enter in agent list: view = %d, want ViewAgentDetail", model.activeView)
	}
	if model.selectedAgent != "worker-1" {
		t.Errorf("selectedAgent = %q, want worker-1", model.selectedAgent)
	}
}

func TestEnterKey_DeployList(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.activeView = ViewDeploys
	m.listCursor = 0
	m.data = &DashboardData{
		Deployments: []DeploymentData{{ID: "dep-001", Name: "my-app", Status: "running", CreatedAt: time.Now()}},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(Model)

	if model.activeView != ViewDeploymentDetail {
		t.Errorf("Enter in deploy list: view = %d, want ViewDeploymentDetail", model.activeView)
	}
	if model.selectedDeploy != "dep-001" {
		t.Errorf("selectedDeploy = %q, want dep-001", model.selectedDeploy)
	}
}

func TestListCursor_UpDown(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.activeView = ViewAgents
	m.data = &DashboardData{
		Agents: []AgentData{{Name: "a"}, {Name: "b"}, {Name: "c"}},
	}

	// Down
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	model := updated.(Model)
	if model.listCursor != 1 {
		t.Errorf("j: cursor = %d, want 1", model.listCursor)
	}

	// Up
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	model = updated.(Model)
	if model.listCursor != 0 {
		t.Errorf("k: cursor = %d, want 0", model.listCursor)
	}

	// Up at 0 stays at 0
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(Model)
	if model.listCursor != 0 {
		t.Errorf("up at 0: cursor = %d, want 0", model.listCursor)
	}
}

func TestListCursor_ClampedAtMax(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.activeView = ViewAgents
	m.listCursor = 1
	m.data = &DashboardData{
		Agents: []AgentData{{Name: "only-one"}},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	model := updated.(Model)
	if model.listCursor != 0 {
		t.Errorf("down past max: cursor = %d, want 0", model.listCursor)
	}
}

func TestRenderFooter_ListViews(t *testing.T) {
	got := renderFooter(120, ViewAgents)
	if !strings.Contains(got, "Navigate") {
		t.Error("agent list footer missing Navigate hint")
	}
	if !strings.Contains(got, "Detail") {
		t.Error("agent list footer missing Detail hint")
	}
	if !strings.Contains(got, "Palette") {
		t.Error("agent list footer missing Palette hint")
	}
}

func TestRenderFooter_DetailViews(t *testing.T) {
	got := renderFooter(120, ViewAgentDetail)
	if !strings.Contains(got, "Back") {
		t.Error("detail footer missing Back hint")
	}
	if !strings.Contains(got, "Palette") {
		t.Error("detail footer missing Palette hint")
	}
}

func TestRenderFooter_ContainerList(t *testing.T) {
	got := renderFooter(120, ViewContainers)
	if !strings.Contains(got, "Navigate") {
		t.Error("container list footer missing Navigate hint")
	}
	// Container list should NOT show Detail (no container detail view)
	if strings.Contains(got, "Detail") {
		t.Error("container list footer should not show Detail hint")
	}
}

func TestRenderFooter_EventList(t *testing.T) {
	got := renderFooter(120, ViewEvents)
	if !strings.Contains(got, "Navigate") {
		t.Error("event list footer missing Navigate hint")
	}
	if strings.Contains(got, "Detail") {
		t.Error("event list footer should not show Detail hint")
	}
}

func TestKey5_Engine(t *testing.T) {
	m := New(nil, 5*time.Second)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	model := updated.(Model)
	if model.activeView != ViewEngine {
		t.Errorf("key 5: view = %d, want ViewEngine", model.activeView)
	}
}

func TestKey6_Events(t *testing.T) {
	m := New(nil, 5*time.Second)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")})
	model := updated.(Model)
	if model.activeView != ViewEvents {
		t.Errorf("key 6: view = %d, want ViewEvents", model.activeView)
	}
	if model.listCursor != 0 {
		t.Errorf("key 6: listCursor = %d, want 0", model.listCursor)
	}
	if model.listOffset != 0 {
		t.Errorf("key 6: listOffset = %d, want 0", model.listOffset)
	}
}

func TestIsListView(t *testing.T) {
	tests := []struct {
		view View
		want bool
	}{
		{ViewAgents, true},
		{ViewDeploys, true},
		{ViewContainers, true},
		{ViewEvents, true},
		{ViewOverview, false},
		{ViewEngine, false},
		{ViewAgentDetail, false},
		{ViewDeploymentDetail, false},
	}

	for _, tt := range tests {
		m := New(nil, 5*time.Second)
		m.activeView = tt.view
		if got := m.isListView(); got != tt.want {
			t.Errorf("isListView(view=%d) = %v, want %v", tt.view, got, tt.want)
		}
	}
}

func TestAdjustListScroll_ScrollsDown(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.activeView = ViewContainers
	m.height = 17 // viewport = 17 - 7 = 10 data rows
	m.data = &DashboardData{}
	for i := range 20 {
		m.data.Containers = append(m.data.Containers, ContainerData{Name: string(rune('a' + i))})
	}

	// Move cursor past the viewport
	m.listCursor = 12
	m.adjustListScroll()

	if m.listOffset == 0 {
		t.Error("adjustListScroll should scroll down when cursor exceeds viewport")
	}
	// Cursor should be within viewport: offset <= cursorLine < offset + viewport
	if m.listCursor < m.listOffset {
		t.Errorf("cursor %d is above offset %d", m.listCursor, m.listOffset)
	}
}

func TestAdjustListScroll_ScrollsUp(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.activeView = ViewAgents
	m.height = 17
	m.listOffset = 5
	m.data = &DashboardData{
		Agents: make([]AgentData, 20),
	}

	// Move cursor above the offset
	m.listCursor = 2
	m.adjustListScroll()

	if m.listOffset != 2 {
		t.Errorf("adjustListScroll: offset = %d, want 2", m.listOffset)
	}
}

func TestCursorContentLine_Agents(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.activeView = ViewAgents
	m.data = &DashboardData{Agents: make([]AgentData, 10)}
	m.listCursor = 5

	if got := m.cursorContentLine(); got != 5 {
		t.Errorf("cursorContentLine = %d, want 5", got)
	}
}

func TestCursorContentLine_Deploys_WithSummaries(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.activeView = ViewDeploys
	now := time.Now()
	// Create 3 deployments for "app-a" (will have summary) and 1 for "app-b"
	m.data = &DashboardData{
		Deployments: []DeploymentData{
			{Name: "app-a", Status: "running", CreatedAt: now},
			{Name: "app-a", Status: "stopped", CreatedAt: now.Add(-time.Hour)},
			{Name: "app-a", Status: "stopped", CreatedAt: now.Add(-2 * time.Hour)},
			{Name: "app-b", Status: "running", CreatedAt: now},
		},
	}

	// Group 0 = app-a (has summary: 2 older), Group 1 = app-b
	// Cursor at group 1 → line should be 2 (app-a data=0, app-a summary=1, app-b data=2)
	m.listCursor = 1
	if got := m.cursorContentLine(); got != 2 {
		t.Errorf("cursorContentLine = %d, want 2", got)
	}
}

func TestApplyListScroll_NoScrollNeeded(t *testing.T) {
	content := "╭─ Title ─╮\n│ Header  │\n│ row 0   │\n│ row 1   │\n╰─────────╯"
	got := applyListScroll(content, 0, 10)
	if got != content {
		t.Error("applyListScroll should return content unchanged when it fits")
	}
}

func TestApplyListScroll_ScrollsDataRows(t *testing.T) {
	lines := []string{
		"╭─ Title ─╮",
		"│ Header  │",
		"│ row 0   │",
		"│ row 1   │",
		"│ row 2   │",
		"│ row 3   │",
		"│ row 4   │",
		"╰─────────╯",
	}
	content := strings.Join(lines, "\n")

	// availHeight=5: sticky top(2) + viewport(2) + sticky bottom(1) = 5
	got := applyListScroll(content, 2, 5)
	gotLines := strings.Split(got, "\n")

	if len(gotLines) != 5 {
		t.Fatalf("got %d lines, want 5", len(gotLines))
	}
	// Should keep header
	if gotLines[0] != lines[0] {
		t.Error("should keep box top border")
	}
	if gotLines[1] != lines[1] {
		t.Error("should keep table header")
	}
	// Should show rows starting from offset 2
	if gotLines[2] != lines[4] { // "│ row 2   │"
		t.Errorf("row 0 = %q, want %q", gotLines[2], lines[4])
	}
	if gotLines[3] != lines[5] { // "│ row 3   │"
		t.Errorf("row 1 = %q, want %q", gotLines[3], lines[5])
	}
	// Bottom border
	if gotLines[4] != lines[7] {
		t.Error("should keep box bottom border")
	}
}

func TestViewSwitchResetsOffset(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.listOffset = 5

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	model := updated.(Model)
	if model.listOffset != 0 {
		t.Errorf("switching view should reset listOffset, got %d", model.listOffset)
	}
}
