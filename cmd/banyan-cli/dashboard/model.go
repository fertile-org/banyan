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
type Model struct { //nolint:govet // bubbletea model readability over fieldalignment
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

	// Action menu state
	actionMenuOpen   bool
	actionMenuCursor int
	actionMenuTarget string   // container or deployment name being acted on
	actionMenuTags   []string // deployment tags (needed for Down RPC)
	actionMenuItems  []actionMenuItem

	// Confirmation dialog state
	confirmState  *confirmState
	confirmAction string // "kill", "restart", "teardown"

	// Log pane state
	logPane *logPaneState

	// Action status message (auto-clears after 5 seconds)
	actionStatus     string
	actionStatusTime time.Time

	// CPU history per container for sparklines (container name → last N values)
	cpuHistory map[string][]float64
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
		if m.confirmState != nil {
			return m.handleConfirmKey(msg)
		}
		if m.actionMenuOpen {
			return m.handleActionMenuKey(msg)
		}
		if m.paletteOpen {
			return m.handlePaletteKey(msg)
		}
		if m.filterEditing {
			return m.handleFilterKey(msg)
		}
		return m.handleKey(msg)

	case tickMsg:
		// Clear stale action status (older than 5 seconds)
		if m.actionStatus != "" && time.Since(m.actionStatusTime) > 5*time.Second {
			m.actionStatus = ""
		}
		return m, tea.Batch(
			fetchDataCmd(m.client),
			tickAfter(m.refreshInterval),
		)

	case dataMsg:
		m.data = &msg.data
		m.err = nil
		m.exportStatus = ""
		// Accumulate CPU history for sparklines
		m.updateCPUHistory()
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

	case actionResultMsg:
		if msg.success {
			m.actionStatus = lipgloss.NewStyle().Foreground(colorGreen).Render("✓ " + msg.message)
		} else {
			m.actionStatus = lipgloss.NewStyle().Foreground(colorRed).Render("✗ " + msg.message)
		}
		m.actionStatusTime = time.Now()
		return m, fetchDataCmd(m.client)

	case logLineMsg:
		if m.logPane != nil {
			// Split incoming data into lines and append
			incoming := strings.Split(strings.TrimRight(msg.line, "\n"), "\n")
			m.logPane.lines = append(m.logPane.lines, incoming...)
			// Keep last 200 lines
			if len(m.logPane.lines) > 200 {
				m.logPane.lines = m.logPane.lines[len(m.logPane.lines)-200:]
			}
			return m, startLogStream(m.client, m.logPane.containerName)
		}
		return m, nil

	case logStreamEndMsg:
		if m.logPane != nil {
			m.logPane.streaming = false
			if msg.err != nil {
				m.logPane.err = msg.err
			}
			m.logPane.lines = append(m.logPane.lines, "[stream ended]")
		}
		return m, nil
	}

	return m, nil
}

// handleKey processes key events in normal (non-palette, non-filter) mode.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	switch msg.String() {
	case "q", "ctrl+c":
		if m.logPane != nil {
			m.logPane = nil
			return m, nil
		}
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
		// Close log pane first
		if m.logPane != nil {
			m.logPane = nil
			return m, nil
		}
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
	case "a":
		return m.openActionMenu()
	case "l":
		if m.activeView == ViewContainers {
			return m.openLogPane()
		}
	case "d":
		if m.activeView == ViewDeploys {
			return m.startTeardownConfirm()
		}
	case "+":
		if m.activeView == ViewDeploymentDetail {
			return m.scaleUp()
		}
	case "-":
		if m.activeView == ViewDeploymentDetail {
			return m.scaleDown()
		}
	}
	return m, nil
}

// handleActionMenuKey processes key events when the action menu is open.
func (m Model) handleActionMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	switch msg.String() {
	case "esc":
		m.actionMenuOpen = false
		return m, nil
	case "up", "k":
		m.actionMenuCursor = max(m.actionMenuCursor-1, 0)
		return m, nil
	case "down", "j":
		m.actionMenuCursor = min(m.actionMenuCursor+1, max(len(m.actionMenuItems)-1, 0))
		return m, nil
	case "enter":
		if m.actionMenuCursor >= len(m.actionMenuItems) {
			return m, nil
		}
		item := m.actionMenuItems[m.actionMenuCursor]
		m.actionMenuOpen = false
		return m.executeActionMenuItem(item)
	}
	return m, nil
}

// handleConfirmKey processes key events in the confirmation dialog.
func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	if m.confirmState == nil {
		return m, nil
	}

	if m.confirmState.typeName != "" {
		// Type-to-confirm mode
		switch msg.String() {
		case "esc":
			m.confirmState = nil
			m.confirmAction = ""
			return m, nil
		case "enter":
			if m.confirmState.input == m.confirmState.typeName {
				return m.executeConfirmedAction()
			}
			return m, nil
		case "backspace":
			if m.confirmState.input != "" {
				m.confirmState.input = m.confirmState.input[:len(m.confirmState.input)-1]
			}
			return m, nil
		default:
			if msg.Type == tea.KeyRunes {
				m.confirmState.input += string(msg.Runes)
			}
			return m, nil
		}
	}

	// Simple y/n mode
	switch msg.String() {
	case "y":
		return m.executeConfirmedAction()
	case "n", "esc":
		m.confirmState = nil
		m.confirmAction = ""
		return m, nil
	}
	return m, nil
}

// openActionMenu opens the action menu for the current list view context.
func (m Model) openActionMenu() (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	switch m.activeView {
	case ViewContainers:
		fd := filteredData(m.data, m.activeView, m.filterText)
		if fd == nil || m.listCursor >= len(fd.Containers) {
			return m, nil
		}
		c := fd.Containers[m.listCursor]
		m.actionMenuOpen = true
		m.actionMenuCursor = 0
		m.actionMenuTarget = c.Name
		m.actionMenuItems = containerActions()
	case ViewDeploys:
		fd := filteredData(m.data, m.activeView, m.filterText)
		if fd == nil {
			return m, nil
		}
		groups := groupDeployments(fd.Deployments)
		if m.listCursor >= len(groups) {
			return m, nil
		}
		m.actionMenuOpen = true
		m.actionMenuCursor = 0
		m.actionMenuTarget = groups[m.listCursor].Latest.Name
		m.actionMenuTags = groups[m.listCursor].Latest.Tags
		m.actionMenuItems = deploymentActions()
	}
	return m, nil
}

// executeActionMenuItem handles selection of an action menu item.
func (m Model) executeActionMenuItem(item actionMenuItem) (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	switch item.key {
	case "cancel":
		return m, nil
	case "kill":
		m.confirmState = &confirmState{
			message: fmt.Sprintf("Kill %s? [y/n]", m.actionMenuTarget),
		}
		m.confirmAction = "kill"
		return m, nil
	case "restart":
		m.confirmState = &confirmState{
			message: fmt.Sprintf("Restart %s? [y/n]", m.actionMenuTarget),
		}
		m.confirmAction = "restart"
		return m, nil
	case "logs":
		return m.openLogPaneForContainer(m.actionMenuTarget)
	case "teardown":
		m.confirmState = &confirmState{
			message:  fmt.Sprintf("Teardown deployment %s?", m.actionMenuTarget),
			typeName: m.actionMenuTarget,
		}
		m.confirmAction = "teardown"
		return m, nil
	case "scale":
		// For scale, find the first service and delegate to detail view
		m.actionStatus = "Use +/- in deployment detail view to scale"
		m.actionStatusTime = time.Now()
		return m, nil
	}
	return m, nil
}

// executeConfirmedAction runs the confirmed action.
func (m Model) executeConfirmedAction() (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	action := m.confirmAction
	target := m.actionMenuTarget
	m.confirmState = nil
	m.confirmAction = ""

	if m.client == nil {
		return m, nil
	}

	switch action {
	case "kill", "restart":
		// Find container data to get taskID and agentName
		c := m.findContainerByName(target)
		if c == nil {
			m.actionStatus = lipgloss.NewStyle().Foreground(colorRed).Render("✗ Container not found")
			m.actionStatusTime = time.Now()
			return m, nil
		}
		return m, stopContainerCmd(m.client, c.TaskID, c.AgentName)
	case "teardown":
		return m, teardownDeploymentCmd(m.client, target, m.actionMenuTags)
	}
	return m, nil
}

// findContainerByName finds a container in the current data by name.
func (m Model) findContainerByName(name string) *ContainerData { //nolint:gocritic // bubbletea value-receiver pattern
	if m.data == nil {
		return nil
	}
	for i := range m.data.Containers {
		if m.data.Containers[i].Name == name {
			return &m.data.Containers[i]
		}
	}
	return nil
}

// killSelectedContainer opens a confirm dialog to kill the selected container.
func (m Model) killSelectedContainer() (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	fd := filteredData(m.data, m.activeView, m.filterText)
	if fd == nil || m.listCursor >= len(fd.Containers) {
		return m, nil
	}
	c := fd.Containers[m.listCursor]
	m.actionMenuTarget = c.Name
	m.confirmState = &confirmState{
		message: fmt.Sprintf("Kill %s? [y/n]", c.Name),
	}
	m.confirmAction = "kill"
	return m, nil
}

// openLogPane opens the log pane for the currently selected container.
func (m Model) openLogPane() (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	fd := filteredData(m.data, m.activeView, m.filterText)
	if fd == nil || m.listCursor >= len(fd.Containers) {
		return m, nil
	}
	c := fd.Containers[m.listCursor]
	return m.openLogPaneForContainer(c.Name)
}

// openLogPaneForContainer opens the log pane for a specific container name.
func (m Model) openLogPaneForContainer(containerName string) (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	m.logPane = &logPaneState{
		containerName: containerName,
		streaming:     true,
	}
	if m.client == nil {
		return m, nil
	}
	return m, startLogStream(m.client, containerName)
}

// startTeardownConfirm opens a type-to-confirm dialog for the selected deployment.
func (m Model) startTeardownConfirm() (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	fd := filteredData(m.data, m.activeView, m.filterText)
	if fd == nil {
		return m, nil
	}
	groups := groupDeployments(fd.Deployments)
	if m.listCursor >= len(groups) {
		return m, nil
	}
	name := groups[m.listCursor].Latest.Name
	m.actionMenuTarget = name
	m.actionMenuTags = groups[m.listCursor].Latest.Tags
	m.confirmState = &confirmState{
		message:  fmt.Sprintf("Teardown deployment %s?", name),
		typeName: name,
	}
	m.confirmAction = "teardown"
	return m, nil
}

// scaleUp sends a scale +1 command for the first service in the deployment.
func (m Model) scaleUp() (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	return m.scaleBy(1)
}

// scaleDown sends a scale -1 command for the first service in the deployment.
func (m Model) scaleDown() (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	return m.scaleBy(-1)
}

// scaleBy adjusts the replica count of the first service in the selected deployment.
func (m Model) scaleBy(delta int32) (tea.Model, tea.Cmd) { //nolint:gocritic // bubbletea value-receiver pattern
	if m.data == nil || m.client == nil {
		return m, nil
	}
	var deploy *DeploymentData
	for i := range m.data.Deployments {
		if m.data.Deployments[i].ID == m.selectedDeploy {
			deploy = &m.data.Deployments[i]
			break
		}
	}
	if deploy == nil || len(deploy.ServiceDetails) == 0 {
		return m, nil
	}
	svc := deploy.ServiceDetails[0]
	newCount := svc.Replicas + delta
	if newCount < 0 {
		newCount = 0
	}
	return m, scaleDeploymentCmd(m.client, deploy.Name, svc.Name, newCount)
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
	case viewActionKill:
		if m.activeView == ViewContainers {
			return m.killSelectedContainer()
		}
		return m, nil
	case viewActionLogs:
		if m.activeView == ViewContainers {
			return m.openLogPane()
		}
		return m, nil
	case viewActionTeardown:
		if m.activeView == ViewDeploys {
			return m.startTeardownConfirm()
		}
		return m, nil
	case viewActionScaleUp:
		if m.activeView == ViewDeploymentDetail {
			return m.scaleUp()
		}
		return m, nil
	case viewActionScaleDown:
		if m.activeView == ViewDeploymentDetail {
			return m.scaleDown()
		}
		return m, nil
	case viewActionActionMenu:
		return m.openActionMenu()
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

	// Show action status below header (auto-clears after 5 seconds)
	if m.actionStatus != "" {
		header = header + "\n  " + m.actionStatus
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
		content = renderContainerList(fd, m.width, m.listCursor, m.cpuHistory)
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

	// Split view for log pane
	if m.logPane != nil {
		logHeight := max(m.height/3, 5)
		pane := renderLogPane(*m.logPane, m.width, logHeight)
		view = lipgloss.JoinVertical(lipgloss.Left, view, pane)
	}

	// Composite overlays on top of the rendered dashboard
	if m.helpOpen {
		boxWidth := max(min(50, m.width-4), 30)
		return applyOverlay(view, renderHelpBox(boxWidth), m.width, m.height)
	}
	if m.paletteOpen {
		boxWidth := max(min(56, m.width-4), 30)
		return applyOverlay(view, renderPaletteBox(m.paletteFilter, m.paletteCursor, boxWidth, m.activeView), m.width, m.height)
	}
	if m.actionMenuOpen {
		boxWidth := max(min(50, m.width-4), 30)
		return applyOverlay(view, renderActionMenu(m.actionMenuTarget, m.actionMenuItems, m.actionMenuCursor, boxWidth), m.width, m.height)
	}
	if m.confirmState != nil {
		boxWidth := max(min(50, m.width-4), 30)
		return applyOverlay(view, renderConfirmDialog(*m.confirmState, boxWidth), m.width, m.height)
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

const maxCPUHistory = 20 // keep last 20 samples per container

// updateCPUHistory appends the current CPU value for each container.
// Maps are reference types so mutation through value receiver works.
func (m *Model) updateCPUHistory() {
	if m.data == nil {
		return
	}
	if m.cpuHistory == nil {
		m.cpuHistory = make(map[string][]float64)
	}

	// Track which containers are still alive
	alive := make(map[string]bool, len(m.data.Containers))
	for i := range m.data.Containers {
		c := &m.data.Containers[i]
		alive[c.Name] = true
		history := m.cpuHistory[c.Name]
		history = append(history, c.CPUPercent)
		if len(history) > maxCPUHistory {
			history = history[len(history)-maxCPUHistory:]
		}
		m.cpuHistory[c.Name] = history
	}

	// Prune containers that no longer exist
	for name := range m.cpuHistory {
		if !alive[name] {
			delete(m.cpuHistory, name)
		}
	}
}

// getCPUHistory returns the CPU history for a container name.
func (m Model) getCPUHistory(name string) []float64 { //nolint:gocritic // bubbletea value-receiver pattern
	if m.cpuHistory == nil {
		return nil
	}
	return m.cpuHistory[name]
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
