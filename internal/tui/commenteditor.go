package tui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/monocle/internal/types"
)

type commentEditorModel struct {
	active      bool
	path        string
	lineStart   int
	lineEnd     int
	targetType  types.TargetType
	commentType types.CommentType
	body        string
	cursor      int // cursor position in runes
	width       int
	height      int
	theme       Theme
	editingID   string // non-empty when editing existing comment
}

func newCommentEditorModel(theme Theme) commentEditorModel {
	return commentEditorModel{
		commentType: types.CommentIssue,
		theme:       theme,
	}
}

type saveCommentMsg struct {
	path        string
	lineStart   int
	lineEnd     int
	targetType  types.TargetType
	commentType types.CommentType
	body        string
	editingID   string
}

type cancelCommentMsg struct{}

func (m commentEditorModel) Init() tea.Cmd {
	return nil
}

func (m commentEditorModel) Update(msg tea.Msg) (commentEditorModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			m.active = false
			return m, func() tea.Msg { return cancelCommentMsg{} }
		case "enter":
			if strings.TrimSpace(m.body) == "" {
				return m, nil
			}
			saveMsg := saveCommentMsg{
				path:        m.path,
				lineStart:   m.lineStart,
				lineEnd:     m.lineEnd,
				targetType:  m.targetType,
				commentType: m.commentType,
				body:        m.body,
				editingID:   m.editingID,
			}
			m.active = false
			return m, func() tea.Msg { return saveMsg }
		case "shift+enter", "alt+enter":
			m.insertAtCursor("\n")
		case "tab":
			// Cycle comment type
			switch m.commentType {
			case types.CommentIssue:
				m.commentType = types.CommentSuggestion
			case types.CommentSuggestion:
				m.commentType = types.CommentNote
			case types.CommentNote:
				m.commentType = types.CommentPraise
			case types.CommentPraise:
				m.commentType = types.CommentIssue
			}
		case "backspace":
			m.deleteBeforeCursor()
		case "delete", "ctrl+d":
			m.deleteAtCursor()
		case "ctrl+k":
			m.killToLineEnd()
		case "ctrl+u":
			m.killToLineStart()
		case "ctrl+w":
			m.deleteWordBeforeCursor()
		case "left", "ctrl+b":
			if m.cursor > 0 {
				m.cursor--
			}
		case "right", "ctrl+f":
			if m.cursor < len([]rune(m.body)) {
				m.cursor++
			}
		case "up", "ctrl+p":
			m.moveCursorVertical(-1)
		case "down", "ctrl+n":
			m.moveCursorVertical(1)
		case "home", "ctrl+a":
			m.moveCursorToLineStart()
		case "end", "ctrl+e":
			m.moveCursorToLineEnd()
		case "ctrl+g":
			return m, func() tea.Msg {
				return externalEditorRequestMsg{body: m.body, origin: overlayComment}
			}
		case "space":
			m.insertAtCursor(" ")
		default:
			// Only add printable characters
			key := msg.String()
			if len(key) == 1 {
				m.insertAtCursor(key)
			}
		}
	}
	return m, nil
}

// insertAtCursor inserts text at the cursor position and advances the cursor.
func (m *commentEditorModel) insertAtCursor(s string) {
	runes := []rune(m.body)
	inserted := []rune(s)
	result := make([]rune, 0, len(runes)+len(inserted))
	result = append(result, runes[:m.cursor]...)
	result = append(result, inserted...)
	result = append(result, runes[m.cursor:]...)
	m.body = string(result)
	m.cursor += len(inserted)
}

// deleteBeforeCursor deletes one rune before the cursor (backspace).
func (m *commentEditorModel) deleteBeforeCursor() {
	if m.cursor == 0 {
		return
	}
	runes := []rune(m.body)
	m.body = string(append(runes[:m.cursor-1], runes[m.cursor:]...))
	m.cursor--
}

// deleteAtCursor deletes one rune at the cursor position (forward delete).
func (m *commentEditorModel) deleteAtCursor() {
	runes := []rune(m.body)
	if m.cursor >= len(runes) {
		return
	}
	m.body = string(append(runes[:m.cursor], runes[m.cursor+1:]...))
}

// killToLineEnd deletes from cursor to the end of the current line (Ctrl+K).
func (m *commentEditorModel) killToLineEnd() {
	runes := []rune(m.body)
	if m.cursor >= len(runes) {
		return
	}
	// Find next newline or end of body
	end := m.cursor
	for end < len(runes) && runes[end] != '\n' {
		end++
	}
	// If cursor is already at a newline, delete just the newline
	if end == m.cursor && end < len(runes) {
		end++
	}
	m.body = string(runes[:m.cursor]) + string(runes[end:])
}

// killToLineStart deletes from start of current line to cursor (Ctrl+U).
func (m *commentEditorModel) killToLineStart() {
	if m.cursor == 0 {
		return
	}
	runes := []rune(m.body)
	// Find start of current line
	start := m.cursor
	for start > 0 && runes[start-1] != '\n' {
		start--
	}
	m.body = string(runes[:start]) + string(runes[m.cursor:])
	m.cursor = start
}

// deleteWordBeforeCursor deletes the word before the cursor (Ctrl+W).
func (m *commentEditorModel) deleteWordBeforeCursor() {
	if m.cursor == 0 {
		return
	}
	runes := []rune(m.body)
	end := m.cursor
	// Skip trailing whitespace
	for end > 0 && (runes[end-1] == ' ' || runes[end-1] == '\t') {
		end--
	}
	// Delete back to start of word
	for end > 0 && runes[end-1] != ' ' && runes[end-1] != '\t' && runes[end-1] != '\n' {
		end--
	}
	m.body = string(runes[:end]) + string(runes[m.cursor:])
	m.cursor = end
}

// moveCursorVertical moves the cursor up (dir=-1) or down (dir=1) by one line.
func (m *commentEditorModel) moveCursorVertical(dir int) {
	runes := []rune(m.body)
	lines := strings.Split(string(runes), "\n")

	// Find current line and column from cursor position
	currentLine := 0
	currentCol := 0
	pos := 0
	for _, r := range runes {
		if pos == m.cursor {
			break
		}
		if r == '\n' {
			currentLine++
			currentCol = 0
		} else {
			currentCol++
		}
		pos++
	}

	targetLine := currentLine + dir
	if targetLine < 0 || targetLine >= len(lines) {
		return
	}

	// Calculate new cursor position
	newPos := 0
	for i := 0; i < targetLine; i++ {
		newPos += len([]rune(lines[i])) + 1 // +1 for newline
	}
	targetCol := currentCol
	lineLen := len([]rune(lines[targetLine]))
	if targetCol > lineLen {
		targetCol = lineLen
	}
	newPos += targetCol
	m.cursor = newPos
}

// moveCursorToLineStart moves the cursor to the first non-whitespace character
// of the current line. If already there (or before it), jumps to column 0.
func (m *commentEditorModel) moveCursorToLineStart() {
	runes := []rune(m.body)

	// Find the start of the current line (column 0)
	lineStart := 0
	for i := m.cursor - 1; i >= 0; i-- {
		if runes[i] == '\n' {
			lineStart = i + 1
			break
		}
	}

	// Find first non-whitespace character on this line
	firstNonWS := lineStart
	for firstNonWS < len(runes) && runes[firstNonWS] != '\n' && (runes[firstNonWS] == ' ' || runes[firstNonWS] == '\t') {
		firstNonWS++
	}

	// Toggle between first non-whitespace and column 0
	if m.cursor == firstNonWS || (m.cursor != lineStart && m.cursor < firstNonWS) {
		m.cursor = lineStart
	} else {
		m.cursor = firstNonWS
	}
}

// moveCursorToLineEnd moves the cursor to the end of the current line.
func (m *commentEditorModel) moveCursorToLineEnd() {
	runes := []rune(m.body)
	for i := m.cursor; i < len(runes); i++ {
		if runes[i] == '\n' {
			m.cursor = i
			return
		}
	}
	m.cursor = len(runes)
}

func (m commentEditorModel) View() string {
	if !m.active {
		return ""
	}

	modalWidth := calcModalWidth(m.width, 0)

	var b strings.Builder

	// Title
	title := "New Comment"
	if m.editingID != "" {
		title = "Edit Comment"
	} else if m.commentType == types.CommentSuggestion && strings.Contains(m.body, "```suggestion") {
		title = "New Suggestion"
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(title))
	b.WriteString("\n\n")

	// Target
	if m.lineStart > 0 {
		if m.lineEnd > m.lineStart {
			b.WriteString(fmt.Sprintf("File: %s (lines %d-%d)\n", m.path, m.lineStart, m.lineEnd))
		} else {
			b.WriteString(fmt.Sprintf("File: %s (line %d)\n", m.path, m.lineStart))
		}
	} else {
		b.WriteString(fmt.Sprintf("File: %s (file-level comment)\n", m.path))
	}
	b.WriteString("\n")

	// Type selector — each type has a color; selected gets solid bg, unselected gets colored text
	typeLabels := []struct {
		t     types.CommentType
		label string
		color color.Color
	}{
		{types.CommentIssue, "ISSUE", lipgloss.Color("1")},
		{types.CommentSuggestion, "SUGGESTION", lipgloss.Color("3")},
		{types.CommentNote, "NOTE", lipgloss.Color("4")},
		{types.CommentPraise, "PRAISE", lipgloss.Color("2")},
	}
	for i, tl := range typeLabels {
		var style lipgloss.Style
		if tl.t == m.commentType {
			style = lipgloss.NewStyle().
				Background(tl.color).
				Foreground(lipgloss.Color("0")).
				Bold(true).
				Padding(0, 1)
		} else {
			style = lipgloss.NewStyle().
				Foreground(tl.color).
				Padding(0, 1)
		}
		b.WriteString(style.Render(tl.label))
		if i < len(typeLabels)-1 {
			b.WriteString(" ")
		}
	}
	b.WriteString("  ")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("(Tab)"))
	b.WriteString("\n\n")

	// Text area — render cursor as reverse-video block over the character at cursor position
	runes := []rune(m.body)
	pos := m.cursor
	if pos > len(runes) {
		pos = len(runes)
	}
	cursorStyle := lipgloss.NewStyle().Reverse(true)
	var bodyDisplay string
	if pos < len(runes) {
		ch := runes[pos]
		if ch == '\n' {
			// Show inverted space before the newline
			bodyDisplay = string(runes[:pos]) + cursorStyle.Render(" ") + string(runes[pos:])
		} else {
			bodyDisplay = string(runes[:pos]) + cursorStyle.Render(string(ch)) + string(runes[pos+1:])
		}
	} else {
		// Cursor past end — show inverted space
		bodyDisplay = string(runes) + cursorStyle.Render(" ")
	}
	b.WriteString(bodyDisplay)
	b.WriteString("\n\n")

	// Hints
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Enter: save  Shift+Enter: newline  Ctrl+g: editor  Esc: cancel  Tab: cycle type"))

	return m.theme.ModalBorder.Width(modalWidth).Render(b.String())
}

func (m *commentEditorModel) open(path string, lineStart, lineEnd int, targetType types.TargetType) {
	m.active = true
	m.path = path
	m.lineStart = lineStart
	m.lineEnd = lineEnd
	m.targetType = targetType
	m.commentType = types.CommentIssue
	m.body = ""
	m.cursor = 0
	m.editingID = ""
}

// handleClick processes a mouse click at content-relative coordinates.
// Returns true if the click was on an interactive element.
func (m *commentEditorModel) handleClick(contentX, contentY int) bool {
	// Type labels are on line 4: title(0), blank(1), file info(2), blank(3), labels(4)
	typeLabelLine := 4
	if contentY != typeLabelLine {
		return false
	}

	labels := []struct {
		t     types.CommentType
		label string
	}{
		{types.CommentIssue, "ISSUE"},
		{types.CommentSuggestion, "SUGGESTION"},
		{types.CommentNote, "NOTE"},
		{types.CommentPraise, "PRAISE"},
	}

	x := 0
	for _, l := range labels {
		labelW := len(l.label) + 2 // padding(0,1) adds 1 each side
		if contentX >= x && contentX < x+labelW {
			m.commentType = l.t
			return true
		}
		x += labelW + 1 // +1 for " " separator
	}
	return false
}

func (m *commentEditorModel) openSuggest(path string, lineStart, lineEnd int, targetType types.TargetType, body string, commentType types.CommentType) {
	m.active = true
	m.path = path
	m.lineStart = lineStart
	m.lineEnd = lineEnd
	m.targetType = targetType
	m.commentType = commentType
	m.body = body
	// Position cursor at end of code content, before the closing fence
	runes := []rune(body)
	m.cursor = len(runes) - 4 // skip "\n```"
	if m.cursor < 0 {
		m.cursor = len(runes)
	}
	m.editingID = ""
}

func (m *commentEditorModel) openEdit(comment *types.ReviewComment) {
	m.active = true
	m.path = comment.TargetRef
	m.lineStart = comment.LineStart
	m.lineEnd = comment.LineEnd
	m.targetType = comment.TargetType
	m.commentType = comment.Type
	m.body = comment.Body
	m.cursor = len([]rune(comment.Body))
	m.editingID = comment.ID
}
