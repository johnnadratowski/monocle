package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

// docPaneModel is the bottom pane that shows a referenced document passage beside
// the code, opened from an annotation's doc links. It renders a doc's lines with
// the referenced range highlighted and scrolled into view. It is inert (active
// false) until a ref is opened, so it adds nothing to the layout when closed.
type docPaneModel struct {
	active  bool
	focused bool
	width   int
	height  int
	theme   *Theme

	annotationID string // which annotation's refs are showing (for cycling)
	title        string
	lines        []string
	offset       int

	// The annotation's refs and which one is currently shown, so the same key can
	// cycle through all docs linked from one annotation.
	refs      []types.DocRef
	activeRef int

	// Highlight span (1-based lines, 0-based cols); zero end means unspecified.
	hlStartLine, hlStartCol, hlEndLine, hlEndCol int
	rangeShifted                                 bool // true when the range was clamped (doc drifted)
}

// openRefs begins showing an annotation's refs starting at index 0. The caller
// supplies the resolved content for the active ref via setContent.
func (m *docPaneModel) openRefs(refs []types.DocRef) {
	m.active = true
	m.refs = refs
	m.activeRef = 0
}

// nextRef advances to the next ref, wrapping. Returns the ref to load, or false
// if there are none.
func (m *docPaneModel) nextRef() (types.DocRef, bool) {
	if len(m.refs) == 0 {
		return types.DocRef{}, false
	}
	m.activeRef = (m.activeRef + 1) % len(m.refs)
	return m.refs[m.activeRef], true
}

// currentRef returns the ref currently being shown.
func (m docPaneModel) currentRef() (types.DocRef, bool) {
	if m.activeRef < 0 || m.activeRef >= len(m.refs) {
		return types.DocRef{}, false
	}
	return m.refs[m.activeRef], true
}

// setContent loads a document's text and the highlight range from the active
// ref, scrolling so the range is visible. content is the full document text.
func (m *docPaneModel) setContent(title, content string, ref types.DocRef) {
	m.title = title
	m.lines = strings.Split(content, "\n")
	m.hlStartLine, m.hlStartCol = ref.StartLine, ref.StartCol
	m.hlEndLine, m.hlEndCol = ref.EndLine, ref.EndCol
	if m.hlEndLine == 0 {
		m.hlEndLine = m.hlStartLine
	}
	m.rangeShifted = m.hlStartLine > len(m.lines)
	m.scrollToRange()
}

// scrollToRange positions the viewport so the highlight start is a few lines from
// the top, clamped to the document.
func (m *docPaneModel) scrollToRange() {
	target := m.hlStartLine - 1 // to 0-based
	if target < 0 {
		target = 0
	}
	if target >= len(m.lines) {
		target = len(m.lines) - 1
	}
	m.offset = target - 2
	m.clamp()
}

func (m *docPaneModel) clamp() {
	max := len(m.lines) - m.viewportHeight()
	if max < 0 {
		max = 0
	}
	if m.offset > max {
		m.offset = max
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *docPaneModel) scrollDown() { m.offset++; m.clamp() }
func (m *docPaneModel) scrollUp()   { m.offset--; m.clamp() }

func (m *docPaneModel) close() {
	m.active = false
	m.focused = false
	m.refs = nil
	m.lines = nil
}

// viewportHeight is the number of doc lines that fit, leaving one row for the
// title bar.
func (m docPaneModel) viewportHeight() int {
	h := m.height - 1
	if h < 1 {
		h = 1
	}
	return h
}

// View renders the doc pane: a title bar plus the visible doc lines with the
// referenced range highlighted.
func (m docPaneModel) View() string {
	if !m.active {
		return ""
	}
	accent := lipgloss.Color(annotationColor)
	titleStyle := lipgloss.NewStyle().Foreground(accent).Bold(true).Width(m.width)

	title := m.title
	if r, ok := m.currentRef(); ok && len(m.refs) > 1 {
		title = fmt.Sprintf("%s  (ref %d/%d)", title, m.activeRef+1, len(m.refs))
		_ = r
	}
	if m.rangeShifted {
		title += "  · range may have shifted"
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(" " + truncateToWidth(title, m.width-1)))

	lineNumStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	hlStyle := lipgloss.NewStyle().Background(accent).Foreground(lipgloss.Color("0"))

	vp := m.viewportHeight()
	for i := 0; i < vp; i++ {
		b.WriteString("\n")
		idx := m.offset + i
		if idx >= len(m.lines) {
			continue
		}
		lineNo := idx + 1 // 1-based
		gutter := lineNumStyle.Render(fmt.Sprintf("%5d ", lineNo))
		text := m.lines[idx]
		rendered := m.highlightLineText(lineNo, text, hlStyle)
		b.WriteString(gutter + truncateToWidth(rendered, m.width-6))
	}
	return b.String()
}

// highlightLineText applies the highlight background to the portion of a line
// that falls within the referenced range. Lines fully inside the range are
// highlighted end to end; the first/last line honor the column bounds.
func (m docPaneModel) highlightLineText(lineNo int, text string, hl lipgloss.Style) string {
	if lineNo < m.hlStartLine || lineNo > m.hlEndLine {
		return text
	}
	start := 0
	end := len(text)
	if lineNo == m.hlStartLine && m.hlStartCol > 0 && m.hlStartCol <= len(text) {
		start = m.hlStartCol
	}
	if lineNo == m.hlEndLine && m.hlEndCol > 0 && m.hlEndCol <= len(text) {
		end = m.hlEndCol
	}
	if start > end {
		start = end
	}
	return text[:start] + hl.Render(text[start:end]) + text[end:]
}

// truncateToWidth hard-caps a (possibly styled) string to a visual width.
func truncateToWidth(s string, w int) string {
	if w < 0 {
		w = 0
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}
