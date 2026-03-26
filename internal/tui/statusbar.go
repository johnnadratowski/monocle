package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

type statusBarModel struct {
	agentName       string
	baseRef         string
	fileCount       int
	commentCount    int
	feedbackStatus  string
	subscriberCount int
	socketStarted   bool
	commandMode     bool
	commandBuffer   string
	contextHints    string // override hints when set (e.g. comment-specific keybinds)
	diffStyle       diffStyle
	width           int
	theme           Theme
}

func newStatusBarModel(theme Theme) statusBarModel {
	return statusBarModel{
		theme: theme,
	}
}

func (m statusBarModel) View() string {
	if m.width == 0 {
		return ""
	}

	if m.commandMode {
		cmdLine := fmt.Sprintf(":%s█", m.commandBuffer)
		return m.theme.StatusBar.Width(m.width).Render(cmdLine)
	}

	// Connection status with agent name
	var connLabel string
	name := m.agentName
	switch {
	case m.subscriberCount > 0:
		if name != "" {
			connLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("● Connected " + name)
		} else {
			connLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("● Connected")
		}
	case m.socketStarted:
		connLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("○ Waiting")
	default:
		connLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("● Disconnected")
	}

	// Info sections
	parts := []string{connLabel}

	if m.baseRef != "" && m.baseRef != "WORKING" {
		ref := m.baseRef
		if len(ref) > 8 {
			ref = ref[:8]
		}
		parts = append(parts, fmt.Sprintf("ref:%s", ref))
	}

	if m.diffStyle == diffStyleFile {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true).Render("[FILE]"))
	}

	parts = append(parts, fmt.Sprintf("%d files", m.fileCount))
	parts = append(parts, fmt.Sprintf("%d comments", m.commentCount))

	if m.feedbackStatus != "" && m.feedbackStatus != "none" {
		fbStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
		parts = append(parts, fbStyle.Render(m.feedbackStatus))
	}

	// Key hints (right-aligned, collapse to ?:help when narrow)
	var fullHints string
	if m.contextHints != "" {
		fullHints = m.contextHints
	} else {
		fullHints = "c:comment  S:submit  P:pause  D:dismiss  q:quit"
	}
	shortHints := "?:help"
	left := strings.Join(parts, "  ")

	leftW := lipgloss.Width(left)
	hints := fullHints
	if leftW+len(fullHints)+2 > m.width {
		hints = shortHints
	}

	gap := m.width - leftW - len(hints) - 2
	if gap < 1 {
		gap = 1
	}

	styledHints := lipgloss.NewStyle().Faint(true).Render(hints)
	bar := left + strings.Repeat(" ", gap) + styledHints
	return m.theme.StatusBar.Render(bar)
}
