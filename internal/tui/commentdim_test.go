package tui

import (
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func dimModel(t *testing.T, style diffStyle, hunk types.DiffHunk) diffViewModel {
	t.Helper()
	th := DefaultTheme()
	km := DefaultKeyMap()
	m := newDiffViewModel(&th, &km)
	m.width, m.height = 120, 40
	m.path = "x.ts"
	m.style = style
	m.commentFilter = commentsDimmed
	m.hunks = []types.DiffHunk{hunk}
	m.buildLines()
	return m
}

// editedDocComment is the shape that exposed the bug: rewriting a doc comment's
// text leaves the /** and */ fences untouched and replaces the body, so the body
// arrives as removed+added lines while the fences stay context.
func editedDocComment() types.DiffHunk {
	return types.DiffHunk{
		OldStart: 1, OldCount: 6, NewStart: 1, NewCount: 6,
		Lines: []types.DiffLine{
			{Kind: types.DiffLineContext, Content: "const a = 1", OldLineNum: 1, NewLineNum: 1},
			{Kind: types.DiffLineContext, Content: "/**", OldLineNum: 2, NewLineNum: 2},
			{Kind: types.DiffLineRemoved, Content: " * Retire old submissions.", OldLineNum: 3},
			{Kind: types.DiffLineRemoved, Content: " * Old second line.", OldLineNum: 4},
			{Kind: types.DiffLineAdded, Content: " * Retire submissions superseded by a newer row.", NewLineNum: 3},
			{Kind: types.DiffLineAdded, Content: " * Carried rows keep their id.", NewLineNum: 4},
			{Kind: types.DiffLineContext, Content: " */", OldLineNum: 5, NewLineNum: 5},
			{Kind: types.DiffLineContext, Content: "async function retire() {}", OldLineNum: 6, NewLineNum: 6},
		},
	}
}

// TestRemovedCommentLinesDim covers the reported symptom: only the top and
// bottom of a block comment greyed. The fences were context lines, which have a
// new-file number; the body was removed, which has none — and the classifier
// only ever looked at the new side.
func TestRemovedCommentLinesDim(t *testing.T) {
	m := dimModel(t, diffStyleUnified, editedDocComment())

	var checked int
	for _, line := range m.lines {
		if line.isHunk {
			continue
		}
		isComment := line.content == "/**" || line.content == " */" ||
			(len(line.content) > 2 && line.content[:3] == " * ")
		if !isComment {
			if m.isDimmedComment(line) {
				t.Errorf("code line should not be dimmed: %q", line.content)
			}
			continue
		}
		checked++
		if !m.isDimmedComment(line) {
			t.Errorf("comment line not dimmed (kind=%s old=%d new=%d): %q",
				line.kind, line.oldLineNum, line.newLineNum, line.content)
		}
	}
	if checked != 6 {
		t.Fatalf("checked %d comment lines, want all 6 of the block", checked)
	}
}

// Split view puts the old side on the left, so the same classification has to
// reach it there too.
func TestRemovedCommentLinesDimInSplit(t *testing.T) {
	m := dimModel(t, diffStyleSplit, editedDocComment())
	var dimmedRemoved int
	for _, line := range m.lines {
		if line.isHunk || line.kind != types.DiffLineRemoved {
			continue
		}
		if m.isDimmedComment(line) {
			dimmedRemoved++
		}
	}
	if dimmedRemoved == 0 {
		t.Error("no removed comment line dimmed in split view")
	}
}

// Hiding runs off the same classification, so a removed comment line has to
// disappear with the rest of its block rather than being left stranded.
func TestRemovedCommentLinesHide(t *testing.T) {
	m := dimModel(t, diffStyleUnified, editedDocComment())
	m.commentFilter = commentsHidden
	m.buildLines()
	for _, line := range m.lines {
		if line.isHunk {
			continue
		}
		if line.kind == types.DiffLineRemoved && m.isHiddenComment(line) {
			continue
		}
		if len(line.content) > 2 && line.content[:3] == " * " && !m.isHiddenComment(line) {
			t.Errorf("comment body still visible when hidden: %q", line.content)
		}
	}
}

// Code must never be caught by either side's classification — the filter is only
// allowed to touch lines that are entirely comment.
func TestCodeIsNeverDimmed(t *testing.T) {
	m := dimModel(t, diffStyleUnified, types.DiffHunk{
		OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 3,
		Lines: []types.DiffLine{
			{Kind: types.DiffLineRemoved, Content: "const a = 1 // trailing comment", OldLineNum: 1},
			{Kind: types.DiffLineAdded, Content: "const a = 2 // trailing comment", NewLineNum: 1},
			{Kind: types.DiffLineContext, Content: "const b = 3", OldLineNum: 2, NewLineNum: 2},
		},
	})
	for _, line := range m.lines {
		if line.isHunk {
			continue
		}
		if m.isDimmedComment(line) {
			t.Errorf("a line with code on it must not dim: %q", line.content)
		}
	}
}
