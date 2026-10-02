package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// A tour note ends with what else the stop carries — its related files, its
// views, a saved layout to reset — as rows of labels the reviewer can click.
// Each label carries the message its keyboard command sends (`:related 2`,
// `:view 2`, `:layout reset`), so a click and the command run the same code.

// noteLink is one clickable label under a note. act is the message a click on
// it sends; state is a view's marker, if known; middle cuts a label too wide
// for the pane in the middle rather than at the end, so a path keeps its file
// name and line.
type noteLink struct {
	label  string
	act    tea.Msg
	state  viewState
	middle bool
}

// linkGroup is a heading and its labels: "Views:" and the stop's views. lead
// is plain text between the heading and the labels; hint follows the labels
// when it fits, saying how to reach them from the keyboard.
type linkGroup struct {
	head  string
	lead  string
	links []noteLink
	hint  string
	trail *noteTrail
}

// noteTrail is set flush right on a group's last row: the stop's layout, always
// shown (John 2026-10-02), "layout default" or "layout saved · reset" with
// "reset" a label. It wins the row's room over the keyboard hint.
type noteTrail struct {
	text string
	link *noteLink
}

func (t noteTrail) width() int {
	w := lipgloss.Width(t.text)
	if t.link != nil {
		w += 3 + lipgloss.Width(t.link.label)
	}
	return w
}

// linkHit is where a label was laid out — a row of the pinned rows and the
// columns it covers, [start, end) — and what a click there sends.
type linkHit struct {
	line, start, end int
	act              tea.Msg
}

// minLinkLabel is the narrowest a label is cut to before its marker is dropped
// to make room: "[2]…" still says which one it is.
const minLinkLabel = 4

// linkAt is what a click at a point in the pane sends — x a column, y a row
// counting the title as 0 — or false when the point is not on a label. Labels
// are pinned to the bottom of the pane, so the note's scroll does not move them.
func (m docPaneModel) linkAt(x, y int) (tea.Msg, bool) {
	if !m.active || !m.note || y < m.headRows() || y >= m.headRows()+m.viewportHeight() {
		return nil, false
	}
	line := y - m.headRows() - m.pinnedTop() + m.pinOffset
	if line < 0 {
		return nil, false
	}
	for _, h := range m.linkHits {
		if h.line == line && x >= h.start && x < h.end {
			return h.act, true
		}
	}
	return nil, false
}

// setGroups replaces the labels under the note, keeping the pane where it is
// scrolled to.
func (m *docPaneModel) setGroups(groups []linkGroup) {
	m.groups = groups
	m.noteWidth = -1
	m.reflow()
}

// layoutGroups lays groups out as rows, each group starting a row of its own,
// and says where each label landed, rows counted from the first.
func layoutGroups(groups []linkGroup, width int) ([]string, []linkHit) {
	var rows []string
	var hits []linkHit
	for _, g := range groups {
		r, h := layoutGroup(g, width)
		for i := range h {
			h[i].line += len(rows)
		}
		rows, hits = append(rows, r...), append(hits, h...)
	}
	return rows, hits
}

// layoutGroup lays one group out: its heading, its lead, then each label —
// bold, underlined, in the accent — and its marker, moving to a new row
// rather than splitting a label across two, since a label is clicked as one
// span. Rows carry the note's one-column margin. A group with no labels has no
// rows.
func layoutGroup(g linkGroup, width int) ([]string, []linkHit) {
	if len(g.links) == 0 && g.trail == nil {
		return nil, nil
	}
	textW := max(width-2, 10)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(annotationColor)).Bold(true).Underline(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	indentW := lipgloss.Width(g.head) + 1
	indent := strings.Repeat(" ", indentW)

	lead := g.lead
	if indentW+lipgloss.Width(lead) > textW {
		lead = "" // a pane this narrow has room for the labels only
	}
	var rows []string
	var hits []linkHit
	row := lipgloss.NewStyle().Bold(true).Render(g.head) + " " + lead
	used := indentW + lipgloss.Width(lead) // columns taken on this row, after the margin
	for _, l := range g.links {
		label := l.label
		marker := l.state.marker()
		if marker != "" {
			marker = " " + marker
		}
		sep := ""
		if used > indentW {
			sep = " · "
			if used+lipgloss.Width(sep)+lipgloss.Width(label)+lipgloss.Width(marker) > textW {
				rows = append(rows, " "+row)
				row, used, sep = indent, indentW, ""
			}
		}
		used += lipgloss.Width(sep)
		room := textW - used - lipgloss.Width(marker)
		if room < minLinkLabel && marker != "" {
			// Too narrow for both: the label is what can be clicked.
			marker, room = "", textW-used
		}
		label = cutLabel(label, max(room, 1), l.middle)
		w := lipgloss.Width(label)
		hits = append(hits, linkHit{line: len(rows), start: 1 + used, end: 1 + used + w, act: l.act})
		row += dim.Render(sep) + labelStyle.Render(label) + marker
		used += w + lipgloss.Width(marker)
	}
	tw := 0
	if g.trail != nil {
		tw = g.trail.width() + 2
	}
	if g.hint != "" && used+2+lipgloss.Width(g.hint)+tw <= textW {
		row += "  " + dim.Render(g.hint)
		used += 2 + lipgloss.Width(g.hint)
	}
	if g.trail != nil {
		if used+tw > textW && len(g.links) > 0 {
			rows = append(rows, " "+row)
			row, used = "", 0
		}
		pad := max(textW-used-g.trail.width(), 1)
		row += strings.Repeat(" ", pad) + dim.Render(g.trail.text)
		col := used + pad + lipgloss.Width(g.trail.text)
		if l := g.trail.link; l != nil {
			row += dim.Render(" · ") + labelStyle.Render(l.label)
			hits = append(hits, linkHit{line: len(rows), start: 1 + col + 3, end: 1 + col + 3 + lipgloss.Width(l.label), act: l.act})
		}
	}
	return append(rows, " "+row), hits
}

// cutLabel fits a label into room columns: in the middle for a path, keeping
// its file name and line, else at the end.
func cutLabel(label string, room int, middle bool) string {
	if lipgloss.Width(label) <= room {
		return label
	}
	if middle {
		if cut := truncateMiddle(label, room); lipgloss.Width(cut) <= room {
			return cut
		}
	}
	return ansi.Truncate(label, room, "…")
}
