package dashboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
)

// View identifies the active dashboard screen.
type View int

const (
	ViewOverview         View = iota
	ViewAgents                // Agent list (selectable)
	ViewDeploys               // Deployment list (selectable)
	ViewContainers            // Flat container list
	ViewEngine                // Engine detail & metrics
	ViewEvents                // Event log (newest first)
	ViewAgentDetail           // Single agent detail
	ViewDeploymentDetail      // Single deployment detail
)

// Model is the main bubbletea model for the dashboard.
type Model struct {
	client          banyanpb.EngineServiceClient
	data            *DashboardData
	err             error
	paletteFilter   string
	selectedAgent   string
	selectedDeploy  string
	filterText      string
	exportStatus    string
	refreshInterval time.Duration
	width           int
	height          int
	activeView      View
	paletteCursor   int
	listCursor      int
	listOffset      int
	paletteOpen     bool
	helpOpen        bool
	filterEditing   bool
}

// New creates a new dashboard model.
func New(client banyanpb.EngineServiceClient, refreshInterval time.Duration) Model {
	return Model{
		client:          client,
		refreshInterval: refreshInterval,
		activeView:      ViewOverview,
	}
}

// Message types

type dataMsg struct{ data DashboardData }
type errMsg struct{ err error }
type tickMsg time.Time

// Init returns the initial commands: fetch data and start the tick loop.
func (m Model) Init() tea.Cmd { //nolint:gocritic // bubbletea requires value receiver
	return tea.Batch(
		fetchDataCmd(m.client),
		tickAfter(m.refreshInterval),
	)
}

// Update handles messages and returns updated model + commands.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea requires value receiver
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.helpOpen {
			return m.handleHelpKey(msg)
		}
		if m.paletteOpen {
			return m.handlePaletteKey(msg)
		}
		if m.filterEditing {
			return m.handleFilterKey(msg)
		}
		return m.handleKey(msg)

	case tickMsg:
		return m, tea.Batch(
			fetchDataCmd(m.client),
			tickAfter(m.refreshInterval),
		)

	case dataMsg:
		m.data = &msg.data
		m.err = nil
		m.exportStatus = ""
		// Clamp cursor to current data bounds after refresh
		if m.isListView() {
			maxIdx := m.maxListIndex()
			if m.listCursor > maxIdx {
				m.listCursor = maxIdx
			}
		}
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil
	}

	return m, nil
}

// handleKey processes key events in normal (non-palette, non-filter) mode.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.helpOpen = true
		return m, nil
	case "p":
		m.paletteOpen = true
		m.paletteFilter = ""
		m.paletteCursor = 0
		m.filterEditing = false
		return m, nil
	case "esc":
		// First Esc clears filter; second Esc navigates back
		if m.filterText != "" {
			m.filterText = ""
			m.listCursor = 0
			m.listOffset = 0
			return m, nil
		}
		switch m.activeView {
		case ViewAgentDetail:
			m.activeView = ViewAgents
		case ViewDeploymentDetail:
			m.activeView = ViewDeploys
		case ViewAgents, ViewDeploys, ViewContainers, ViewEngine, ViewEvents:
			m.activeView = ViewOverview
		}
		return m, nil
	case "/":
		if m.isListView() {
			m.filterEditing = true
			m.listCursor = 0
			m.listOffset = 0
		}
	case "e":
		if m.isListView() && m.data != nil {
			fd := filteredData(m.data, m.activeView, m.filterText)
			filename, err := exportViewCSV(fd, m.activeView)
			if err != nil {
				m.exportStatus = fmt.Sprintf("Export failed: %v", err)
			} else {
				m.exportStatus = filename
			}
		}
	case "1":
		m.activeView = ViewOverview
		m.filterText = ""
		m.filterEditing = false
	case "2":
		m.activeView = ViewAgents
		m.listCursor = 0
		m.listOffset = 0
		m.filterText = ""
		m.filterEditing = false
	case "3":
		m.activeView = ViewDeploys
		m.listCursor = 0
		m.listOffset = 0
		m.filterText = ""
		m.filterEditing = false
	case "4":
		m.activeView = ViewContainers
		m.listCursor = 0
		m.listOffset = 0
		m.filterText = ""
		m.filterEditing = false
	case "5":
		m.activeView = ViewEngine
		m.filterText = ""
		m.filterEditing = false
	case "6":
		m.activeView = ViewEvents
		m.listCursor = 0
		m.listOffset = 0
		m.filterText = ""
		m.filterEditing = false
	case "r":
		return m, fetchDataCmd(m.client)
	case "up", "k":
		m.listCursor = max(m.listCursor-1, 0)
		m.adjustListScroll()
	case "down", "j":
		m.listCursor = m.clampCursor(m.listCursor + 1)
		m.adjustListScroll()
	case "enter":
		return m.handleEnter()
	}
	return m, nil
}

// handleFilterKey processes key events when the filter input is active.
func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	switch msg.String() {
	case "esc":
		m.filterEditing = false
		return m, nil
	case "enter":
		m.filterEditing = false
		return m, nil
	case "up", "k":
		m.listCursor = max(m.listCursor-1, 0)
		m.adjustListScroll()
		return m, nil
	case "down", "j":
		m.listCursor = m.clampCursor(m.listCursor + 1)
		m.adjustListScroll()
		return m, nil
	case "backspace":
		if m.filterText != "" {
			m.filterText = m.filterText[:len(m.filterText)-1]
			m.listCursor = 0
			m.listOffset = 0
		}
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	default:
		if msg.Type == tea.KeyRunes {
			m.filterText += string(msg.Runes)
			m.listCursor = 0
			m.listOffset = 0
		}
		return m, nil
	}
}

// handleHelpKey processes key events when the help overlay is open.
func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	switch msg.String() {
	case "?", "esc", "q":
		m.helpOpen = false
	}
	return m, nil
}

// handlePaletteKey processes key events when the command palette is open.
func (m Model) handlePaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	switch msg.String() {
	case "esc":
		m.paletteOpen = false
		return m, nil
	case "enter":
		actions := filterPaletteActions(m.paletteFilter, m.activeView)
		if len(actions) == 0 {
			return m, nil
		}
		idx := min(m.paletteCursor, len(actions)-1)
		return m.executePaletteAction(actions[idx])
	case "up":
		m.paletteCursor = max(m.paletteCursor-1, 0)
		return m, nil
	case "down":
		actions := filterPaletteActions(m.paletteFilter, m.activeView)
		m.paletteCursor = min(m.paletteCursor+1, max(len(actions)-1, 0))
		return m, nil
	case "backspace":
		if m.paletteFilter != "" {
			m.paletteFilter = m.paletteFilter[:len(m.paletteFilter)-1]
			m.paletteCursor = 0
		}
		return m, nil
	default:
		if msg.Type == tea.KeyRunes {
			m.paletteFilter += string(msg.Runes)
			m.paletteCursor = 0
		}
		return m, nil
	}
}

// executePaletteAction performs the selected palette action.
func (m Model) executePaletteAction(action paletteAction) (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	m.paletteOpen = false
	m.paletteFilter = ""
	m.paletteCursor = 0

	switch action.view {
	case viewActionRefresh:
		return m, fetchDataCmd(m.client)
	case viewActionQuit:
		return m, tea.Quit
	case viewActionFilter:
		m.filterEditing = true
		m.listCursor = 0
		m.listOffset = 0
		return m, nil
	case viewActionExport:
		if m.data != nil {
			fd := filteredData(m.data, m.activeView, m.filterText)
			filename, err := exportViewCSV(fd, m.activeView)
			if err != nil {
				m.exportStatus = fmt.Sprintf("Export failed: %v", err)
			} else {
				m.exportStatus = filename
			}
		}
		return m, nil
	default:
		// View switch — clear filter state
		m.filterText = ""
		m.filterEditing = false
		m.activeView = action.view
		m.listCursor = 0
		m.listOffset = 0
		return m, nil
	}
}

// handleEnter processes Enter key in list views to drill into detail.
func (m Model) handleEnter() (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	fd := filteredData(m.data, m.activeView, m.filterText)
	if fd == nil {
		return m, nil
	}
	switch m.activeView {
	case ViewAgents:
		if m.listCursor < len(fd.Agents) {
			m.selectedAgent = fd.Agents[m.listCursor].Name
			m.activeView = ViewAgentDetail
		}
	case ViewDeploys:
		groups := groupDeployments(fd.Deployments)
		if m.listCursor < len(groups) {
			m.selectedDeploy = groups[m.listCursor].Latest.ID
			m.activeView = ViewDeploymentDetail
		}
	}
	return m, nil
}

// clampCursor ensures cursor doesn't exceed list bounds.
func (m Model) clampCursor(cursor int) int { //nolint:gocritic // bubbletea value-receiver pattern
	maxIdx := m.maxListIndex()
	if cursor > maxIdx {
		return maxIdx
	}
	return cursor
}

// maxListIndex returns the maximum valid cursor index for the current list view.
func (m Model) maxListIndex() int { //nolint:gocritic // bubbletea value-receiver pattern
	fd := filteredData(m.data, m.activeView, m.filterText)
	if fd == nil {
		return 0
	}
	switch m.activeView {
	case ViewAgents:
		return max(len(fd.Agents)-1, 0)
	case ViewDeploys:
		groups := groupDeployments(fd.Deployments)
		return max(len(groups)-1, 0)
	case ViewContainers:
		return max(len(fd.Containers)-1, 0)
	case ViewEvents:
		return max(len(fd.Events)-1, 0)
	}
	return 0
}

// isListView returns true if the current view is a scrollable list.
func (m Model) isListView() bool { //nolint:gocritic // bubbletea value-receiver pattern
	switch m.activeView {
	case ViewAgents, ViewDeploys, ViewContainers, ViewEvents:
		return true
	}
	return false
}

// adjustListScroll ensures the cursor is visible within the scrollable viewport.
func (m *Model) adjustListScroll() {
	if !m.isListView() {
		return
	}
	// Viewport = available data rows inside the box
	// height - header(1) - footer(1) - spacing(2) - box_top(1) - table_header(1) - box_bottom(1) = height - 7
	viewport := m.height - 7
	if viewport <= 0 {
		return
	}

	cursorLine := m.cursorContentLine()
	if cursorLine < m.listOffset {
		m.listOffset = cursorLine
	}
	if cursorLine >= m.listOffset+viewport {
		m.listOffset = cursorLine - viewport + 1
	}
}

// cursorContentLine returns the line index of the cursor within the
// scrollable data rows of the rendered content. For views where items
// can span multiple lines (e.g. deployments with summary rows), this
// counts actual rendered lines up to the cursor position.
func (m Model) cursorContentLine() int { //nolint:gocritic // bubbletea value-receiver pattern
	fd := filteredData(m.data, m.activeView, m.filterText)
	if fd == nil {
		return 0
	}
	switch m.activeView {
	case ViewAgents:
		return m.listCursor
	case ViewContainers, ViewEvents:
		return m.listCursor
	case ViewDeploys:
		groups := groupDeployments(fd.Deployments)
		line := 0
		for i := 0; i < m.listCursor && i < len(groups); i++ {
			line++ // data row
			if len(groups[i].Older) > 0 {
				line++ // summary row
			}
		}
		return line
	}
	return 0
}

// applyListScroll scrolls the content of a list view box, keeping the
// box top border and table header sticky at the top, and the box bottom
// border at the bottom. Only the data rows in between are scrolled.
func applyListScroll(content string, scrollOffset, availHeight int) string {
	lines := splitLines(content)
	if len(lines) <= availHeight {
		return content
	}

	// Sticky regions: first 2 lines (box top + header), last 1 line (box bottom)
	const stickyTop = 2
	const stickyBottom = 1

	if len(lines) < stickyTop+stickyBottom+1 {
		return content
	}

	scrollRegion := lines[stickyTop : len(lines)-stickyBottom]
	viewportHeight := availHeight - stickyTop - stickyBottom

	if viewportHeight <= 0 || len(scrollRegion) <= viewportHeight {
		return content
	}

	offset := min(scrollOffset, len(scrollRegion)-viewportHeight)
	offset = max(offset, 0)

	visible := scrollRegion[offset : offset+viewportHeight]

	var result []string
	result = append(result, lines[:stickyTop]...)
	result = append(result, visible...)
	result = append(result, lines[len(lines)-stickyBottom:]...)
	return joinLines(result)
}

// View renders the dashboard.
func (m Model) View() string { //nolint:gocritic // bubbletea requires value receiver
	if m.width == 0 {
		return "Initializing..."
	}

	header := renderHeader(m.width, m.refreshInterval)

	// Show export status below header (clears on next data refresh)
	if m.exportStatus != "" {
		statusLine := lipgloss.NewStyle().Foreground(colorGreen).Render("  ✓ Exported to " + m.exportStatus)
		header = header + "\n" + statusLine
	}

	// Compute filtered data for list views
	fd := m.data
	if m.filterText != "" && m.data != nil {
		fd = filteredData(m.data, m.activeView, m.filterText)
	}

	var content string
	switch m.activeView {
	case ViewOverview:
		content = renderOverview(m.data, m.width)
	case ViewAgents:
		content = renderAgentList(fd, m.width, m.listCursor)
	case ViewDeploys:
		content = renderDeploymentList(fd, m.width, m.listCursor)
	case ViewContainers:
		content = renderContainerList(fd, m.width, m.listCursor)
	case ViewEngine:
		content = renderEngineDetail(m.data, m.width)
	case ViewEvents:
		content = renderEventList(fd, m.width, m.listCursor)
	case ViewAgentDetail:
		content = renderAgentDetail(m.data, m.selectedAgent, m.width)
	case ViewDeploymentDetail:
		content = renderDeploymentDetail(m.data, m.selectedDeploy, m.width)
	}

	if m.err != nil && m.data == nil {
		content = renderErrorView(m.err, m.width)
	} else if m.err != nil {
		// Show error in header area but keep stale data visible
		content = styleError.Render(fmt.Sprintf("  ⚠ %v", m.err)) + "\n\n" + content
	}

	footer := renderFooter(m.width, m.activeView)

	// Filter bar (shown when editing or when a filter is applied)
	var filterBar string
	if m.filterEditing || m.filterText != "" {
		filterBar = renderFilterBar(m.filterText, m.filterEditing, m.width)
	}

	// Calculate available height for content
	headerHeight := lipgloss.Height(header)
	footerHeight := lipgloss.Height(footer)
	availHeight := m.height - headerHeight - footerHeight - 2 // 2 for spacing
	if filterBar != "" {
		availHeight--
	}

	// Scroll or truncate content to fit available height
	contentLines := lipgloss.Height(content)
	if contentLines > availHeight && availHeight > 0 {
		if m.isListView() {
			content = applyListScroll(content, m.listOffset, availHeight)
		} else {
			lines := splitLines(content)
			if len(lines) > availHeight {
				lines = lines[:availHeight]
			}
			content = joinLines(lines)
		}
	}

	// Build final view layout
	var sections []string
	sections = append(sections, header, "")
	if filterBar != "" {
		sections = append(sections, filterBar)
	}
	sections = append(sections, content, "", footer)
	view := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Composite overlays on top of the rendered dashboard
	if m.helpOpen {
		boxWidth := max(min(50, m.width-4), 30)
		return applyOverlay(view, renderHelpBox(boxWidth), m.width, m.height)
	}
	if m.paletteOpen {
		boxWidth := max(min(56, m.width-4), 30)
		return applyOverlay(view, renderPaletteBox(m.paletteFilter, m.paletteCursor, boxWidth, m.activeView), m.width, m.height)
	}

	return view
}

// renderHeader renders the dashboard title bar.
func renderHeader(width int, interval time.Duration) string {
	title := styleTitle.Render("Banyan Dashboard")
	refresh := styleDim.Render(fmt.Sprintf("↻ %s", interval))

	gap := max(width-lipgloss.Width(title)-lipgloss.Width(refresh), 1)

	return title + fmt.Sprintf("%*s", gap, "") + refresh
}

// renderFooter renders the bottom navigation bar.
func renderFooter(width int, active View) string {
	var left string

	switch active {
	case ViewOverview:
		tabs := []struct {
			key  string
			name string
			view View
		}{
			{"1", "Overview", ViewOverview},
			{"2", "Agents", ViewAgents},
			{"3", "Deploys", ViewDeploys},
			{"4", "Containers", ViewContainers},
			{"6", "Events", ViewEvents},
		}

		var parts []string
		for _, t := range tabs {
			label := fmt.Sprintf("%s %s", t.key, t.name)
			if t.view == active {
				parts = append(parts, styleSelected.Render(label))
			} else {
				parts = append(parts, styleDim.Render(label))
			}
		}
		left = " " + strings.Join(parts, "  ")

	case ViewAgents, ViewDeploys:
		left = " " + styleDim.Render("↑↓ Navigate  ↵ Detail  Esc Back")

	case ViewContainers, ViewEvents:
		left = " " + styleDim.Render("↑↓ Navigate  Esc Back")

	case ViewAgentDetail, ViewDeploymentDetail, ViewEngine:
		left = " " + styleDim.Render("Esc Back")
	}

	right := styleDim.Render("p Palette  ? Help  r Refresh  q Quit")

	gap := max(width-lipgloss.Width(left)-lipgloss.Width(right), 1)

	return left + fmt.Sprintf("%*s", gap, "") + right
}

// renderPlaceholder renders a stub screen for unimplemented views.
func renderPlaceholder(title, message string) string {
	return renderBox(title, styleDim.Render(message), 50)
}

// renderErrorView renders a full-screen error message.
func renderErrorView(err error, width int) string {
	content := styleError.Render("Engine unreachable") + "\n" +
		styleDim.Render(err.Error()) + "\n\n" +
		styleDim.Render("Retrying automatically...")
	return renderBox("Error", content, min(width, 60))
}

// fetchDataCmd returns a tea.Cmd that fetches dashboard data from the engine.
func fetchDataCmd(client banyanpb.EngineServiceClient) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := client.GetDashboardData(ctx, &banyanpb.GetDashboardDataRequest{})
		if err != nil {
			return errMsg{err: err}
		}
		return dataMsg{data: ConvertFromProto(resp)}
	}
}

// tickAfter returns a tea.Cmd that sends a tickMsg after the given duration.
func tickAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	return strings.Split(s, "\n")
}

// joinLines joins lines back into a single string.
func joinLines(lines []string) string {
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	return result
}
