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

func TestWithPathHeader(t *testing.T) {
	box := makeBox(40)
	boxW := lipgloss.Width(strings.Split(box, "\n")[0])
	first := func(out string) string { return stripANSISeq(strings.Split(out, "\n")[0]) }
	firstRaw := func(out string) string { return strings.Split(out, "\n")[0] }

	t.Run("embeds the path and preserves width", func(t *testing.T) {
		out := withPathHeader(box, "pkg/util.go", "", lipgloss.Color("8"))
		if !strings.Contains(first(out), "pkg/util.go") {
			t.Errorf("header should contain the path, got %q", first(out))
		}
		if w := lipgloss.Width(firstRaw(out)); w != boxW {
			t.Errorf("header width %d != box width %d", w, boxW)
		}
	})

	t.Run("shows the mode badge next to the path", func(t *testing.T) {
		out := withPathHeader(box, "pkg/util.go", "ALL", lipgloss.Color("8"))
		got := first(out)
		if !strings.Contains(got, "pkg/util.go") || !strings.Contains(got, "[ALL]") {
			t.Errorf("expected path and [ALL] badge, got %q", got)
		}
		if idx, bidx := strings.Index(got, "pkg/util.go"), strings.Index(got, "[ALL]"); idx > bidx {
			t.Errorf("badge should follow the path, got %q", got)
		}
		if w := lipgloss.Width(firstRaw(out)); w != boxW {
			t.Errorf("header width %d != box width %d", w, boxW)
		}
	})

	t.Run("bottom border is left untouched", func(t *testing.T) {
		out := withPathHeader(box, "pkg/util.go", "ALL", lipgloss.Color("8"))
		if got, want := strings.Split(out, "\n")[2], strings.Split(box, "\n")[2]; got != want {
			t.Errorf("bottom border changed: %q", got)
		}
	})

	t.Run("empty label is a no-op", func(t *testing.T) {
		if withPathHeader(box, "", "ALL", lipgloss.Color("8")) != box {
			t.Error("empty label should leave the box unchanged")
		}
	})

	t.Run("long path is left-truncated keeping the filename", func(t *testing.T) {
		out := withPathHeader(box, "very/long/path/to/some/deep/nested/file.go", "", lipgloss.Color("8"))
		got := first(out)
		if !strings.Contains(got, "file.go") || !strings.Contains(got, "…") {
			t.Errorf("expected left-truncated path keeping the filename, got %q", got)
		}
		if w := lipgloss.Width(firstRaw(out)); w != boxW {
			t.Errorf("truncated header width %d != box width %d", w, boxW)
		}
	})

	t.Run("narrow pane drops the badge before the path", func(t *testing.T) {
		narrow := makeBox(18)
		narrowW := lipgloss.Width(strings.Split(narrow, "\n")[0])
		out := withPathHeader(narrow, "some/deep/file.go", "v3→v5 SPLIT", lipgloss.Color("8"))
		got := first(out)
		if strings.Contains(got, "SPLIT") {
			t.Errorf("badge should be dropped when it cannot fit, got %q", got)
		}
		if !strings.Contains(got, "file.go") {
			t.Errorf("path should survive, got %q", got)
		}
		if w := lipgloss.Width(firstRaw(out)); w != narrowW {
			t.Errorf("narrow header width %d != box width %d", w, narrowW)
		}
	})
}

func TestDiffViewModeLabel(t *testing.T) {
	tests := []struct {
		name string
		m    diffViewModel
		want string
	}{
		{"compact unified diff has no badge", diffViewModel{}, ""},
		{"full file", diffViewModel{fullFile: true}, "ALL"},
		{"split", diffViewModel{style: diffStyleSplit}, "SPLIT"},
		{"full file and split combine", diffViewModel{fullFile: true, style: diffStyleSplit}, "ALL · SPLIT"},
		{"raw file view", diffViewModel{style: diffStyleFile}, "FILE"},
		{"artifact raw text has no badge", diffViewModel{contentID: "a", contentMode: true}, ""},
		{"artifact diff", diffViewModel{contentID: "a"}, "DIFF"},
		{"artifact split diff", diffViewModel{contentID: "a", style: diffStyleSplit}, "SPLIT"},
		{
			"artifact version range",
			diffViewModel{contentID: "a", diffBaseVersion: 3, diffToVersion: 5},
			"v3→v5 DIFF",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.modeLabel(); got != tt.want {
				t.Errorf("modeLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}
