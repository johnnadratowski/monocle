package tui

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestDocPaneOpenAndHighlight(t *testing.T) {
	theme := DefaultTheme()
	m := docPaneModel{width: 60, height: 10, theme: &theme}
	refs := []types.DocRef{
		{Kind: types.DocRefFile, Doc: "TODO.md", Label: "a", StartLine: 3, EndLine: 4},
		{Kind: types.DocRefFile, Doc: "DESIGN.md", Label: "b", StartLine: 1, EndLine: 1},
	}
	m.openRefs(refs)
	if !m.active || m.activeRef != 0 {
		t.Fatalf("openRefs: active=%v activeRef=%d", m.active, m.activeRef)
	}

	content := "line1\nline2\nTARGET-A\nTARGET-A2\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12"
	m.setContent("TODO.md", content, refs[0])
	out := m.View()
	if !strings.Contains(out, "TARGET-A") {
		t.Error("doc pane should show the highlighted lines")
	}
	if !strings.Contains(out, "TODO.md") {
		t.Error("doc pane should show the title")
	}
	if !strings.Contains(out, "ref 1/2") {
		t.Error("title should show ref index when multiple refs")
	}

	r, ok := m.nextRef()
	if !ok || m.activeRef != 1 || r.Doc != "DESIGN.md" {
		t.Errorf("nextRef: ok=%v activeRef=%d doc=%s", ok, m.activeRef, r.Doc)
	}

	m.setContent("short.md", "only one line", types.DocRef{StartLine: 99, EndLine: 100})
	if !m.rangeShifted {
		t.Error("expected rangeShifted=true for an out-of-bounds range")
	}

	m.close()
	if m.active || m.View() != "" {
		t.Error("close() should deactivate and render empty")
	}
}

func TestCursorAnnotationCoversRange(t *testing.T) {
	theme := DefaultTheme()
	m := diffViewModel{
		theme: &theme, hl: newHighlighter(), mdStyler: newMarkdownStyler(theme),
		path: "main.go", width: 80, height: 30, style: diffStyleUnified,
		hunks: []types.DiffHunk{{
			OldStart: 1, OldCount: 0, NewStart: 1, NewCount: 3, Header: "f",
			Lines: []types.DiffLine{
				{Kind: types.DiffLineAdded, NewLineNum: 1, Content: "a"},
				{Kind: types.DiffLineAdded, NewLineNum: 2, Content: "b"},
				{Kind: types.DiffLineAdded, NewLineNum: 3, Content: "c"},
			},
		}},
		annotations: []types.Annotation{
			{ID: "x1", TargetRef: "main.go", LineStart: 1, LineEnd: 2, Summary: "s",
				Refs: []types.DocRef{{Kind: types.DocRefFile, Doc: "d.md"}}},
		},
	}
	m.buildLines()
	// Put the cursor on the first code line (within the annotation range).
	for i, ln := range m.lines {
		if ln.newLineNum == 1 {
			m.cursor = i
		}
	}
	a := m.CursorAnnotation()
	if a == nil || a.ID != "x1" {
		t.Fatalf("CursorAnnotation on line 1 = %v, want x1", a)
	}
	// A line outside the range returns nil.
	for i, ln := range m.lines {
		if ln.newLineNum == 3 {
			m.cursor = i
		}
	}
	if got := m.CursorAnnotation(); got != nil {
		t.Errorf("CursorAnnotation on line 3 = %v, want nil", got)
	}
}

func TestCycleFocusIncludesDocPane(t *testing.T) {
	m := appModel{}
	m.docPane.active = true
	m.focus = focusSidebar
	m = m.cycleFocus(1)
	if m.focus != focusMain {
		t.Errorf("after one cycle: focus=%d, want focusMain", m.focus)
	}
	m = m.cycleFocus(1)
	if m.focus != focusDoc {
		t.Errorf("after two cycles: focus=%d, want focusDoc", m.focus)
	}
	m = m.cycleFocus(1)
	if m.focus != focusSidebar {
		t.Errorf("after three cycles: focus=%d, want focusSidebar (wrap)", m.focus)
	}
	// Reverse.
	m = m.cycleFocus(-1)
	if m.focus != focusDoc {
		t.Errorf("reverse from sidebar: focus=%d, want focusDoc", m.focus)
	}
}
