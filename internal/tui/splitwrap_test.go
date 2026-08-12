package tui

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

const longLeft = "func handleRequestWithAVeryLongSignature(ctx context.Context, req *Request) error { // old"
const longRight = "func handleRequestWithAVeryLongSignature(ctx context.Context, req *Request, opts ...Option) error { // new"

func newSplitModel(t *testing.T, wrap bool) diffViewModel {
	t.Helper()
	theme := DefaultTheme()
	m := diffViewModel{
		path:  "x.go",
		width: 96, height: 40, tabSize: 4, style: diffStyleSplit, wrap: wrap,
		theme: &theme,
		lines: []diffViewLine{
			{isSplit: true, kind: types.DiffLineRemoved, oldLineNum: 41, content: longLeft,
				rightKind: types.DiffLineAdded, rightLineNum: 41, rightContent: longRight},
			{isSplit: true, kind: types.DiffLineContext, oldLineNum: 42, content: "\tshort()",
				rightKind: types.DiffLineContext, rightLineNum: 42, rightContent: "\tshort()"},
			{isSplit: true, leftEmpty: true, kind: types.DiffLineContext, rightKind: types.DiffLineAdded,
				rightLineNum: 43, rightContent: "\tadded only on the right, long enough to need two rows"},
		},
	}
	m.hl = newHighlighter()
	m.mdStyler = newMarkdownStyler(theme)
	return m
}

// splitColumns reassembles each half of the pane independently. Joining whole
// rows would interleave the two sides, so a phrase wrapped across rows on the
// left would never appear contiguous.
func splitColumns(rows []string) (left, right string) {
	var l, r []string
	for _, row := range rows {
		i := strings.Index(row, "│")
		if i < 0 {
			continue
		}
		l = append(l, strings.TrimSpace(row[:i]))
		r = append(r, strings.TrimSpace(row[i+len("│"):]))
	}
	return strings.Join(l, " "), strings.Join(r, " ")
}

func renderedRows(m diffViewModel) []string {
	out := stripANSISeq(m.View())
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func TestSplitWrap(t *testing.T) {
	t.Run("without wrap a long side is truncated to one row", func(t *testing.T) {
		m := newSplitModel(t, false)
		if got := len(renderedRows(m)); got != len(m.lines) {
			t.Errorf("expected one row per line, got %d rows for %d lines", got, len(m.lines))
		}
		if _, right := splitColumns(renderedRows(m)); strings.Contains(right, "// new") {
			t.Error("unwrapped split should truncate, but the line's tail is visible")
		}
	})

	t.Run("with wrap the full text is shown", func(t *testing.T) {
		m := newSplitModel(t, true)
		left, right := splitColumns(renderedRows(m))
		for _, want := range []string{"// old"} {
			if !strings.Contains(left, want) {
				t.Errorf("left column should show %q, got:\n%s", want, left)
			}
		}
		for _, want := range []string{"opts ...Option", "// new", "need two rows"} {
			if !strings.Contains(right, want) {
				t.Errorf("right column should show %q, got:\n%s", want, right)
			}
		}
	})

	// The invariant that matters: screenLinesFor drives scrolling and cursor
	// placement, so if it disagrees with what renderSplitLine draws, the cursor
	// and the viewport bottom drift apart as you move down the file.
	t.Run("screenLinesFor matches the rows actually drawn", func(t *testing.T) {
		for _, wrap := range []bool{false, true} {
			m := newSplitModel(t, wrap)
			want := 0
			for i := range m.lines {
				want += m.screenLinesFor(i)
			}
			if got := len(renderedRows(m)); got != want {
				t.Errorf("wrap=%v: drew %d rows, screenLinesFor totals %d", wrap, got, want)
			}
		}
	})

	t.Run("the divider stays in one column on every row", func(t *testing.T) {
		m := newSplitModel(t, true)
		rows := renderedRows(m)
		col := strings.Index(rows[0], "│")
		if col < 0 {
			t.Fatalf("no divider in the first row: %q", rows[0])
		}
		for i, r := range rows {
			if got := strings.Index(r, "│"); got != col {
				t.Errorf("row %d has its divider at column %d, want %d:\n%q", i, got, col, r)
			}
		}
	})

	t.Run("a line number appears once, on the row that starts the line", func(t *testing.T) {
		m := newSplitModel(t, true)
		rows := renderedRows(m)
		if n := strings.Count(strings.Join(rows, "\n"), "  41 "); n != 2 {
			t.Errorf("line 41 should be numbered once per side, got %d occurrences", n)
		}
		if !strings.Contains(rows[0], "41") {
			t.Errorf("the first row of the line should carry the number, got %q", rows[0])
		}
		if strings.Contains(rows[1], "41") {
			t.Errorf("a continuation row should not repeat the number, got %q", rows[1])
		}
	})

	t.Run("an absent side stays blank across every row of a wrapped line", func(t *testing.T) {
		m := newSplitModel(t, true)
		rows := renderedRows(m)
		// The last line has leftEmpty and wraps to two rows.
		last := rows[len(rows)-2:]
		for _, r := range last {
			left := r[:strings.Index(r, "│")]
			if strings.TrimSpace(left) != "" {
				t.Errorf("left side should be blank on a leftEmpty line, got %q", left)
			}
		}
	})

	t.Run("toggling wrap changes the split row count", func(t *testing.T) {
		m := newSplitModel(t, false)
		before := len(renderedRows(m))
		m.ToggleWrap()
		if after := len(renderedRows(m)); after <= before {
			t.Errorf("w should expand wrapped split rows: %d -> %d", before, after)
		}
		m.ToggleWrap()
		if after := len(renderedRows(m)); after != before {
			t.Errorf("toggling back should restore %d rows, got %d", before, after)
		}
	})
}
