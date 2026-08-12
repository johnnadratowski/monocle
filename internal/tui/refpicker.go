package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/core"
	"github.com/josephschmitt/monocle/internal/types"
)

const refPickerPageSize = 20

type refPickerModel struct {
	entries          []core.LogEntry
	snapshots        []types.ReviewSnapshot
	cursor           int
	offset           int // scroll offset for visible entries
	width            int
	height           int
	active           bool
	autoActive       bool // whether auto-advance is currently on
	snapshotActive   bool // whether a snapshot is the current diff base
	activeSnapshotID int  // ID of the active snapshot (for checkmark)
	hasMore          bool // true if last fetch returned a full page
	loading          bool // true while loading more entries
	theme            Theme
}

func newRefPickerModel(theme Theme) refPickerModel {
	return refPickerModel{theme: theme}
}

type openRefPickerMsg struct {
	entries    []core.LogEntry
	snapshots  []types.ReviewSnapshot
	autoActive bool
}

type selectRefMsg struct {
	hash string
	auto bool
}

type selectSnapshotMsg struct {
	snapshotID int
}

type cancelRefPickerMsg struct{}

type loadMoreRefsMsg struct {
	entries []core.LogEntry
	hasMore bool
}

// snapshotCount returns the number of snapshot entries in the picker.
func (m refPickerModel) snapshotCount() int {
	return len(m.snapshots)
}

// workingTreeCursor returns the cursor position of the "Working Tree" entry.
// Layout: snapshots (0..N-1), working tree (N), commits (N+1..).
func (m refPickerModel) workingTreeCursor() int {
	return m.snapshotCount()
}

// commitStartCursor returns the cursor position of the first commit entry.
func (m refPickerModel) commitStartCursor() int {
	return m.workingTreeCursor() + 1
}

func (m refPickerModel) maxCursor() int {
	n := m.commitStartCursor() + len(m.entries) - 1
	if m.hasMore {
		n++ // "Load more..." option
	}
	return n
}

func (m refPickerModel) Update(msg tea.Msg) (refPickerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < m.maxCursor() {
				m.cursor++
				m.ensureVisible()
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
		case "g":
			m.cursor = 0
			m.ensureVisible()
		case "G":
			m.cursor = m.maxCursor()
			m.ensureVisible()
		case "enter":
			// Snapshot entries (top of list)
			if m.cursor < m.snapshotCount() {
				id := m.snapshots[m.cursor].ID
				return m, func() tea.Msg { return selectSnapshotMsg{snapshotID: id} }
			}
			// "Working Tree" option
			if m.cursor == m.workingTreeCursor() {
				return m, func() tea.Msg { return selectRefMsg{auto: true} }
			}
			// "Load more..." option
			commitStart := m.commitStartCursor()
			if m.hasMore && m.cursor == commitStart+len(m.entries) {
				m.loading = true
				return m, nil // app.go handles the actual fetch
			}
			idx := m.cursor - commitStart
			if idx >= 0 && idx < len(m.entries) {
				return m, func() tea.Msg { return selectRefMsg{hash: m.entries[idx].Hash} }
			}
		case "esc", "q":
			return m, func() tea.Msg { return cancelRefPickerMsg{} }
		}

	case loadMoreRefsMsg:
		m.entries = msg.entries
		m.hasMore = msg.hasMore
		m.loading = false
		m.ensureVisible()
	}
	return m, nil
}

func (m refPickerModel) View() string {
	if !m.active {
		return ""
	}

	boxW := CalcModalWidth(m.width, 80)
	contentW := boxW - 6 // 2 border + 4 padding (2 each side)

	var b strings.Builder

	title := lipgloss.NewStyle().Bold(true).Render("Select Base Ref")
	b.WriteString(title + "\n\n")

	// Snapshot entries (listed first when they exist)
	if len(m.snapshots) > 0 {
		faintSep := lipgloss.NewStyle().Faint(true)
		b.WriteString(faintSep.Render("  Since Review") + "\n")
		for i, snap := range m.snapshots {
			label := fmt.Sprintf("  Round %d (%s)", snap.ReviewRound, relativeTime(snap.CreatedAt))
			if m.snapshotActive && m.activeSnapshotID == snap.ID {
				label += " ✓"
			}
			if m.cursor == i {
				b.WriteString(lipgloss.NewStyle().Reverse(true).Render(label))
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render(label))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// "Working Tree" option — shows git diff against base ref
	wtLabel := "  Working Tree"
	if m.autoActive && !m.snapshotActive {
		wtLabel = "  Working Tree ✓"
	}
	if m.cursor == m.workingTreeCursor() {
		b.WriteString(lipgloss.NewStyle().Reverse(true).Render(wtLabel))
	} else {
		b.WriteString(wtLabel)
	}
	b.WriteString("\n\n")

	// Commit entries with scrolling
	vh := m.viewportHeight()
	end := m.offset + vh
	if end > len(m.entries) {
		end = len(m.entries)
	}

	faintStyle := lipgloss.NewStyle().Faint(true)
	if m.offset > 0 {
		b.WriteString(faintStyle.Render("  ▲ more") + "\n")
	}

	// Determine hash column width from the first entry (typically 7 chars)
	hashWidth := 7
	if len(m.entries) > 0 {
		hashWidth = len(m.entries[0].Hash)
	}
	hashColW := hashWidth + 3 // "  " prefix + hash + " " gap
	subjectW := contentW - hashColW
	if subjectW < 10 {
		subjectW = 10
	}
	hashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Width(hashColW).
		PaddingLeft(2).PaddingRight(1)
	subjectStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Width(subjectW)
	plainHashStyle := lipgloss.NewStyle().Width(hashColW).PaddingLeft(2).PaddingRight(1)
	plainSubjectStyle := lipgloss.NewStyle().Width(subjectW)
	commitStart := m.commitStartCursor()
	for i := m.offset; i < end; i++ {
		entry := m.entries[i]
		var line string
		if m.cursor == commitStart+i {
			// Render without foreground colors so Reverse works cleanly
			hashBlock := plainHashStyle.Render(entry.Hash)
			subjectBlock := plainSubjectStyle.Render(entry.Subject)
			line = lipgloss.JoinHorizontal(lipgloss.Top, hashBlock, subjectBlock)
			// Pad each line to full width so the highlight spans the row
			parts := strings.Split(line, "\n")
			for j, l := range parts {
				if w := lipgloss.Width(l); w < contentW {
					parts[j] = l + strings.Repeat(" ", contentW-w)
				}
			}
			line = lipgloss.NewStyle().Reverse(true).Render(strings.Join(parts, "\n"))
		} else {
			hashBlock := hashStyle.Render(entry.Hash)
			subjectBlock := subjectStyle.Render(entry.Subject)
			line = lipgloss.JoinHorizontal(lipgloss.Top, hashBlock, subjectBlock)
		}
		b.WriteString(line + "\n")
	}

	if m.hasMore {
		loadMoreLabel := "  Load more..."
		if m.loading {
			loadMoreLabel = "  Loading..."
		}
		if m.cursor == commitStart+len(m.entries) {
			b.WriteString(lipgloss.NewStyle().Reverse(true).Render(loadMoreLabel))
		} else {
			b.WriteString(faintStyle.Render(loadMoreLabel))
		}
		b.WriteString("\n")
	} else if end < len(m.entries) {
		b.WriteString(faintStyle.Render("  ▼ more") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("  enter:select  esc:cancel"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("4")).
		Padding(1, 2).
		Width(boxW).
		Render(b.String())
}

// handleClick processes a mouse click at content-relative coordinates.
// Returns a command and whether the click was handled.
func (m *refPickerModel) handleClick(contentY int) (tea.Cmd, bool) {
	// Content layout:
	// Line 0: "Select Base Ref"
	// Line 1: blank
	// If snapshots: "Since Review" header, snapshot entries, blank
	// "Working Tree" option, blank
	// Commit entries

	nextLine := 2 // after title + blank

	// Snapshot entries (if any)
	if len(m.snapshots) > 0 {
		// "Since Review" header
		nextLine++ // header line
		snapStartLine := nextLine
		for i := range m.snapshots {
			if contentY == snapStartLine+i {
				m.cursor = i
				id := m.snapshots[i].ID
				return func() tea.Msg { return selectSnapshotMsg{snapshotID: id} }, true
			}
		}
		nextLine += len(m.snapshots) + 1 // entries + blank
	}

	// Working Tree option
	if contentY == nextLine {
		m.cursor = m.workingTreeCursor()
		return func() tea.Msg { return selectRefMsg{auto: true} }, true
	}
	nextLine += 2 // option + blank

	// Commit entries
	entryStartLine := nextLine
	if m.offset > 0 {
		entryStartLine++ // "▲ more" indicator line
	}

	clickedIdx := contentY - entryStartLine + m.offset
	if clickedIdx >= 0 && clickedIdx < len(m.entries) {
		m.cursor = m.commitStartCursor() + clickedIdx
		hash := m.entries[clickedIdx].Hash
		return func() tea.Msg { return selectRefMsg{hash: hash} }, true
	}

	return nil, false
}

// viewportHeight returns how many commit entries fit in the ref picker.
// Uses a stable calculation based only on screen size so the modal
// doesn't resize while scrolling.
func (m refPickerModel) viewportHeight() int {
	// Cap modal to ~60% of screen height, minus chrome
	// Chrome: title(1) + blank(1) + auto line(1) + blank(1) + load-more/footer(2) + padding(2) + border(2) = 10
	maxModalH := m.height * 3 / 5
	availableLines := maxModalH - 10
	if availableLines < 1 {
		return 1
	}
	// Budget ~2/3 of available lines for entries to account for wrapping
	vh := availableLines * 2 / 3
	if vh < 1 {
		vh = 1
	}
	return vh
}

// ensureVisible adjusts the scroll offset so the cursor stays within the
// visible viewport.
func (m *refPickerModel) ensureVisible() {
	// Snapshot entries and "Working Tree" option are always visible above the list
	commitStart := m.commitStartCursor()
	if m.cursor < commitStart {
		m.offset = 0
		return
	}
	// entries use 0-based indexing relative to commitStart
	entryIdx := m.cursor - commitStart
	if entryIdx < m.offset {
		m.offset = entryIdx
	}
	vh := m.viewportHeight()
	if vh > 0 && entryIdx >= m.offset+vh {
		m.offset = entryIdx - vh + 1
	}
}
