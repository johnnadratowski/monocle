package tui

import (
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
		m.lines[i].summaryItemIDs = nil
	}
	if len(m.summaryItems) == 0 || m.path == "" {
		return
	}
	// Whole-file mode is a single hunk spanning the file, so hunk granularity
	// would paint the entire file one colour. It shows lines, so it tags lines.
	if m.fullFile || m.contentMode || m.style == diffStyleFile {
		m.tagSummaryLines()
		return
	}

	// The compact diff shows hunks, so it tags hunks: a hunk is the unit the
	// agent is asked to tag and the unit the reviewer moves in, and tagging its
	// context lines individually would fragment it under a filter.
	//
	// Walk the rows in order, carrying the current hunk's item forward. Rows
	// between hunk headers all belong to the same hunk, including the comment and
	// annotation boxes attached to them.
	var ids []string
	for i, ln := range m.lines {
		if ln.isHunk {
			ids = m.itemsForHunk(i)
		}
		m.lines[i].summaryItemIDs = ids
	}
}

// tagSummaryLines tags each row by its own new-file line. Rows with no new-file
// number — removed lines — inherit from their neighbours: a removed line sits
// against the added line that replaced it, so the row after it is the right
// answer, and the row before it is the fallback at the end of a block.
func (m *diffViewModel) tagSummaryLines() {
	for i, ln := range m.lines {
		if ln.isHunk {
			continue
		}
		if n := newLineOf(ln); n > 0 {
			m.lines[i].summaryItemIDs = m.itemsForLine(n)
		}
	}
	for i := range m.lines {
		ln := m.lines[i]
		// Only rows that could not be asked directly borrow from a neighbour. A
		// row WITH a new-file number was asked and answered "no item"; filling it
		// anyway would bleed a tag outwards from every range boundary and paint
		// lines the agent never claimed.
		if ln.isHunk || len(ln.summaryItemIDs) > 0 || newLineOf(ln) > 0 {
			continue
		}
		if ids := m.neighbourTag(i); len(ids) > 0 {
			m.lines[i].summaryItemIDs = ids
		}
	}
}

// neighbourTag looks forward first, then back, for the tag of an adjacent row,
// stopping at a hunk boundary so a tag never leaks across one.
func (m diffViewModel) neighbourTag(i int) []string {
	for j := i + 1; j < len(m.lines) && !m.lines[j].isHunk; j++ {
		if newLineOf(m.lines[j]) > 0 {
			return m.lines[j].summaryItemIDs
		}
	}
	for j := i - 1; j >= 0 && !m.lines[j].isHunk; j-- {
		if newLineOf(m.lines[j]) > 0 {
			return m.lines[j].summaryItemIDs
		}
	}
	return nil
}

// targetHere reports whether a target names the thing currently on screen. An
// artifact is matched by its id, a file by its path — and never the other way
// round: in content mode m.path holds a synthetic name like "content.md", which
// a file target must not be allowed to match.
func (m diffViewModel) targetHere(t types.SummaryTarget) bool {
	if t.IsArtifact() {
		return m.contentID != "" && t.Artifact == m.contentID
	}
	return m.contentID == "" && m.path != "" && t.Path == m.path
}

// itemsForLine returns every item covering one line of what is on screen, in
// reading order. More than one is normal rather than a mistake: a single line of
// prose can carry two separate fixes, and both items own it.
func (m diffViewModel) itemsForLine(n int) []string {
	var ids []string
	for _, it := range m.summaryItems {
		for _, t := range it.Targets {
			if m.targetHere(t) && t.CoversLine(n) {
				ids = append(ids, it.ID)
				break
			}
		}
	}
	return ids
}

// itemsForHunk finds every summary item accounting for the hunk starting at the
// given row. An item claims the hunk when it covers any of the hunk's new-file
// lines, or when it claims the whole file.
func (m diffViewModel) itemsForHunk(start int) []string {
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
	var ids []string
	for _, it := range m.summaryItems {
		if m.itemClaims(it, lines) {
			ids = append(ids, it.ID)
		}
	}
	return ids
}

func (m diffViewModel) itemClaims(it types.SummaryItem, lines []int) bool {
	for _, t := range it.Targets {
		if !m.targetHere(t) {
			continue
		}
		// A whole-file target claims every hunk in it, which is the right default
		// for a change that IS the file.
		if t.WholeFile() {
			return true
		}
		for _, n := range lines {
			if t.CoversLine(n) {
				return true
			}
		}
	}
	return false
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

// summaryBarFor is the bar drawn in a row's gutter — a colour, plus whether more
// than one item claims the row. A nil colour means no item does. Colours are
// always on, selection or not: seeing the shape of a review before picking
// anything out of it is most of the value.
//
// With an item selected, a shared row shows THAT item's colour rather than the
// first claimant's. The selection is the question being asked, and answering it
// with another item's colour on a row the selected item owns reads as "this
// belongs to something else" — the opposite of the truth.
func (m diffViewModel) summaryBarFor(line diffViewLine) summaryBar {
	if len(line.summaryItemIDs) == 0 || line.isHunk {
		return summaryBar{}
	}
	id := line.summaryItemIDs[0]
	if m.activeSummaryID != "" && lineClaimedBy(line, m.activeSummaryID) {
		id = m.activeSummaryID
	}
	for i, it := range m.summaryItems {
		if it.ID == id {
			return summaryBar{color: summaryColor(i), shared: len(line.summaryItemIDs) > 1}
		}
	}
	return summaryBar{}
}

// lineClaimedBy reports whether one item accounts for a row.
func lineClaimedBy(line diffViewLine, id string) bool {
	for _, got := range line.summaryItemIDs {
		if got == id {
			return true
		}
	}
	return false
}

// outsideActiveSummary reports whether a row is claimed by no item the selection
// names — the rows to fade in whole-file mode, and to drop in the compact diff.
// A row two items share stays in both their filters: it is the second item's own
// evidence, and dropping it would hide the very change that item describes.
func (m diffViewModel) outsideActiveSummary(line diffViewLine) bool {
	if m.activeSummaryID == "" {
		return false
	}
	return !lineClaimedBy(line, m.activeSummaryID)
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

// summaryHidesEverything reports whether a selection has removed every row the
// pane would otherwise draw. Only asked while a filter is active, so an unfiltered
// view pays nothing for it.
func (m diffViewModel) summaryHidesEverything() bool {
	if m.activeSummaryID == "" || len(m.lines) == 0 {
		return false
	}
	for _, ln := range m.lines {
		if !m.isHiddenBySummary(ln) && !m.isHiddenComment(ln) {
			return false
		}
	}
	return true
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
	m.diffView.rebuild()
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
