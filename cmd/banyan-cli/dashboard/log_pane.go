package dashboard

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
)

// logPaneState holds the state for the log streaming pane.
type logPaneState struct {
	err           error
	containerName string
	lines         []string
	streaming     bool
}

// logLineMsg carries a single log line from the stream.
type logLineMsg struct {
	line string
}

// logStreamEndMsg signals the log stream has ended.
type logStreamEndMsg struct {
	err error
}

// startLogStream returns a tea.Cmd that streams logs via GetLogs RPC.
func startLogStream(client banyanpb.EngineServiceClient, containerName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		_ = cancel // cancel is available if needed by the caller

		stream, err := client.GetLogs(ctx, &banyanpb.GetLogsRequest{
			ContainerName: containerName,
			Follow:        true,
			Tail:          50,
		})
		if err != nil {
			cancel()
			return logStreamEndMsg{err: err}
		}

		resp, err := stream.Recv()
		if err != nil {
			cancel()
			return logStreamEndMsg{err: err}
		}

		cancel()
		return logLineMsg{line: string(resp.Data)}
	}
}

// renderLogPane renders the log pane at the bottom of the screen.
func renderLogPane(state logPaneState, width, height int) string {
	stTitle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	stBorder := lipgloss.NewStyle().Foreground(colorBorder)
	stHint := lipgloss.NewStyle().Foreground(colorDim)

	innerWidth := width - 4

	// Top border
	title := stTitle.Render("Logs: " + state.containerName)
	if state.streaming {
		title += " " + lipgloss.NewStyle().Foreground(colorGreen).Render("●")
	}
	titleWidth := lipgloss.Width(title)
	topFill := max(width-5-titleWidth, 1)
	top := stBorder.Render("╭─ ") + title + stBorder.Render(" "+strings.Repeat("─", topFill)+"╮")

	line := func(content string) string {
		lineWidth := lipgloss.Width(content)
		pad := max(innerWidth-lineWidth, 0)
		return stBorder.Render("│ ") + content + strings.Repeat(" ", pad) + stBorder.Render(" │")
	}

	// Reserve lines for border + hint
	contentHeight := height - 3 // top + hint + bottom
	if contentHeight < 1 {
		contentHeight = 1
	}

	var rows []string
	rows = append(rows, top)

	switch {
	case state.err != nil:
		rows = append(rows, line(styleError.Render(fmt.Sprintf("Error: %v", state.err))))
	case len(state.lines) == 0:
		rows = append(rows, line(stHint.Render("Waiting for logs...")))
	default:
		// Show last N lines that fit
		start := 0
		if len(state.lines) > contentHeight {
			start = len(state.lines) - contentHeight
		}
		for _, l := range state.lines[start:] {
			rows = append(rows, line(truncate(l, innerWidth)))
		}
	}

	// Pad to fill height
	for len(rows) < height-1 {
		rows = append(rows, line(""))
	}

	rows = append(rows,
		stBorder.Render("╰"+strings.Repeat("─", width-2)+"╯"),
	)

	return strings.Join(rows, "\n")
}
