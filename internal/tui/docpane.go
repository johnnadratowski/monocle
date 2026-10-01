package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

	// links follow the note as clickable labels (a tour stop's views), and
	// linkHits say where reflow put each one, so a click can be mapped back.
	links    []noteLink
	linkHits []linkHit
}

// noteLink is a label under a note that opens something when clicked: one of a
// tour stop's views.
type noteLink struct {
	label string
}

// linkHit is where a link was laid out: a row of the pane's lines and the
// columns its label covers, [start, end). n is the link's position, 1-based.
type linkHit struct {
	line, start, end int
	n                int
}

// openNote shows a block of markdown under a heading, followed by links. key
// identifies what is showing (like annotationID does for refs), so the caller
// can tell whether the pane already holds it.
func (m *docPaneModel) openNote(key, title, body string, links []noteLink, styler *markdownStyler) {
	m.active = true
	m.note = true
	m.annotationID = key
	m.title = title
	m.noteSource = body
	m.links = links
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
	rows, hits := layoutLinks(m.links, m.width)
	for i := range hits {
		hits[i].line += len(m.lines)
	}
	m.lines = append(m.lines, rows...)
	m.linkHits = hits
	m.clamp()
}

// linkAt is the link (1-based) whose label is at a point in the pane — x a
// column, y a row counting the title as 0 — or false for anywhere else.
func (m docPaneModel) linkAt(x, y int) (int, bool) {
	if !m.active || !m.note || y < 1 || y > m.viewportHeight() {
		return 0, false
	}
	line := m.offset + y - 1
	for _, h := range m.linkHits {
		if h.line == line && x >= h.start && x < h.end {
			return h.n, true
		}
	}
	return 0, false
}

// linkHint follows the labels when it fits: they are live, and this is how to
// reach them from the keyboard.
const linkHint = "click, or :view N"

// layoutLinks lays a note's links out as rows under it: "Views:", then each
// label — bold, underlined, in the accent — moving to a new row rather than
// splitting a label across two, since a label is clicked as one span. It
// returns the rows, with the note's one-column margin, and where each label
// landed, rows counted from the first of them.
func layoutLinks(links []noteLink, width int) ([]string, []linkHit) {
	if len(links) == 0 {
		return nil, nil
	}
	textW := width - 2
	if textW < 10 {
		textW = 10
	}
	const head = "Views: "
	indent := strings.Repeat(" ", len(head))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(annotationColor)).Bold(true).Underline(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	var rows []string
	var hits []linkHit
	row := lipgloss.NewStyle().Bold(true).Render(strings.TrimSpace(head)) + " "
	used := len(head) // columns taken on this row, after the margin
	for i, l := range links {
		label := l.label
		sep := ""
		if used > len(indent) {
			sep = " · "
			if used+lipgloss.Width(sep)+lipgloss.Width(label) > textW {
				rows = append(rows, " "+row)
				row, used, sep = indent, len(indent), ""
			}
		}
		used += lipgloss.Width(sep)
		if room := textW - used; lipgloss.Width(label) > room {
			label = ansi.Truncate(label, room, "…")
		}
		w := lipgloss.Width(label)
		hits = append(hits, linkHit{line: len(rows), start: 1 + used, end: 1 + used + w, n: i + 1})
		row += dim.Render(sep) + labelStyle.Render(label)
		used += w
	}
	if used+2+len(linkHint) <= textW {
		row += "  " + dim.Render(linkHint)
	}
	return append(rows, " "+row), hits
}

// setLinks replaces the links under the note, keeping the pane where it is
// scrolled to.
func (m *docPaneModel) setLinks(links []noteLink) {
	m.links = links
	m.noteWidth = -1
	m.reflow()
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
	rows, _ := layoutLinks(m.links, width)
	return len(wrapNote(m.noteSource, width, m.styler)) + len(rows) + 1
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
	m.note = false
	m.noteSource = ""
	m.links = nil
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
	if m.note {
		// The width is only final at render time (the horizontal layout measures
		// the rendered sidebar), so re-wrap here if it moved. m is a copy.
		m.reflow()
		for i := 0; i < vp; i++ {
			b.WriteString("\n")
			if idx := m.offset + i; idx < len(m.lines) {
				b.WriteString(truncateToWidth(m.lines[idx], m.width))
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
