package tui

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func editorFor(t *testing.T, ct types.CommentType) commentEditorModel {
	t.Helper()
	theme := DefaultTheme()
	m := commentEditorModel{theme: theme, width: 100, height: 30}
	m.open("a.go", 1, 1, types.TargetFile, ct)
	return m
}

// Tab stepping onto a type the selector cannot draw is what made `Q` and `A`
// look broken: the type was set, but nothing in the UI said so.
func TestEveryCommentTypeIsSelectable(t *testing.T) {
	t.Run("every type in the table is rendered as a label", func(t *testing.T) {
		out := stripANSISeq(editorFor(t, types.CommentIssue).View())
		for _, ct := range commentTypes {
			if !strings.Contains(out, ct.label) {
				t.Errorf("%q is missing from the type selector:\n%s", ct.label, out)
			}
		}
	})

	t.Run("the table covers every declared comment type", func(t *testing.T) {
		declared := []types.CommentType{
			types.CommentIssue, types.CommentSuggestion, types.CommentQuestion,
			types.CommentAnswer, types.CommentNote, types.CommentPraise,
		}
		in := map[types.CommentType]bool{}
		for _, ct := range commentTypes {
			in[ct.kind] = true
		}
		for _, d := range declared {
			if !in[d] {
				t.Errorf("comment type %q exists but the editor never offers it", d)
			}
		}
	})

	t.Run("Tab visits every type and returns to the first", func(t *testing.T) {
		seen := map[types.CommentType]bool{}
		cur := commentTypes[0].kind
		for range commentTypes {
			seen[cur] = true
			cur = nextCommentType(cur)
		}
		if cur != commentTypes[0].kind {
			t.Errorf("the cycle should wrap to %q, got %q", commentTypes[0].kind, cur)
		}
		for _, ct := range commentTypes {
			if !seen[ct.kind] {
				t.Errorf("Tab never reaches %q", ct.kind)
			}
		}
	})

	t.Run("the selected type is the one highlighted", func(t *testing.T) {
		for _, ct := range commentTypes {
			m := editorFor(t, ct.kind)
			if m.commentType != ct.kind {
				t.Errorf("opening with %q gave %q", ct.kind, m.commentType)
			}
			if !strings.Contains(stripANSISeq(m.View()), ct.label) {
				t.Errorf("%q not shown when selected", ct.label)
			}
		}
	})

	// Click targets are derived from the same table, so a label's column and
	// what clicking it selects cannot drift apart.
	t.Run("clicking each label selects it", func(t *testing.T) {
		x := 0
		for _, want := range commentTypes {
			m := editorFor(t, types.CommentIssue)
			if !m.handleClick(x, 4) {
				t.Errorf("click on %s at x=%d should be handled", want.label, x)
			}
			if m.commentType != want.kind {
				t.Errorf("click at x=%d selected %q, want %q", x, m.commentType, want.kind)
			}
			x += len(want.label) + 2 + 1 // padding(0,1) + separator
		}
	})
}
