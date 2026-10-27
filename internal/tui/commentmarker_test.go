package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/josephschmitt/monocle/internal/types"
)

// A block of comment lines, a single comment line, and a doc comment whose body
// the diff rewrote: the shapes the hide state collapses (John 2026-10-27: "I also
// want to be able to hide comments from the regions … a global thing in tour").
func markerHunk() types.DiffHunk {
	return types.DiffHunk{
		OldStart: 1, OldCount: 9, NewStart: 1, NewCount: 9,
		Lines: []types.DiffLine{
			{Kind: types.DiffLineContext, Content: "const a = 1", OldLineNum: 1, NewLineNum: 1},
			{Kind: types.DiffLineContext, Content: "// one", OldLineNum: 2, NewLineNum: 2},
			{Kind: types.DiffLineContext, Content: "// two", OldLineNum: 3, NewLineNum: 3},
			{Kind: types.DiffLineContext, Content: "// three", OldLineNum: 4, NewLineNum: 4},
			{Kind: types.DiffLineContext, Content: "const b = 2", OldLineNum: 5, NewLineNum: 5},
			{Kind: types.DiffLineContext, Content: "// alone", OldLineNum: 6, NewLineNum: 6},
			{Kind: types.DiffLineContext, Content: "const c = 3", OldLineNum: 7, NewLineNum: 7},
			{Kind: types.DiffLineRemoved, Content: "// old words", OldLineNum: 8},
			{Kind: types.DiffLineAdded, Content: "// new words", NewLineNum: 8},
			{Kind: types.DiffLineAdded, Content: "// more words", NewLineNum: 9},
			{Kind: types.DiffLineContext, Content: "const d = 4", OldLineNum: 9, NewLineNum: 10},
		},
	}
}

func markerModel(t *testing.T) diffViewModel {
	t.Helper()
	m := dimModel(t, diffStyleUnified, markerHunk())
	m.setCommentFilter(commentsHidden)
	return m
}

func TestHiddenCommentsCollapseToOneMarkerRow(t *testing.T) {
	m := markerModel(t)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "3 comment lines hidden") {
		t.Errorf("the run of three comment lines is not one marker row:\n%s", view)
	}
	if strings.Contains(view, "// one") || strings.Contains(view, "// alone") || strings.Contains(view, "// new words") {
		t.Errorf("a hidden comment line is still drawn:\n%s", view)
	}
	if strings.Contains(view, "1 comment lines hidden") || strings.Contains(view, "1 comment line hidden") {
		t.Errorf("a single comment line got a marker; it is only hidden (the explorer does the same):\n%s", view)
	}
	for _, code := range []string{"const a = 1", "const b = 2", "const c = 3", "const d = 4"} {
		if !strings.Contains(view, code) {
			t.Errorf("code line %q is gone:\n%s", code, view)
		}
	}
}

func TestAMarkerCountsTheChangedCommentLines(t *testing.T) {
	view := ansi.Strip(markerModel(t).View())
	if !strings.Contains(view, "3 comment lines hidden") || !strings.Contains(view, "+2") || !strings.Contains(view, "−1") {
		t.Errorf("the rewritten comment's marker does not say what changed (+2 −1):\n%s", view)
	}
}

// A marker is one row of the view: the scroll math counts it, and the cursor never rests on it.
func TestAMarkerIsOneRowAndNeverTheCursor(t *testing.T) {
	m := markerModel(t)
	rows, markers := 0, 0
	for i := range m.lines {
		rows += m.screenLinesFor(i)
		if _, ok := m.commentMarkerAt(i); ok {
			markers++
			if m.isSelectable(i) {
				t.Errorf("the cursor can rest on the marker at %d", i)
			}
			if m.screenLinesFor(i) != 1 || m.displayLinesFor(i) != 1 {
				t.Errorf("the marker at %d is not one row: screen %d, display %d", i, m.screenLinesFor(i), m.displayLinesFor(i))
			}
		}
	}
	if markers != 2 {
		t.Errorf("markers = %d, want 2 (the block of three, the rewritten comment)", markers)
	}
	drawn := strings.Count(m.View(), "\n") + 1
	if rows > m.height || drawn < rows {
		t.Errorf("rows counted %d, drawn %d (height %d)", rows, drawn, m.height)
	}
	// Real line numbers: the code after the hidden block keeps its own.
	for i, ln := range m.lines {
		if ln.content == "const b = 2" && ln.newLineNum != 5 {
			t.Errorf("const b = 2 is line %d, want 5 (row %d)", ln.newLineNum, i)
		}
	}
}

// Shown and dimmed have no markers.
func TestNoMarkerUnlessHidden(t *testing.T) {
	for _, mode := range []commentFilterMode{commentsShown, commentsDimmed} {
		m := dimModel(t, diffStyleUnified, markerHunk())
		m.setCommentFilter(mode)
		if strings.Contains(ansi.Strip(m.View()), "comment lines hidden") {
			t.Errorf("mode %d draws a marker", mode)
		}
	}
}
