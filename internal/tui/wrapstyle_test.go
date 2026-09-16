package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

var sgrOpen = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// openingStyle returns the SGR sequence a row starts with, or "" when the row
// begins with bare text — which is what makes it draw in the default colour.
func openingStyle(row string) string {
	if loc := sgrOpen.FindStringIndex(row); loc != nil && loc[0] == 0 {
		return row[:loc[1]]
	}
	return ""
}

// TestCarryStyles covers the rule a terminal enforces and ansi.Wrap ignores: a
// row inherits nothing from the row above it, so a style spanning a wrap
// boundary has to be re-opened.
func TestCarryStyles(t *testing.T) {
	t.Run("a span across the boundary is re-opened", func(t *testing.T) {
		rows := carryStyles([]string{"\x1b[31mred start", "still red", "red end\x1b[m"})
		for i, row := range rows {
			if openingStyle(row) != "\x1b[31m" {
				t.Errorf("row %d opens with %q, want the red still active", i, openingStyle(row))
			}
		}
	})

	t.Run("a reset ends the carry", func(t *testing.T) {
		rows := carryStyles([]string{"\x1b[31mred\x1b[m", "plain", "more plain"})
		if got := openingStyle(rows[1]); got != "" {
			t.Errorf("row 1 opens with %q; the colour was closed on row 0", got)
		}
		if got := openingStyle(rows[2]); got != "" {
			t.Errorf("row 2 opens with %q, want nothing", got)
		}
	})

	t.Run("the latest of several spans wins", func(t *testing.T) {
		rows := carryStyles([]string{"\x1b[31ma\x1b[m\x1b[32mb", "c"})
		if !strings.HasPrefix(rows[1], "\x1b[32m") {
			t.Errorf("row 1 = %q, want it to carry the green", rows[1])
		}
		if strings.HasPrefix(rows[1], "\x1b[31m") {
			t.Error("the reset before the green should have dropped the red")
		}
	})

	t.Run("plain text is untouched", func(t *testing.T) {
		in := []string{"one", "two", "three"}
		out := carryStyles(in)
		for i := range in {
			if out[i] != in[i] {
				t.Errorf("row %d = %q, want %q", i, out[i], in[i])
			}
		}
	})

	t.Run("a single row is never rewritten", func(t *testing.T) {
		in := []string{"\x1b[31monly"}
		if out := carryStyles(in); out[0] != in[0] {
			t.Errorf("got %q, want %q", out[0], in[0])
		}
	})

	// A parameter list of zeroes is a reset however it is spelled.
	t.Run("reset spellings", func(t *testing.T) {
		for _, reset := range []string{"\x1b[m", "\x1b[0m", "\x1b[00m", "\x1b[0;0m"} {
			rows := carryStyles([]string{"\x1b[31mred" + reset, "plain"})
			if got := openingStyle(rows[1]); got != "" {
				t.Errorf("%q should reset; row 1 opens with %q", reset, got)
			}
		}
	})

	// Cursor moves and the like are not style state and must not be carried.
	t.Run("non-SGR sequences are ignored", func(t *testing.T) {
		rows := carryStyles([]string{"\x1b[2Kcleared", "next"})
		if got := openingStyle(rows[1]); got != "" {
			t.Errorf("row 1 opens with %q, want nothing carried", got)
		}
	})
}

// TestWrappedCommentKeepsItsColour is the reported symptom: a long source
// comment is one token, so its colour opens once at the start of the line and
// every wrapped row after the first fell outside it and drew in the default
// foreground.
func TestWrappedCommentKeepsItsColour(t *testing.T) {
	m := diffViewModel{width: 60, height: 40, wrap: true, path: "main.go", focused: true}
	th := DefaultTheme()
	m.theme = &th
	m.hl = newHighlighter()

	const comment = "// This is a long source comment that will certainly need to wrap across more than one row in a narrow pane."
	styled := m.hl.highlightLine("main.go", comment, nil, nil, nil, 0)
	want := openingStyle(styled)
	if want == "" {
		t.Fatal("precondition: the highlighter should colour a comment")
	}

	rows := wrapContent(styled, 40)
	if len(rows) < 3 {
		t.Fatalf("precondition: expected the comment to wrap, got %d rows", len(rows))
	}
	for i, row := range rows {
		if got := openingStyle(row); got != want {
			t.Errorf("row %d opens with %q, want %q — it will draw uncoloured", i, got, want)
		}
	}
}

// Wrapping must not change how many rows a line occupies, or the cursor and the
// viewport bottom drift apart: screenLinesFor counts rows off the plain text
// while the renderer wraps the styled text.
func TestCarryStylesKeepsRowCount(t *testing.T) {
	m := diffViewModel{path: "main.go"}
	th := DefaultTheme()
	m.theme = &th
	m.hl = newHighlighter()

	const code = "func example(alpha, beta, gamma string) error { return fmt.Errorf(\"%s %s %s\", alpha, beta, gamma) }"
	plain := wrapContent(code, 30)
	styled := wrapContent(m.hl.highlightLine("main.go", code, nil, nil, nil, 0), 30)
	if len(plain) != len(styled) {
		t.Errorf("plain wrapped to %d rows, styled to %d", len(plain), len(styled))
	}
}

// The comment filter's dim state was checked only on the unwrapped path, so
// turning wrap on silently un-dimmed every comment it was meant to fade.
func TestWrapModeStillDimsComments(t *testing.T) {
	const comment = "// a source comment long enough to wrap when the pane is narrow indeed"

	newModel := func(wrap bool) diffViewModel {
		m := diffViewModel{width: 50, height: 40, wrap: wrap, path: "main.go"}
		th := DefaultTheme()
		m.theme = &th
		m.hl = newHighlighter()
		m.commentFilter = commentsDimmed
		m.commentLines = map[int]bool{7: true}
		return m
	}
	line := diffViewLine{kind: types.DiffLineContext, content: comment, oldLineNum: 7, newLineNum: 7}

	unwrapped := newModel(false).renderDiffLine(line, 0, 40, false, false)
	wrapped := newModel(true).renderDiffLine(line, 0, 40, false, false)

	dim := openingStyle(renderDimmedComment(comment, nil, 0))
	if dim == "" {
		t.Fatal("precondition: the dim style should emit a sequence")
	}
	if !strings.Contains(unwrapped, dim) {
		t.Fatalf("precondition: the unwrapped line should be dimmed, got %q", unwrapped)
	}
	for i, row := range strings.Split(wrapped, "\n") {
		if !strings.Contains(row, dim) {
			t.Errorf("wrapped row %d is not dimmed: %q", i, row)
		}
	}
}
