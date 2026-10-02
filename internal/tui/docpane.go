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

	// Note mode: the pane shows prose the agent wrote — a tour stop's note —
	// rather than an excerpt of a document. There are no line numbers to show
	// and nothing to highlight, and prose has to wrap, so it renders
	// differently. noteSource is the markdown; lines hold it styled and wrapped
	// for noteWidth, and are rebuilt when the width changes.
	note       bool
	noteSource string
	noteWidth  int
	styler     *markdownStyler

	// groups follow the note as rows of clickable labels (a tour stop's
	// related files, views, layout). They are laid out into pinned, which
	// stays at the bottom of the pane while only the note above it scrolls —
	// a long note must not push the labels out of reach — and linkHits say
	// where each label is among the pinned rows, so a click can be mapped
	// back.
	groups   []linkGroup
	pinned   []string
	linkHits []linkHit
}

// openNote shows a block of markdown under a heading, followed by rows of
// labels. key identifies what is showing (like annotationID does for refs), so
// the caller can tell whether the pane already holds it.
func (m *docPaneModel) openNote(key, title, body string, groups []linkGroup, styler *markdownStyler) {
	m.active = true
	m.note = true
	m.annotationID = key
	m.title = title
	m.noteSource = body
	m.groups = groups
	m.styler = styler
	m.refs = nil
	m.activeRef = 0
	m.hlStartLine, m.hlStartCol, m.hlEndLine, m.hlEndCol = 0, 0, 0, 0
	m.rangeShifted = false
	m.offset = 0
	m.noteWidth = -1 // force a wrap at the next width we learn
	m.reflow()
}

// reflow re-wraps the note for the current width. A no-op outside note mode,
// and when the width has not changed.
func (m *docPaneModel) reflow() {
	if !m.note || m.noteWidth == m.width {
		return
	}
	m.noteWidth = m.width
	m.lines = wrapNote(m.noteSource, m.width, m.styler)
	m.pinned, m.linkHits = layoutGroups(m.groups, m.width)
	m.clamp()
}

// pinnedShown is how many pinned rows fit: all of them, unless the pane is
// shorter than they are.
func (m docPaneModel) pinnedShown() int {
	return min(len(m.pinned), m.viewportHeight())
}

// noteRows is how many rows the note itself gets: what the pinned rows leave,
// less a blank row between the two when there is room for one.
func (m docPaneModel) noteRows() int {
	rows := m.viewportHeight() - m.pinnedShown()
	if m.pinnedShown() > 0 && rows > 1 {
		rows-- // the blank row above the labels
	}
	return max(rows, 0)
}

// noteScrollHint says how much of the note is out of sight, for the title:
// "↓ 12 lines", "↑ 3 · ↓ 9 lines", or "" when it all fits.
func (m docPaneModel) noteScrollHint() string {
	above, below := m.offset, len(m.lines)-m.offset-m.noteRows()
	switch {
	case above > 0 && below > 0:
		return fmt.Sprintf("↑ %d · ↓ %d lines", above, below)
	case below > 0:
		return fmt.Sprintf("↓ %d lines", below)
	case above > 0:
		return fmt.Sprintf("↑ %d lines", above)
	}
	return ""
}

// wrapNote styles each markdown line and wraps it to the pane, leaving a
// one-column margin. Styling first, then wrapping, keeps a bold span that
// crosses a wrap boundary bold on both rows.
func wrapNote(src string, width int, styler *markdownStyler) []string {
	textW := width - 2
	if textW < 10 {
		textW = 10
	}
	var out []string
	for _, raw := range strings.Split(strings.TrimRight(src, "\n"), "\n") {
		styled := raw
		if styler != nil {
			styled = styler.StyleLine(raw)
		}
		for _, row := range wrapContent(styled, textW) {
			out = append(out, " "+row)
		}
	}
	return out
}

// noteHeight is how many rows the note wants, title included, at a width —
// so a two-line note does not take half the screen.
func (m docPaneModel) noteHeight(width int) int {
	rows, _ := layoutGroups(m.groups, width)
	if len(rows) > 0 {
		rows = append(rows, "") // the blank row before them
	}
	return len(wrapNote(m.noteSource, width, m.styler)) + len(rows) + 1
}

// pinnedTop is the viewport row (0-based, under the title) the pinned rows
// start on.
func (m docPaneModel) pinnedTop() int {
	return m.viewportHeight() - m.pinnedShown()
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
	max := len(m.lines) - m.noteRows()
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
	m.note = false
	m.noteSource = ""
	m.groups = nil
	m.pinned = nil
	m.linkHits = nil
	m.annotationID = ""
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
	// The width is only final at render time (the horizontal layout measures
	// the rendered sidebar), so re-wrap a note here if it moved. m is a copy.
	m.reflow()

	title := m.title
	if r, ok := m.currentRef(); ok && len(m.refs) > 1 {
		title = fmt.Sprintf("%s  (ref %d/%d)", title, m.activeRef+1, len(m.refs))
		_ = r
	}
	if m.rangeShifted {
		title += "  · range may have shifted"
	}
	if m.note {
		if hint := m.noteScrollHint(); hint != "" {
			title += "  " + hint
		}
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(" " + truncateToWidth(title, m.width-1)))

	lineNumStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	hlStyle := lipgloss.NewStyle().Background(accent).Foreground(lipgloss.Color("0"))

	vp := m.viewportHeight()
	if m.note {
		// The note scrolls; the label rows stay pinned to the bottom.
		top, rows := m.pinnedTop(), m.noteRows()
		for i := 0; i < vp; i++ {
			b.WriteString("\n")
			switch {
			case i >= top:
				b.WriteString(truncateToWidth(m.pinned[i-top], m.width))
			case i < rows && m.offset+i < len(m.lines):
				b.WriteString(truncateToWidth(m.lines[m.offset+i], m.width))
			}
		}
		return b.String()
	}
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
