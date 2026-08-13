package tui

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

const overflowing = "func handleRequestWithAVeryLongSignature(ctx context.Context, req *Request, opts ...Option) (*Response, error) { // trailing"

func autoWrapModel(t *testing.T) diffViewModel {
	t.Helper()
	theme := DefaultTheme()
	m := diffViewModel{
		path: "x.go", theme: &theme, hl: newHighlighter(), mdStyler: newMarkdownStyler(theme),
		width: 60, height: 30, tabSize: 4, wrap: false,
		lines: []diffViewLine{
			{kind: types.DiffLineContext, newLineNum: 1, content: "package main"},
			{kind: types.DiffLineContext, newLineNum: 2, content: overflowing},
			{kind: types.DiffLineContext, newLineNum: 3, content: "short()"},
		},
	}
	return m
}

func rowCount(m diffViewModel) int {
	out := m.View()
	if out == "" {
		return 0
	}
	return len(strings.Split(out, "\n"))
}

func TestCursorLineAutoWraps(t *testing.T) {
	t.Run("the cursor's line expands when it runs off the side", func(t *testing.T) {
		m := autoWrapModel(t)
		m.cursor = 1
		if got := m.screenLinesFor(1); got < 2 {
			t.Errorf("the overflowing cursor line should occupy %d rows, got %d", 2, got)
		}
		if !strings.Contains(stripANSISeq(m.View()), "trailing") {
			t.Error("the tail of the cursor line should be readable without scrolling")
		}
	})

	t.Run("other lines stay one row", func(t *testing.T) {
		m := autoWrapModel(t)
		m.cursor = 0
		if got := m.screenLinesFor(1); got != 1 {
			t.Errorf("a non-cursor overflowing line should stay one row, got %d", got)
		}
		if strings.Contains(stripANSISeq(m.View()), "trailing") {
			t.Error("a non-cursor line should still be truncated")
		}
	})

	t.Run("a line that fits does not expand", func(t *testing.T) {
		m := autoWrapModel(t)
		m.cursor = 0
		if got := m.screenLinesFor(0); got != 1 {
			t.Errorf("a line that fits should stay one row, got %d", got)
		}
	})

	// The invariant that keeps the cursor and the viewport together: whatever
	// screenLinesFor claims must be what the renderer actually draws.
	t.Run("screenLinesFor matches the rows drawn", func(t *testing.T) {
		for _, cursor := range []int{0, 1, 2} {
			m := autoWrapModel(t)
			m.cursor = cursor
			want := 0
			for i := range m.lines {
				want += m.screenLinesFor(i)
			}
			if got := rowCount(m); got != want {
				t.Errorf("cursor %d: drew %d rows, screenLinesFor totals %d", cursor, got, want)
			}
		}
	})

	t.Run("moving the cursor moves the expansion with it", func(t *testing.T) {
		m := autoWrapModel(t)
		m.cursor = 0
		before := rowCount(m)
		m.cursor = 1
		after := rowCount(m)
		if after <= before {
			t.Errorf("landing on the long line should add rows: %d -> %d", before, after)
		}
		m.cursor = 2
		if got := rowCount(m); got != before {
			t.Errorf("leaving it should give the rows back: %d, want %d", got, before)
		}
	})

	t.Run("structural rows never expand", func(t *testing.T) {
		m := autoWrapModel(t)
		for _, line := range []diffViewLine{
			{isHunk: true, content: overflowing},
			{isComment: true, content: overflowing},
			{isAnnotation: true, content: overflowing},
			{verbatim: true, content: overflowing},
		} {
			if m.autoWrapsCursorLine(line, 40) {
				t.Errorf("a structural row should not auto-wrap: %+v", line)
			}
		}
	})

	t.Run("wrap mode leaves the predicate alone", func(t *testing.T) {
		m := autoWrapModel(t)
		m.wrap = true
		if m.autoWrapsCursorLine(m.lines[1], 40) {
			t.Error("with wrap on, every line already wraps; the cursor is not special")
		}
	})
}
