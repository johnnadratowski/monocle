package tui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

type reviewSummaryModel struct {
	active         bool
	summary        *types.ReviewSummary
	agentConnected bool
	action         types.SubmitAction
	body            string
	copyToClipboard bool
	width           int
	height          int
	theme           Theme
}

func newReviewSummaryModel(theme Theme) reviewSummaryModel {
	return reviewSummaryModel{theme: theme}
}

type confirmSubmitMsg struct {
	action          types.SubmitAction
	body            string
	copyToClipboard bool
}
type cancelSubmitMsg struct{}

type yankReviewMsg struct {
	action types.SubmitAction
	body   string
}
type yankSuccessMsg struct{}
type yankFailMsg struct {
	err string
}

func (m *reviewSummaryModel) open(summary *types.ReviewSummary, agentConnected bool) {
	m.active = true
	m.summary = summary
	m.agentConnected = agentConnected
	m.body = ""
	m.copyToClipboard = false

	// Default action: request_changes if issues or suggestions, approve otherwise
	hasActionable := summary != nil && (summary.IssueCt+summary.SuggestionCt > 0)
	if hasActionable {
		m.action = types.ActionRequestChanges
	} else {
		m.action = types.ActionApprove
	}
}

func (m reviewSummaryModel) Init() tea.Cmd {
	return nil
}

// canSubmit returns true if the current state is valid for submission.
// Request changes requires at least one unresolved comment or body text.
func (m reviewSummaryModel) canSubmit() bool {
	if m.action != types.ActionRequestChanges {
		return true
	}
	if strings.TrimSpace(m.body) != "" {
		return true
	}
	if m.summary != nil && (m.summary.IssueCt+m.summary.SuggestionCt+m.summary.NoteCt+m.summary.PraiseCt > 0) {
		return true
	}
	return false
}

func (m reviewSummaryModel) Update(msg tea.Msg) (reviewSummaryModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			if !m.canSubmit() {
				return m, nil
			}
			m.active = false
			submitMsg := confirmSubmitMsg{
				action:          m.action,
				body:            m.body,
				copyToClipboard: m.copyToClipboard,
			}
			return m, func() tea.Msg { return submitMsg }
		case "esc":
			m.active = false
			return m, func() tea.Msg { return cancelSubmitMsg{} }
		case "tab":
			// Cycle review action
			if m.action == types.ActionApprove {
				m.action = types.ActionRequestChanges
			} else {
				m.action = types.ActionApprove
			}
		case "ctrl+y":
			if !m.canSubmit() {
				return m, nil
			}
			m.active = false
			yank := yankReviewMsg{
				action: m.action,
				body:   m.body,
			}
			return m, func() tea.Msg { return yank }
		case "shift+tab":
			m.copyToClipboard = !m.copyToClipboard
		case "ctrl+g":
			return m, func() tea.Msg {
				return externalEditorRequestMsg{body: m.body, origin: overlayReview}
			}
		case "shift+enter", "alt+enter":
			m.body += "\n"
		case "backspace":
			if len(m.body) > 0 {
				m.body = m.body[:len(m.body)-1]
			}
		case "ctrl+w", "alt+backspace":
			m.deleteLastWord()
		case "space":
			m.body += " "
		default:
			key := msg.String()
			if len(key) == 1 {
				m.body += key
			}
		}
	}
	return m, nil
}

// deleteLastWord deletes the word at the end of the body (Ctrl+W / Alt+Backspace).
func (m *reviewSummaryModel) deleteLastWord() {
	if len(m.body) == 0 {
		return
	}
	runes := []rune(m.body)
	end := len(runes)
	// Skip trailing whitespace
	for end > 0 && (runes[end-1] == ' ' || runes[end-1] == '\t') {
		end--
	}
	// Delete back to start of word
	for end > 0 && runes[end-1] != ' ' && runes[end-1] != '\t' && runes[end-1] != '\n' {
		end--
	}
	m.body = string(runes[:end])
}

// handleClick processes a mouse click at content-relative coordinates.
// Returns true if the click was on an interactive element.
func (m *reviewSummaryModel) handleClick(contentX, contentY int) bool {
	// Action labels are on line 2: title(0), blank(1), labels(2)
	if contentY == 2 {
		labels := []struct {
			a     types.SubmitAction
			label string
		}{
			{types.ActionApprove, "APPROVE"},
			{types.ActionRequestChanges, "REQUEST CHANGES"},
		}
		x := 0
		for _, l := range labels {
			labelW := len(l.label) + 2 // padding(0,1)
			if contentX >= x && contentX < x+labelW {
				m.action = l.a
				return true
			}
			x += labelW + 1
		}
		return false
	}

	// Checkbox: find the line with "[x]" or "[ ]" by computing its position.
	// The checkbox line is variable due to comment counts. We compute it by
	// counting the rendered content lines that precede it.
	checkboxLine := m.checkboxLine()
	if contentY == checkboxLine && contentX >= 0 && contentX <= 2 {
		m.copyToClipboard = !m.copyToClipboard
		return true
	}

	return false
}

// checkboxLine computes the content line number where the clipboard checkbox renders.
func (m reviewSummaryModel) checkboxLine() int {
	// Line 0: "Submit Review"
	// Line 1: blank
	// Line 2: action labels
	// Line 3: blank
	line := 4

	// Variable-height comment summary section
	hasComments := m.summary != nil && (m.summary.IssueCt+m.summary.SuggestionCt+m.summary.NoteCt+m.summary.PraiseCt > 0)
	if hasComments {
		if m.summary.IssueCt > 0 {
			line++
		}
		if m.summary.SuggestionCt > 0 {
			line++
		}
		if m.summary.NoteCt > 0 {
			line++
		}
		if m.summary.PraiseCt > 0 {
			line++
		}
		line++ // blank after counts

		if len(m.summary.FileComments) > 0 {
			line++ // "Files:" header
			line += len(m.summary.FileComments)
			line++ // blank
		}
		if len(m.summary.ContentComments) > 0 {
			line++ // "Content Items:" header
			line += len(m.summary.ContentComments)
			line++ // blank
		}
	}

	// "Comment (optional):" header
	line++
	// body text line
	line++
	// blank
	line++
	// checkbox line
	return line
}

func (m reviewSummaryModel) View() string {
	if !m.active {
		return ""
	}

	modalWidth := CalcModalWidth(m.width, 0)

	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Submit Review"))
	b.WriteString("\n\n")

	// Action selector (Tab to cycle)
	actionLabels := []struct {
		a     types.SubmitAction
		label string
		color color.Color
	}{
		{types.ActionApprove, "APPROVE", lipgloss.Color("2")},
		{types.ActionRequestChanges, "REQUEST CHANGES", lipgloss.Color("1")},
	}
	for i, al := range actionLabels {
		var style lipgloss.Style
		if al.a == m.action {
			style = lipgloss.NewStyle().
				Background(al.color).
				Foreground(lipgloss.Color("0")).
				Bold(true).
				Padding(0, 1)
		} else {
			style = lipgloss.NewStyle().
				Foreground(al.color).
				Padding(0, 1)
		}
		b.WriteString(style.Render(al.label))
		if i < len(actionLabels)-1 {
			b.WriteString(" ")
		}
	}
	b.WriteString("  ")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("(Tab)"))
	b.WriteString("\n\n")

	// Comment counts (if any inline comments)
	hasComments := m.summary != nil && (m.summary.IssueCt+m.summary.SuggestionCt+m.summary.NoteCt+m.summary.PraiseCt > 0)
	if hasComments {
		if m.summary.IssueCt > 0 {
			b.WriteString(fmt.Sprintf("  Issues:      %d\n", m.summary.IssueCt))
		}
		if m.summary.SuggestionCt > 0 {
			b.WriteString(fmt.Sprintf("  Suggestions: %d\n", m.summary.SuggestionCt))
		}
		if m.summary.NoteCt > 0 {
			b.WriteString(fmt.Sprintf("  Notes:       %d\n", m.summary.NoteCt))
		}
		if m.summary.PraiseCt > 0 {
			b.WriteString(fmt.Sprintf("  Praise:      %d\n", m.summary.PraiseCt))
		}
		b.WriteString("\n")

		// Comments by file
		if len(m.summary.FileComments) > 0 {
			b.WriteString(lipgloss.NewStyle().Bold(true).Render("Files:"))
			b.WriteString("\n")
			for path, cmts := range m.summary.FileComments {
				b.WriteString(fmt.Sprintf("  %s (%d comments)\n", path, len(cmts)))
			}
			b.WriteString("\n")
		}

		// Comments on content items
		if len(m.summary.ContentComments) > 0 {
			b.WriteString(lipgloss.NewStyle().Bold(true).Render("Content Items:"))
			b.WriteString("\n")
			for id, cmts := range m.summary.ContentComments {
				b.WriteString(fmt.Sprintf("  %s (%d comments)\n", id, len(cmts)))
			}
			b.WriteString("\n")
		}
	}

	// General review comment input
	commentLabel := "Comment (optional):"
	if !m.canSubmit() {
		commentLabel = "Comment (required):"
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(commentLabel))
	b.WriteString("\n")
	bodyDisplay := m.body + "█"
	b.WriteString(bodyDisplay)
	b.WriteString("\n\n")

	// Copy to clipboard checkbox
	check := " "
	if m.copyToClipboard {
		check = "x"
	}
	b.WriteString(fmt.Sprintf("[%s] Copy to clipboard  ", check))
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("(Shift+Tab)"))
	b.WriteString("\n\n")

	// Delivery status
	if m.agentConnected {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("Review will be sent immediately"))
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("Review will be queued for delivery"))
	}
	b.WriteString("\n\n")

	if m.canSubmit() {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("Enter: submit  Ctrl+g: editor  Ctrl+y: yank  Esc: cancel"))
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("Add a comment or body text to request changes"))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("Ctrl+g: editor  Ctrl+y: yank  Esc: cancel"))
	}

	return m.theme.ModalBorder.Width(modalWidth).Render(b.String())
}
