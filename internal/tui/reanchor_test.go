package tui

import (
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

// compactDiff mimics what leaving whole-file mode leaves behind: only the
// hunks, so most source lines have no row at all.
func compactDiff(t *testing.T) diffViewModel {
	t.Helper()
	var lines []diffViewLine
	add := func(n int) {
		lines = append(lines, diffViewLine{kind: types.DiffLineContext, newLineNum: n, content: "x"})
	}
	lines = append(lines, diffViewLine{isHunk: true, content: "@@ -100,4 +100,4 @@"})
	for n := 100; n <= 103; n++ {
		add(n)
	}
	lines = append(lines, diffViewLine{isHunk: true, content: "@@ -500,4 +500,4 @@"})
	for n := 500; n <= 503; n++ {
		add(n)
	}
	return diffViewModel{lines: lines, width: 80, height: 20, tabSize: 4}
}

func TestReanchorFallsBackToNearestLine(t *testing.T) {
	// The reported bug: toggling `a` off from an unchanged line threw the
	// position away, because that line is exactly what the compact diff omits.
	t.Run("a line the compact diff omits lands nearby, not at the top", func(t *testing.T) {
		m := compactDiff(t)
		m.reanchorTo(480) // unchanged context, nearest row is 500
		if got := m.lineNumAt(m.cursor); got != 500 {
			t.Errorf("landed on line %d, want 500", got)
		}
		if m.cursor == 0 {
			t.Error("cursor went to the top of the file")
		}
	})

	t.Run("an exact match still wins", func(t *testing.T) {
		m := compactDiff(t)
		m.reanchorTo(502)
		if got := m.lineNumAt(m.cursor); got != 502 {
			t.Errorf("landed on line %d, want 502", got)
		}
	})

	t.Run("closest is by distance, not direction", func(t *testing.T) {
		m := compactDiff(t)
		m.reanchorTo(120) // 103 is 17 away, 500 is 380
		if got := m.lineNumAt(m.cursor); got != 103 {
			t.Errorf("landed on line %d, want 103", got)
		}
	})

	t.Run("a tie lands on the earlier line", func(t *testing.T) {
		m := compactDiff(t)
		// Midpoint of 103 and 500 is 301.5; 302 is 199 from 103 and 198 from
		// 500, so use 301: 198 from 103, 199 from 500.
		m.reanchorTo(301)
		if got := m.lineNumAt(m.cursor); got != 103 {
			t.Errorf("landed on line %d, want the earlier side (103)", got)
		}
	})

	t.Run("past the end lands on the last numbered line", func(t *testing.T) {
		m := compactDiff(t)
		m.reanchorTo(9000)
		if got := m.lineNumAt(m.cursor); got != 503 {
			t.Errorf("landed on line %d, want 503", got)
		}
	})

	t.Run("before the start lands on the first numbered line", func(t *testing.T) {
		m := compactDiff(t)
		m.reanchorTo(1)
		if got := m.lineNumAt(m.cursor); got != 100 {
			t.Errorf("landed on line %d, want 100", got)
		}
	})

	// A view with nothing numbered has no better answer than the top, and must
	// not index off the end reaching for one.
	t.Run("a view with no numbered lines is safe", func(t *testing.T) {
		m := diffViewModel{width: 80, height: 20, lines: []diffViewLine{
			{isHunk: true, content: "@@"},
		}}
		m.reanchorTo(42)
		if m.cursor != 0 || m.offset != 0 {
			t.Errorf("cursor=%d offset=%d, want the top", m.cursor, m.offset)
		}
	})
}
