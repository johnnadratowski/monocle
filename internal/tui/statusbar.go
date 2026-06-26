package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

type statusBarModel struct {
	agentName        string
	baseRef          string
	fileCount        int
	reviewedCount    int
	annotationCount  int
	commentCount     int
	feedbackStatus   string
	subscriberCount  int
	connectionMode   string // "queue" for queue-mode connections
	socketStarted    bool
	commandMode      bool
	commandBuffer    string
	searchMode       bool
	searchBuffer     string
	searchBackward   bool
	searchInfo       string // transient "match i/N" indicator after a search
	contextHints     string // override hints when set (e.g. comment-specific keybinds)
	diffStyle        diffStyle
	contentMode      bool   // true when viewing content (plan/doc) in raw mode
	contentID        string // non-empty when viewing a content item (raw or diff)
	diffBaseVersion  int    // base version being diffed from (0 = default)
	diffToVersion    int    // target version being diffed to
	waitingForReview bool
	width            int
	theme            Theme
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

	if m.searchMode {
		prefix := "/"
		if m.searchBackward {
			prefix = "?"
		}
		searchLine := fmt.Sprintf("%s%s█", prefix, m.searchBuffer)
		return m.theme.StatusBar.Width(m.width).Render(searchLine)
	}

	// Connection status with agent name
	var connLabel string
	name := m.agentName
	switch {
	case m.waitingForReview:
		waitStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
		icon := "●"
		if m.subscriberCount == 0 && m.connectionMode != "queue" {
			icon = "○"
		}
		connLabel = waitStyle.Render(icon + " Waiting for Review")
	case m.subscriberCount > 0 || m.connectionMode == "queue":
		connLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("● Connected")
		if name != "" {
			connLabel += " " + name
		}
	case m.socketStarted:
		connLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("○ Waiting")
	default:
		connLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("● Disconnected")
	}

	// Info sections. The base ref and file count moved to the top bar, so the
	// status bar focuses on review progress.
	parts := []string{connLabel}

	if m.contentID != "" && !m.contentMode {
		// Viewing a content item diff
		diffLabel := "DIFF"
		if m.diffStyle == diffStyleSplit {
			diffLabel = "SPLIT"
		}
		if m.diffBaseVersion > 0 {
			diffLabel = fmt.Sprintf("v%d\u2192v%d %s", m.diffBaseVersion, m.diffToVersion, diffLabel)
		}
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true).Render("["+diffLabel+"]"))
	} else if m.diffStyle == diffStyleFile {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true).Render("[FILE]"))
	}

	if m.fileCount > 0 {
		reviewedStyle := lipgloss.NewStyle()
		if m.reviewedCount >= m.fileCount {
			reviewedStyle = reviewedStyle.Foreground(lipgloss.Color("2")) // all reviewed
		}
		parts = append(parts, reviewedStyle.Render(fmt.Sprintf("%d/%d reviewed", m.reviewedCount, m.fileCount)))
	}
	parts = append(parts, fmt.Sprintf("%d comments", m.commentCount))
	parts = append(parts, fmt.Sprintf("%d annotations", m.annotationCount))

	if m.searchInfo != "" {
		searchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
		parts = append(parts, searchStyle.Render(m.searchInfo))
	}

	if m.feedbackStatus != "" && m.feedbackStatus != "none" {
		fbStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
		parts = append(parts, fbStyle.Render(m.feedbackStatus))
	}

	// Key hints (right-aligned, collapse to H:help when narrow)
	var fullHints string
	if m.contextHints != "" {
		fullHints = m.contextHints
	} else {
		fullHints = "c:comment  /:search  S:submit  D:dismiss  H:help"
	}
	shortHints := "H:help"
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
