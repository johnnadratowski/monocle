package tui

import (
	"image/color"

	"github.com/josephschmitt/monocle/internal/types"
)

// Summary items tag hunks, not lines. The agent supplies new-file ranges, but a
// removed line has no new-file number, so matching per line would tag the "after"
// of an edit and not the "before" — leaving half of every change untagged. A
// hunk is also the unit a reviewer moves in, which is what the tagging is for.

// tagSummaryHunks records, for every built row, which summary item accounts for
// the hunk it belongs to. Run once per build so the renderers can answer from
// the row alone.
func (m *diffViewModel) tagSummaryHunks() {
	for i := range m.lines {
		m.lines[i].summaryItemID = ""
	}
	if len(m.summaryItems) == 0 || m.path == "" {
		return
	}
	// Walk the rows in order, carrying the current hunk's item forward. Rows
	// between hunk headers all belong to the same hunk, including the comment and
	// annotation boxes attached to them.
	id := ""
	for i, ln := range m.lines {
		if ln.isHunk {
			id = m.itemForHunk(i)
		}
		m.lines[i].summaryItemID = id
	}
}

// itemForHunk finds the summary item accounting for the hunk starting at the
// given row. An item claims the hunk when it covers any of the hunk's new-file
// lines, or when it claims the whole file.
func (m diffViewModel) itemForHunk(start int) string {
	// Collect the new-file lines this hunk shows, stopping at the next header.
	var lines []int
	for i := start + 1; i < len(m.lines); i++ {
		ln := m.lines[i]
		if ln.isHunk {
			break
		}
		if n := newLineOf(ln); n > 0 {
			lines = append(lines, n)
		}
	}
	for _, it := range m.summaryItems {
		if m.itemClaims(it, lines) {
			return it.ID
		}
	}
	return ""
}

func (m diffViewModel) itemClaims(it types.SummaryItem, lines []int) bool {
	for _, t := range it.Targets {
		if t.Path != m.path {
			continue
		}
		// A whole-file target claims every hunk in it, which is the right default
		// for a change that IS the file.
		if t.WholeFile() {
			return true
		}
		for _, n := range lines {
			if t.Covers(m.path, n) {
				return true
			}
		}
	}
	return false
}

// newLineOf returns a row's new-file line number, taking the right side in split
// mode where that is where the new file lives.
func newLineOf(ln diffViewLine) int {
	if ln.rightLineNum > 0 {
		return ln.rightLineNum
	}
	return ln.newLineNum
}

// activeSummaryIndex is the position of the item currently filtering the view,
// or -1 when nothing is selected.
func (m diffViewModel) activeSummaryIndex() int {
	if m.activeSummaryID == "" {
		return -1
	}
	for i, it := range m.summaryItems {
		if it.ID == m.activeSummaryID {
			return i
		}
	}
	return -1
}

// summaryColorFor is the colour of the bar drawn in a row's gutter, or nil when
// the row belongs to no item. Colours are always on, selection or not: seeing
// the shape of a review before picking anything out of it is most of the value.
func (m diffViewModel) summaryColorFor(line diffViewLine) color.Color {
	if line.summaryItemID == "" || line.isHunk {
		return nil
	}
	for i, it := range m.summaryItems {
		if it.ID == line.summaryItemID {
			return summaryColor(i)
		}
	}
	return nil
}

// outsideActiveSummary reports whether a row belongs to a different item than
// the one selected — the rows to fade in whole-file mode, and to drop in the
// compact diff.
func (m diffViewModel) outsideActiveSummary(line diffViewLine) bool {
	if m.activeSummaryID == "" {
		return false
	}
	return line.summaryItemID != m.activeSummaryID
}

// With an item selected, the rest of the review is either dropped or faded, and
// which one depends on the view. The compact diff is already a filtered view of
// the file, so filtering it further is in keeping: show that item's hunks and
// nothing else. Whole-file mode exists to show the file as it is, so removing
// parts of it there would defeat the point — the other hunks stay, greyed, and
// the reviewer keeps their bearings.

// isHiddenBySummary reports whether a row is dropped from the compact diff
// because it belongs to a different item than the selected one.
func (m diffViewModel) isHiddenBySummary(line diffViewLine) bool {
	if m.fullFile || m.contentMode || m.style == diffStyleFile {
		return false
	}
	return m.outsideActiveSummary(line)
}

// isFadedBySummary reports whether a row is greyed because it belongs to a
// different item than the selected one. Only in the views that keep it.
func (m diffViewModel) isFadedBySummary(line diffViewLine) bool {
	if m.isHiddenBySummary(line) {
		return false // it is not on screen at all
	}
	return m.outsideActiveSummary(line)
}

// faded is the single question the renderers ask: should this row be drawn
// greyed rather than styled? Two independent reasons converge here — the
// source-comment filter and a summary selection — and a renderer that asked only
// one of them would contradict the other.
func (m diffViewModel) faded(line diffViewLine) bool {
	return m.isDimmedComment(line) || m.isFadedBySummary(line)
}

// --- app-level plumbing ---

// setSummaryItems takes a new summary from the engine and keeps every view that
// answers to it in step. A selection that no longer names an existing item is
// dropped: the agent re-sending a different set must not leave the review
// filtered to something that is gone, showing nothing and explaining nothing.
func (m *appModel) setSummaryItems(items []types.SummaryItem) {
	m.summaryItems = items
	if m.activeSummaryID != "" {
		found := false
		for _, it := range items {
			if it.ID == m.activeSummaryID {
				found = true
				break
			}
		}
		if !found {
			m.activeSummaryID = ""
		}
	}
	m.diffView.summaryItems = items
	m.diffView.activeSummaryID = m.activeSummaryID
	m.summaryModal.activeID = m.activeSummaryID
	m.sidebar.summaryItems = items
	m.sidebar.activeSummaryID = m.activeSummaryID
}

// selectSummaryItem filters the review to one item, or clears the filter when
// given an empty id.
func (m *appModel) selectSummaryItem(id string) {
	m.activeSummaryID = id
	m.diffView.activeSummaryID = id
	m.summaryModal.activeID = id
	m.sidebar.activeSummaryID = id
	// The line list itself changes shape in the compact diff, so it has to be
	// rebuilt rather than merely redrawn.
	m.diffView.buildLines()
	m.diffView.cursor = m.diffView.nearestSelectable(m.diffView.cursor, 1)
	m.diffView.ensureVisible()
}

// activeSummaryItem returns the item currently filtering the review, and its
// position (which is what picks its colour), or nil.
func (m appModel) activeSummaryItem() (*types.SummaryItem, int) {
	if m.activeSummaryID == "" {
		return nil, -1
	}
	for i := range m.summaryItems {
		if m.summaryItems[i].ID == m.activeSummaryID {
			return &m.summaryItems[i], i
		}
	}
	return nil, -1
}
