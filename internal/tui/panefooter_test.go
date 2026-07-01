package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func makeBox(w int) string {
	top := "┌" + strings.Repeat("─", w-2) + "┐"
	mid := "│" + strings.Repeat(" ", w-2) + "│"
	bot := "└" + strings.Repeat("─", w-2) + "┘"
	return strings.Join([]string{top, mid, bot}, "\n")
}

func TestWithPathFooter(t *testing.T) {
	box := makeBox(30)
	boxW := lipgloss.Width(strings.Split(box, "\n")[0])

	t.Run("embeds the path and preserves width", func(t *testing.T) {
		out := withPathFooter(box, "pkg/util.go", lipgloss.Color("8"))
		last := strings.Split(out, "\n")[2]
		if !strings.Contains(stripANSISeq(last), "pkg/util.go") {
			t.Errorf("footer should contain the path, got %q", stripANSISeq(last))
		}
		if w := lipgloss.Width(last); w != boxW {
			t.Errorf("footer width %d != box width %d", w, boxW)
		}
	})

	t.Run("empty label is a no-op", func(t *testing.T) {
		if withPathFooter(box, "", lipgloss.Color("8")) != box {
			t.Error("empty label should leave the box unchanged")
		}
	})

	t.Run("long path is left-truncated keeping the filename", func(t *testing.T) {
		out := withPathFooter(box, "very/long/path/to/some/deep/file.go", lipgloss.Color("8"))
		last := stripANSISeq(strings.Split(out, "\n")[2])
		if !strings.Contains(last, "file.go") || !strings.Contains(last, "…") {
			t.Errorf("expected left-truncated path keeping the filename, got %q", last)
		}
		if w := lipgloss.Width(strings.Split(out, "\n")[2]); w != boxW {
			t.Errorf("truncated footer width %d != box width %d", w, boxW)
		}
	})
}
