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
	ViewOverview View = iota
	ViewAgents
	ViewDeploys
	ViewContainers
)

// Model is the main bubbletea model for the dashboard.
type Model struct {
	client          banyanpb.EngineServiceClient
	data            *DashboardData
	err             error
	refreshInterval time.Duration
	width           int
	height          int
	activeView      View
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
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchDataCmd(m.client),
		tickAfter(m.refreshInterval),
	)
}

// Update handles messages and returns updated model + commands.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "1":
			m.activeView = ViewOverview
		case "2":
			m.activeView = ViewAgents
		case "3":
			m.activeView = ViewDeploys
		case "4":
			m.activeView = ViewContainers
		case "r":
			return m, fetchDataCmd(m.client)
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(
			fetchDataCmd(m.client),
			tickAfter(m.refreshInterval),
		)

	case dataMsg:
		m.data = &msg.data
		m.err = nil
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil
	}

	return m, nil
}

// View renders the dashboard.
func (m Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	header := renderHeader(m.width, m.refreshInterval)

	var content string
	switch m.activeView {
	case ViewOverview:
		content = renderOverview(m.data, m.width)
	case ViewAgents:
		content = renderPlaceholder("Agents", "Agent detail view — coming soon (Phase 4)")
	case ViewDeploys:
		content = renderPlaceholder("Deployments", "Deployment detail view — coming soon (Phase 4)")
	case ViewContainers:
		content = renderPlaceholder("Containers", "Container list view — coming soon (Phase 4)")
	}

	if m.err != nil && m.data == nil {
		content = renderErrorView(m.err, m.width)
	} else if m.err != nil {
		// Show error in header area but keep stale data visible
		content = styleError.Render(fmt.Sprintf("  ⚠ %v", m.err)) + "\n\n" + content
	}

	footer := renderFooter(m.width, m.activeView)

	// Calculate available height for content
	headerHeight := lipgloss.Height(header)
	footerHeight := lipgloss.Height(footer)
	availHeight := m.height - headerHeight - footerHeight - 2 // 2 for spacing

	// Truncate content if needed
	contentLines := lipgloss.Height(content)
	if contentLines > availHeight && availHeight > 0 {
		lines := splitLines(content)
		if len(lines) > availHeight {
			lines = lines[:availHeight]
		}
		content = joinLines(lines)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, "", content, "", footer)
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
	tabs := []struct {
		key  string
		name string
		view View
	}{
		{"1", "Overview", ViewOverview},
		{"2", "Agents", ViewAgents},
		{"3", "Deploys", ViewDeploys},
		{"4", "Containers", ViewContainers},
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

	nav := " " + lipgloss.JoinHorizontal(lipgloss.Center, parts[0], "  ", parts[1], "  ", parts[2], "  ", parts[3])
	quit := styleDim.Render("r Refresh  q Quit")

	gap := max(width-lipgloss.Width(nav)-lipgloss.Width(quit), 1)

	return nav + fmt.Sprintf("%*s", gap, "") + quit
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
