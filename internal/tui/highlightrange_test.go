package tui

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

// renderedRow is how the diff draws the row showing new-file line n.
func renderedRow(m appModel, n int) string {
	dv := m.diffView
	for i, l := range dv.lines {
		if dv.lineNumAt(i) == n && !l.isHunk && !l.isComment && !l.isAnnotation {
			return dv.renderDiffLine(l, 10, dv.width-10, false, false)
		}
	}
	return ""
}

// A highlighted range tints the rows from its first line to its last, in its
// own file only, without moving the cursor or the scroll. It stays through a
// file switch until it is replaced or cleared.
func TestHighlightRangeTintsItsRows(t *testing.T) {
	m, _ := tourApp(t) // a.go on screen, cursor on line 5
	plain := map[int]string{}
	for _, n := range []int{9, 10, 12, 13} {
		plain[n] = renderedRow(m, n)
	}
	cursor, offset := m.diffView.cursor, m.diffView.offset
	m = updateApp(t, m, highlightRangeMsg{path: "a.go", start: 10, end: 12})
	if m.diffView.cursor != cursor || m.diffView.offset != offset {
		t.Fatal("highlighting moved the cursor or the scroll")
	}
	if renderedRow(m, 10) == plain[10] || renderedRow(m, 12) == plain[12] {
		t.Error("the range's rows are drawn as before")
	}
	if renderedRow(m, 9) != plain[9] || renderedRow(m, 13) != plain[13] {
		t.Error("rows outside the range changed")
	}
	if !m.diffView.inHighlight(lineRow(m, 11)) {
		t.Error("line 11, inside the range, is not highlighted")
	}

	// Another file shows nothing; back in a.go the range is still there.
	m = updateApp(t, m, gotoLineMsg{path: "b.go", line: 11})
	for i, l := range m.diffView.lines {
		if m.diffView.inHighlight(l) {
			t.Fatalf("b.go row %d is highlighted", i)
		}
	}
	m = updateApp(t, m, gotoLineMsg{path: "a.go", line: 30})
	if renderedRow(m, 11) == "" || !m.diffView.inHighlight(lineRow(m, 11)) {
		t.Error("the range did not survive a file switch")
	}

	// A new range replaces it; a clear removes it.
	m = updateApp(t, m, highlightRangeMsg{path: "a.go", start: 20, end: 21})
	if m.diffView.inHighlight(lineRow(m, 11)) || !m.diffView.inHighlight(lineRow(m, 20)) {
		t.Error("a new range did not replace the old one")
	}
	m = updateApp(t, m, highlightRangeMsg{})
	if m.diffView.inHighlight(lineRow(m, 20)) || renderedRow(m, 10) != plain[10] {
		t.Error("the clear left the range")
	}
}

// lineRow is the row showing new-file line n.
func lineRow(m appModel, n int) diffViewLine {
	for i, l := range m.diffView.lines {
		if m.diffView.lineNumAt(i) == n && !l.isHunk {
			return l
		}
	}
	return diffViewLine{}
}

// The tint is highlight_color when it is set, else the theme's.
func TestHighlightColor(t *testing.T) {
	m, _ := tourAppWith(t, testTour(), &types.Config{})
	m = updateApp(t, m, highlightRangeMsg{path: "a.go", start: 10, end: 12})
	if bg, _ := m.diffView.rowBg(lineRow(m, 10), types.DiffLineAdded); bg != m.theme.HighlightBg {
		t.Errorf("tint %v, want the theme's %v", bg, m.theme.HighlightBg)
	}
	if bg, _ := m.diffView.rowBg(lineRow(m, 9), types.DiffLineAdded); bg != m.theme.AddedBg {
		t.Errorf("a row outside the range has %v, want the added background", bg)
	}
	m.engine.GetConfig().HighlightColor = "#123456"
	m = updateApp(t, m, highlightRangeMsg{path: "a.go", start: 10, end: 12})
	if bg, _ := m.diffView.rowBg(lineRow(m, 10), types.DiffLineAdded); bg != lipgloss.Color("#123456") {
		t.Errorf("tint %v, want highlight_color #123456", bg)
	}
	if DefaultTheme().HighlightBg == nil || LightTheme().HighlightBg == nil {
		t.Error("a theme has no highlight colour")
	}
}

// The tint shows in the whole-file view too, and in the split view.
func TestHighlightShowsInEveryView(t *testing.T) {
	m, _ := tourApp(t)
	row := lineRow(m, 10)
	file := func(dv diffViewModel) string { return dv.renderContentLine(row, 4, dv.width-4, false, false) }
	split := func(dv diffViewModel) string {
		return dv.renderSplitSide("  10 ", row.content, types.DiffLineAdded, false, nil, 5, 40, row, false, false)
	}
	plainFile, plainSplit := file(m.diffView), split(m.diffView)
	m = updateApp(t, m, highlightRangeMsg{path: "a.go", start: 10, end: 12})
	if file(m.diffView) == plainFile {
		t.Error("the whole-file view does not show the highlight")
	}
	if split(m.diffView) == plainSplit {
		t.Error("the split view does not show the highlight")
	}
}
