package tui

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

// TestVisibleControls covers the display half of the NUL bug: even once the file
// is no longer misfiled as binary, a raw control byte draws as nothing, so the
// before and after lines of the reported diff would look identical.
func TestVisibleControls(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text is untouched", "func main() {", "func main() {"},
		{"tabs survive", "\tindented", "\tindented"},
		{"CRLF survives", "line\r", "line\r"},
		{"NUL becomes a glyph", "a\x00b", "a␀b"},
		{"ESC becomes a glyph", "\x1b[31m", "␛[31m"},
		{"DEL becomes a glyph", "a\x7fb", "a␡b"},
		{"the glyph names the byte", "\x01\x02\x1f", "␁␂␟"},
		{"UTF-8 is not mangled", "héllo → 世界", "héllo → 世界"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := visibleControls(c.in); got != c.want {
				t.Errorf("visibleControls(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// One glyph per byte, so every column offset downstream — intra-line change
// spans, comment anchors, the cursor — still points at the character it did.
func TestVisibleControlsKeepsColumns(t *testing.T) {
	in := "ab\x00cd\x01ef"
	got := visibleControls(in)
	if n := len([]rune(got)); n != len(in) {
		t.Errorf("rune count = %d, want %d (one glyph per byte)", n, len(in))
	}
	if idx := strings.IndexRune(got, '␀'); idx != len("ab") {
		t.Errorf("the NUL glyph is at byte %d; it should still be the third character", idx)
	}
}

// An escape sequence inside reviewed content must not reach the terminal as a
// command. Sanitizing at ingestion is what stops a diff from repainting the UI.
func TestVisibleControlsDefangsEscapes(t *testing.T) {
	got := visibleControls("before\x1b[2J\x1b[Hafter")
	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("an ESC survived sanitizing: %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Errorf("surrounding text should be intact: %q", got)
	}
}

func TestVisibleControlHunks(t *testing.T) {
	in := []types.DiffHunk{{Lines: []types.DiffLine{
		{Content: "clean line"},
		{Content: "dirty\x00line"},
	}}}
	out := visibleControlHunks(in)

	if out[0].Lines[1].Content != "dirty␀line" {
		t.Errorf("line not sanitized: %q", out[0].Lines[1].Content)
	}
	// The engine's copy is what feedback quotes back to the agent, so it has to
	// stay byte-faithful.
	if in[0].Lines[1].Content != "dirty\x00line" {
		t.Errorf("the input was mutated: %q", in[0].Lines[1].Content)
	}
}

// TestNulInSourceFileRendersAsADiff is the reported bug end to end: a TypeScript
// file whose entire change is replacing two raw NULs with the two-character \0
// escape. git diffs it as text, so Monocle must show it — with the NUL visible,
// since it is the thing being reviewed.
func TestNulInSourceFileRendersAsADiff(t *testing.T) {
	const before = "  const replaced = new Set(staged.map((s) => `${s.document_kind}\x00${s.person_label ?? ''}`))"
	const after = "  const replaced = new Set(staged.map((s) => `${s.document_kind}\\0${s.person_label ?? ''}`))"

	hunks := []types.DiffHunk{{
		OldStart: 306, NewStart: 306,
		Lines: []types.DiffLine{
			{Kind: types.DiffLineContext, Content: "  const carried = business.brale_doc_submissions_unlinked ?? []", OldLineNum: 306, NewLineNum: 306},
			{Kind: types.DiffLineRemoved, Content: before, OldLineNum: 309},
			{Kind: types.DiffLineAdded, Content: after, NewLineNum: 309},
		},
	}}

	if isBinaryContent(hunks) {
		t.Fatal("a TypeScript diff git renders as text must not be filed as binary")
	}

	m := diffViewModel{width: 200, height: 40, path: "server/models/v1/braleDocumentStaging.ts"}
	m.hunks = visibleControlHunks(hunks)
	m.buildLines()

	var removed string
	for _, l := range m.lines {
		if l.kind == types.DiffLineRemoved {
			removed = l.content
		}
	}
	if removed == "" {
		t.Fatal("the removed line is missing from the rendered diff")
	}
	if strings.ContainsRune(removed, 0) {
		t.Error("a raw NUL reached the render path")
	}
	if !strings.ContainsRune(removed, '␀') {
		t.Errorf("the NUL should be visible on the before side, got: %q", removed)
	}
}
