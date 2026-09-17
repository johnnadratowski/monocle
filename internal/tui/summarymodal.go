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
	items   []types.SummaryItem
	commits []core.LogEntry
	base    string
	name    string
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
	if contentW < 20 {
		contentW = 20
	}

	// Side by side when there is room for two readable columns; stacked when
	// narrow, because a 20-column commit subject is worse than no columns.
	leftW, rightW := contentW, 0
	const gap = 3
	if len(m.commits) > 0 && contentW >= 70 {
		rightW = contentW/2 - gap
		leftW = contentW - rightW - gap
	}

	var b strings.Builder
	title := "Review Summary"
	if m.name != "" {
		title = m.name
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(title) + "\n\n")

	left := m.itemRows(leftW)
	right := m.commitRows(rightW)
	if rightW == 0 {
		b.WriteString(strings.Join(left, "\n"))
		if len(m.commits) > 0 {
			b.WriteString("\n\n" + strings.Join(m.commitRows(contentW), "\n"))
		}
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

	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(m.hint()))
	return b.String()
}

func (m summaryModalModel) hint() string {
	if len(m.items) == 0 {
		return "esc: close"
	}
	if m.activeID != "" {
		return "j/k: move  enter: show only this item  esc: clear the filter and close"
	}
	return "j/k: move  enter: show only this item  esc: close"
}

func rowAt(rows []string, i int) string {
	if i < len(rows) {
		return rows[i]
	}
	return ""
}

// itemRows renders the left half: one row per item, led by its colour bar.
func (m summaryModalModel) itemRows(w int) []string {
	head := lipgloss.NewStyle().Faint(true).Render("What this round fixed")
	if len(m.items) == 0 {
		return []string{
			head,
			lipgloss.NewStyle().Faint(true).Render("  the agent sent no summary for this review"),
		}
	}
	rows := []string{head}
	for i, it := range m.items {
		bar := lipgloss.NewStyle().Foreground(summaryColor(i)).Render("▌")
		mark := " "
		if it.ID == m.activeID {
			mark = "✓"
		}
		text := it.Text
		// 4 leading cells (bar, space, mark, space) plus the target count.
		count := m.targetLabel(it)
		room := w - 4 - lipgloss.Width(count) - 1
		if room > 3 && lipgloss.Width(text) > room {
			text = truncateMiddle(text, room)
		}
		line := bar + " " + mark + " " + text
		if count != "" {
			line += " " + lipgloss.NewStyle().Faint(true).Render(count)
		}
		if i == m.cursor {
			line = lipgloss.NewStyle().Reverse(true).Render(padToWidth(bar+" "+mark+" "+text, w-lipgloss.Width(count)-1)) +
				" " + lipgloss.NewStyle().Faint(true).Render(count)
		}
		rows = append(rows, line)
	}
	return rows
}

// targetLabel says how much of the diff an item accounts for, in files — the
// unit the reviewer navigates in. An item with no targets says so, because an
// untagged item cannot be selected usefully.
func (m summaryModalModel) targetLabel(it types.SummaryItem) string {
	if len(it.Targets) == 0 {
		return "untagged"
	}
	files := map[string]bool{}
	for _, t := range it.Targets {
		files[t.Path] = true
	}
	if len(files) == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", len(files))
}

// commitRows renders the right half: the commits the review contains.
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
