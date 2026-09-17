package tui

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func commentModel(t *testing.T, style diffStyle) diffViewModel {
	t.Helper()
	th := DefaultTheme()
	km := DefaultKeyMap()
	m := newDiffViewModel(&th, &km)
	// Narrow enough that an expanded body wraps to several rows; a collapsed one
	// is always a three-line box.
	m.width, m.height = 60, 40
	m.path = "main.go"
	m.style = style
	// A delay of -1 disables cursor auto-expand, so the tests measure the
	// explicit toggles rather than the hover behaviour.
	m.commentExpandDelay = -1
	m.comments = []types.ReviewComment{
		// Bodies long enough that expanding them takes more rows than the
		// three-line collapsed box.
		{ID: "c1", Type: types.CommentIssue, TargetRef: "main.go", LineStart: 1, LineEnd: 1,
			Body: "the first comment, written at enough length that its expanded box needs several rows to hold the text while the collapsed box stays three lines tall"},
		{ID: "c2", Type: types.CommentNote, TargetRef: "main.go", LineStart: 2, LineEnd: 2,
			Body: "the second comment, also written at enough length to make the difference between collapsed and expanded plain in the row count"},
	}
	m.hunks = []types.DiffHunk{{
		OldStart: 1, OldCount: 1, NewStart: 1, NewCount: 2,
		Lines: []types.DiffLine{
			{Kind: types.DiffLineContext, Content: "package main", OldLineNum: 1, NewLineNum: 1},
			{Kind: types.DiffLineAdded, Content: "func main() {}", NewLineNum: 2},
		},
	}}
	m.buildLines()
	return m
}

// rows counts the screen rows every line occupies, which is what tells an
// expanded comment from a collapsed one.
func rows(m diffViewModel) int {
	n := 0
	for i := range m.lines {
		n += m.screenLinesFor(i)
	}
	return n
}

// TestExpandAllComments covers the new whole-file toggle. Reading a review's
// conversation before deciding is a different task from glancing at the comment
// under the cursor, and doing it one comment at a time loses the thread.
func TestExpandAllComments(t *testing.T) {
	for _, style := range []struct {
		name  string
		style diffStyle
	}{
		{"unified", diffStyleUnified},
		{"split", diffStyleSplit},
		{"file", diffStyleFile},
	} {
		t.Run(style.name, func(t *testing.T) {
			m := commentModel(t, style.style)
			collapsed := rows(m)

			m.expandAllComments = true
			expandedAll := rows(m)
			if expandedAll <= collapsed {
				t.Fatalf("expanding all took %d rows, collapsed took %d — nothing expanded", expandedAll, collapsed)
			}

			// Both comments, not just one: expanding a single comment must not
			// reach the same height, or the toggle is doing half its job.
			m.expandAllComments = false
			m.expandedCommentID = "c1"
			expandedOne := rows(m)
			if expandedOne >= expandedAll {
				t.Errorf("one comment took %d rows and all took %d; all should be taller", expandedOne, expandedAll)
			}
		})
	}
}

// The render and the row-height calculation must agree, or the cursor drifts
// away from what is actually drawn.
func TestExpandAllMatchesWhatIsDrawn(t *testing.T) {
	m := commentModel(t, diffStyleUnified)
	m.expandAllComments = true
	m.focused = true

	for i, line := range m.lines {
		if !line.isComment {
			continue
		}
		drawn := strings.Count(m.renderCommentLine(line, false), "\n") + 1
		if counted := m.screenLinesFor(i); counted != drawn {
			t.Errorf("line %d: screenLinesFor says %d rows, the renderer drew %d", i, counted, drawn)
		}
	}
}

// Turning the whole-file toggle off leaves the cursor's own comment where the
// reader left it, rather than collapsing everything indiscriminately.
func TestExpandAllDoesNotClobberTheSingleToggle(t *testing.T) {
	m := commentModel(t, diffStyleUnified)
	m.expandedCommentID = "c2"

	m.expandAllComments = true
	if !m.commentExpanded(&m.comments[0]) {
		t.Error("c1 should be expanded while the all-toggle is on")
	}

	m.expandAllComments = false
	if m.commentExpanded(&m.comments[0]) {
		t.Error("c1 should collapse again when the all-toggle goes off")
	}
	if !m.commentExpanded(&m.comments[1]) {
		t.Error("c2 was expanded before the all-toggle and should still be")
	}
}

func TestCommentExpandedHandlesNil(t *testing.T) {
	m := commentModel(t, diffStyleUnified)
	m.expandAllComments = true
	if m.commentExpanded(nil) {
		t.Error("a line with no comment is never expanded")
	}
}
