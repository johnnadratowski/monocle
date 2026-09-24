package tui

import (
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

// partialBlockComment is the reported shape: a hunk that shows the MIDDLE of a
// doc comment. The /** that opened it is above the hunk window, so the fragment
// on its own lexes as ordinary text.
func partialBlockComment() types.DiffHunk {
	return types.DiffHunk{
		OldStart: 4, OldCount: 4, NewStart: 4, NewCount: 4,
		Lines: []types.DiffLine{
			{Kind: types.DiffLineContext, Content: " * Retire submissions superseded by a newer row.", OldLineNum: 4, NewLineNum: 4},
			{Kind: types.DiffLineRemoved, Content: " * Old wording.", OldLineNum: 5},
			{Kind: types.DiffLineAdded, Content: " * New wording.", NewLineNum: 5},
			{Kind: types.DiffLineContext, Content: " * Carried rows keep their id.", OldLineNum: 6, NewLineNum: 6},
		},
	}
}

const fullSource = `const a = 1
/**
 * Retire submissions superseded by a newer row.
 * New wording.
 * Carried rows keep their id.
 */
async function retire() {}
`

func sourceModel(t *testing.T) diffViewModel {
	t.Helper()
	th := DefaultTheme()
	km := DefaultKeyMap()
	m := newDiffViewModel(&th, &km)
	m.width, m.height = 120, 40
	m.path = "x.ts"
	m.commentFilter = commentsDimmed
	m.hunks = []types.DiffHunk{partialBlockComment()}
	m.buildLines()
	return m
}

// TestPartialBlockCommentNeedsTheSource states the problem: without the file,
// the fragment cannot be classified, because nothing in it says it is already
// inside a comment.
func TestPartialBlockCommentNeedsTheSource(t *testing.T) {
	m := sourceModel(t)
	if !m.needsCommentSource() {
		t.Error("a compact diff with the filter on should want the full file")
	}
	dimmed := 0
	for _, ln := range m.lines {
		if !ln.isHunk && m.isDimmedComment(ln) {
			dimmed++
		}
	}
	if dimmed != 0 {
		t.Logf("fragment alone classified %d lines (implementation may improve this)", dimmed)
	}
}

// With the file in hand every line of the block classifies, however little of it
// the diff shows.
func TestFullSourceClassifiesAPartialBlock(t *testing.T) {
	m := sourceModel(t)
	if !m.setCommentSource("x.ts", fullSource) {
		t.Fatal("the source should have been accepted for the current path")
	}
	if m.needsCommentSource() {
		t.Error("the source is loaded; it should not be asked for again")
	}

	// Every row that exists in the new file — which is what the source describes.
	// Removed lines are the documented gap; see the test below.
	checked := 0
	for _, ln := range m.lines {
		if ln.isHunk || newLineOf(ln) == 0 {
			continue
		}
		checked++
		if !m.isDimmedComment(ln) {
			t.Errorf("not dimmed (kind=%s old=%d new=%d): %q",
				ln.kind, ln.oldLineNum, ln.newLineNum, ln.content)
		}
	}
	if checked != 3 {
		t.Fatalf("checked %d rows, want the three with new-file numbers", checked)
	}
}

// A removed line inside a block the diff only partly shows is a known gap: the
// old file is never fetched, and guessing from its neighbours dims deleted code.
// Pinned so the trade-off is deliberate rather than forgotten.
func TestRemovedLineInsideAPartialBlockIsNotClassified(t *testing.T) {
	m := sourceModel(t)
	m.setCommentSource("x.ts", fullSource)
	for _, ln := range m.lines {
		if ln.kind != types.DiffLineRemoved {
			continue
		}
		if m.isDimmedComment(ln) {
			t.Errorf("unexpected: %q classified — if this now works, update the note", ln.content)
		}
	}
}

// Deleted code between two adjacent comments must never be dimmed — that is the
// error the dropped bracketing heuristic made.
func TestDeletedCodeIsNeverDimmed(t *testing.T) {
	th := DefaultTheme()
	km := DefaultKeyMap()
	m := newDiffViewModel(&th, &km)
	m.width, m.height = 120, 40
	m.path = "x.ts"
	m.commentFilter = commentsDimmed
	m.hunks = []types.DiffHunk{{
		OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 3,
		Lines: []types.DiffLine{
			{Kind: types.DiffLineContext, Content: "// a comment", OldLineNum: 1, NewLineNum: 1},
			{Kind: types.DiffLineRemoved, Content: "const gone = 1", OldLineNum: 2},
			{Kind: types.DiffLineContext, Content: "// another comment", OldLineNum: 3, NewLineNum: 2},
		},
	}}
	m.buildLines()
	m.setCommentSource("x.ts", "// a comment\n// another comment\n")

	for _, ln := range m.lines {
		if ln.kind == types.DiffLineRemoved && m.isDimmedComment(ln) {
			t.Errorf("a line of code between two comments must not be dimmed: %q", ln.content)
		}
	}
}

// A source that arrives after the reviewer has moved on belongs to another file.
func TestStaleSourceIsDropped(t *testing.T) {
	m := sourceModel(t)
	if m.setCommentSource("other.ts", fullSource) {
		t.Error("a source for a different path must be refused")
	}
	if m.commentSource != "" {
		t.Error("the stale source should not have been stored")
	}
}

// Whole-file mode already displays every line, so its own reconstruction is the
// file and there is nothing to fetch.
func TestWholeFileModeNeedsNoSource(t *testing.T) {
	m := sourceModel(t)
	m.fullFile = true
	if m.needsCommentSource() {
		t.Error("whole-file mode shows the file already")
	}
	m.fullFile = false
	m.commentFilter = commentsShown
	if m.needsCommentSource() {
		t.Error("with the filter off there is nothing to classify")
	}
}
