package tui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/core"
	"github.com/josephschmitt/monocle/internal/types"
)

// reviewCommitLimit bounds the commit list. A review with more commits than this
// is not one anybody reads commit by commit, and an unbounded log would make
// opening the modal cost more the longer the branch.
const reviewCommitLimit = 100

// summaryPalette assigns each item a colour it keeps everywhere it appears: the
// modal row, the gutter bar on its hunks, the active-filter badge. Magenta, cyan
// and blue lead because red, green and yellow already mean removed, added and
// suggestion in a diff — an item bar in those would read as diff semantics.
var summaryPalette = []color.Color{
	lipgloss.Color("5"), lipgloss.Color("6"), lipgloss.Color("4"),
	lipgloss.Color("3"), lipgloss.Color("2"), lipgloss.Color("1"),
}

// summaryColor returns the colour for the item at a position. It cycles rather
// than running out: past six items the colours repeat, which is still better
// than some items having none.
func summaryColor(i int) color.Color {
	if i < 0 {
		return lipgloss.Color("8")
	}
	return summaryPalette[i%len(summaryPalette)]
}

// summaryModalModel is the review's front page: what the agent says this round
// fixed, and which commits it contains. Two halves because they answer different
// questions — the items are the intent, the commits are the record — and reading
// one against the other is most of what a reviewer does first.
type summaryModalModel struct {
	active   bool
	items    []types.SummaryItem
	overview string // the round in a sentence or two, above the items
	commits  []core.LogEntry
	base     string
	name     string // the review's name, for the modal title
	cursor   int
	activeID string // the item currently filtering the view, if any
	width    int
	height   int
	theme    Theme
}

func newSummaryModalModel(theme Theme) summaryModalModel {
	return summaryModalModel{theme: theme}
}

// openSummaryMsg carries everything the modal shows. Commits are fetched when it
// opens rather than held on the model, so they cannot go stale behind it.
type openSummaryMsg struct {
	items    []types.SummaryItem
	overview string
	commits  []core.LogEntry
	base     string
	name     string
}

// selectSummaryItemMsg filters the review to one item; an empty ID clears the
// filter, which is what makes esc able to both close and clear.
type selectSummaryItemMsg struct {
	id string
}

type closeSummaryMsg struct{}

func (m *summaryModalModel) open(msg openSummaryMsg) {
	m.active = true
	m.items = msg.items
	m.overview = msg.overview
	m.commits = msg.commits
	m.base = msg.base
	m.name = msg.name
	m.cursor = 0
	// Open on the item already filtering, so reopening to change selection does
	// not start from the top of the list every time.
	for i, it := range m.items {
		if it.ID == m.activeID {
			m.cursor = i
			break
		}
	}
}

func (m summaryModalModel) Update(msg tea.Msg) (summaryModalModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.active = false
		// Esc clears an active filter as well as closing, so the key that got you
		// out of the modal is also the key that gets you out of the filter.
		if m.activeID != "" {
			id := ""
			return m, func() tea.Msg { return selectSummaryItemMsg{id: id} }
		}
		return m, func() tea.Msg { return closeSummaryMsg{} }
	case "q":
		m.active = false
		return m, func() tea.Msg { return closeSummaryMsg{} }
	case "j", "down":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "g":
		m.cursor = 0
	case "G":
		if len(m.items) > 0 {
			m.cursor = len(m.items) - 1
		}
	case "enter", " ", "space":
		if m.cursor < 0 || m.cursor >= len(m.items) {
			return m, nil
		}
		id := m.items[m.cursor].ID
		// Selecting the item already active turns the filter off, so one key both
		// applies and undoes it.
		if id == m.activeID {
			id = ""
		}
		m.active = false
		return m, func() tea.Msg { return selectSummaryItemMsg{id: id} }
	}
	return m, nil
}

func (m summaryModalModel) View() string {
	if !m.active {
		return ""
	}

	boxW := CalcModalWidth(m.width, 110)
	contentW := boxW - 6
	if contentW < 24 {
		contentW = 24
	}

	// Side by side when there is room for two readable columns; stacked when
	// narrow, because a 20-column commit subject is worse than no columns.
	const gap = 3
	leftW, rightW := contentW, 0
	side := contentW >= 74
	if side {
		rightW = contentW/2 - gap
		leftW = contentW - rightW - gap
	}

	var b strings.Builder
	title := "Review Summary"
	if m.name != "" {
		title = m.name
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(title) + "\n")
	if m.overview != "" {
		// Above the columns and across the full width: it describes the round, not
		// either column, and wrapping it into one would make it read as a note on
		// the items rather than the thing they add up to.
		for _, row := range wrapContent(m.overview, contentW) {
			b.WriteString(row + "\n")
		}
	}
	b.WriteString("\n")

	left := m.itemRows(leftW)
	right := m.commitRows(rightW)
	if !side {
		b.WriteString(strings.Join(left, "\n"))
		b.WriteString("\n\n")
		b.WriteString(strings.Join(m.commitRows(contentW), "\n"))
	} else {
		rows := max(len(left), len(right))
		for i := 0; i < rows; i++ {
			b.WriteString(padToWidth(rowAt(left, i), leftW))
			b.WriteString(strings.Repeat(" ", gap))
			b.WriteString(rowAt(right, i))
			if i < rows-1 {
				b.WriteString("\n")
			}
		}
	}

	if detail := m.detailRows(contentW); len(detail) > 0 {
		b.WriteString("\n\n")
		b.WriteString(strings.Join(detail, "\n"))
	}

	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(m.hint()))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("4")).
		Padding(1, 2).
		Width(boxW).
		Render(b.String())
}

func (m summaryModalModel) hint() string {
	if len(m.items) == 0 {
		return "esc: close"
	}
	if m.activeID != "" {
		return "j/k: move   enter: show only this item   esc: clear the filter and close"
	}
	return "j/k: move   enter: show only this item   esc: close"
}

func rowAt(rows []string, i int) string {
	if i < len(rows) {
		return rows[i]
	}
	return ""
}

// itemRows renders the left half: one row per item, led by its colour bar. Rows
// are composed as plain text and styled last, so the cursor highlight spans
// exactly the same width as an unselected row.
func (m summaryModalModel) itemRows(w int) []string {
	head := lipgloss.NewStyle().Faint(true).Render("What this round fixed")
	if len(m.items) == 0 {
		return []string{head, lipgloss.NewStyle().Faint(true).Render("  no summary sent for this review")}
	}
	rows := []string{head}
	for i, it := range m.items {
		mark := " "
		if it.ID == m.activeID {
			mark = "✓"
		}
		count := m.targetLabel(it)
		// "▌ x " prefix is 4 cells; the count sits right-aligned after a space.
		room := w - 4 - lipgloss.Width(count) - 1
		text := it.Text
		if room > 3 && lipgloss.Width(text) > room {
			text = truncateMiddle(text, room)
		}
		body := padToWidth(mark+" "+text, w-2-lipgloss.Width(count)) + count

		bar := lipgloss.NewStyle().Foreground(summaryColor(i)).Render("▌")
		if i == m.cursor {
			rows = append(rows, bar+lipgloss.NewStyle().Reverse(true).Render(padToWidth(body, w-1)))
			continue
		}
		rows = append(rows, bar+body)
	}
	return rows
}

// detailRows shows the item under the cursor in full: its whole line of text,
// which the list itself has to truncate to keep one row per item, and what it
// actually points at. Without this the list can name a file count but never the
// file, and a trimmed item is unreadable rather than merely abbreviated.
func (m summaryModalModel) detailRows(w int) []string {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return nil
	}
	it := m.items[m.cursor]
	bar := lipgloss.NewStyle().Foreground(summaryColor(m.cursor)).Render("▌")
	var rows []string
	for i, row := range wrapContent(it.Text, w-2) {
		if i == 0 {
			rows = append(rows, bar+" "+row)
			continue
		}
		rows = append(rows, "  "+row)
	}
	if where := m.targetDetail(it); where != "" {
		for _, row := range wrapContent(where, w-2) {
			rows = append(rows, "  "+lipgloss.NewStyle().Faint(true).Render(row))
		}
	}
	return rows
}

// targetDetail names what the item points at, so the reviewer can tell whether a
// "2 files" item covers the two they care about before selecting it.
func (m summaryModalModel) targetDetail(it types.SummaryItem) string {
	if len(it.Targets) == 0 {
		return "no targets — this item cannot be filtered to"
	}
	var names []string
	seen := map[string]bool{}
	for _, t := range it.Targets {
		name := t.Path
		if t.IsArtifact() {
			name = "artifact " + t.Artifact
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

// targetLabel says how much of the diff an item accounts for, in files — the
// unit the reviewer navigates in. An item with no targets says so, because an
// untagged item cannot be selected usefully.
func (m summaryModalModel) targetLabel(it types.SummaryItem) string {
	if len(it.Targets) == 0 {
		return "untagged"
	}
	files, artifacts := map[string]bool{}, map[string]bool{}
	for _, t := range it.Targets {
		if t.IsArtifact() {
			artifacts[t.Artifact] = true
			continue
		}
		files[t.Path] = true
	}
	switch {
	case len(artifacts) == 0:
		return fmt.Sprintf("%d file%s", len(files), plural(len(files)))
	case len(files) == 0:
		return fmt.Sprintf("%d artifact%s", len(artifacts), plural(len(artifacts)))
	default:
		return fmt.Sprintf("%d file%s +%d", len(files), plural(len(files)), len(artifacts))
	}
}

// commitRows renders the right half: the commits the review contains. An empty
// list is still worth a row — "uncommitted changes" is an answer, and leaving
// the column blank would read as a failure to load.
func (m summaryModalModel) commitRows(w int) []string {
	if w <= 0 {
		return nil
	}
	label := "Commits"
	if m.base != "" {
		label = "Commits since " + shortHash(m.base)
	}
	rows := []string{lipgloss.NewStyle().Faint(true).Render(label)}
	if len(m.commits) == 0 {
		return append(rows, lipgloss.NewStyle().Faint(true).Render("  uncommitted changes"))
	}
	hashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	for _, c := range m.commits {
		subject := c.Subject
		room := w - lipgloss.Width(c.Hash) - 3
		if room > 3 && lipgloss.Width(subject) > room {
			subject = truncateMiddle(subject, room)
		}
		rows = append(rows, "  "+hashStyle.Render(c.Hash)+" "+subject)
	}
	return rows
}
