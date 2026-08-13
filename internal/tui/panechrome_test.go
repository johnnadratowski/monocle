package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func makeBox(w int) string {
	top := "┌" + strings.Repeat("─", w-2) + "┐"
	mid := "│" + strings.Repeat(" ", w-2) + "│"
	bot := "└" + strings.Repeat("─", w-2) + "┘"
	return strings.Join([]string{top, mid, bot}, "\n")
}

func TestWithPaneChrome(t *testing.T) {
	box := makeBox(40)
	boxW := lipgloss.Width(strings.Split(box, "\n")[0])
	first := func(out string) string { return stripANSISeq(strings.Split(out, "\n")[0]) }
	firstRaw := func(out string) string { return strings.Split(out, "\n")[0] }

	t.Run("embeds the path and preserves width", func(t *testing.T) {
		out := withPaneChrome(box, paneChrome{label: "pkg/util.go"}, lipgloss.Color("8"))
		if !strings.Contains(first(out), "pkg/util.go") {
			t.Errorf("header should contain the path, got %q", first(out))
		}
		if w := lipgloss.Width(firstRaw(out)); w != boxW {
			t.Errorf("header width %d != box width %d", w, boxW)
		}
	})

	t.Run("shows the mode badge next to the path", func(t *testing.T) {
		out := withPaneChrome(box, paneChrome{label: "pkg/util.go", mode: "ALL"}, lipgloss.Color("8"))
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
		out := withPaneChrome(box, paneChrome{label: "pkg/util.go", mode: "ALL"}, lipgloss.Color("8"))
		if got, want := strings.Split(out, "\n")[2], strings.Split(box, "\n")[2]; got != want {
			t.Errorf("bottom border changed: %q", got)
		}
	})

	t.Run("empty label is a no-op", func(t *testing.T) {
		if withPaneChrome(box, paneChrome{label: "", mode: "ALL"}, lipgloss.Color("8")) != box {
			t.Error("empty label should leave the box unchanged")
		}
	})

	t.Run("long path is truncated in the middle keeping both ends", func(t *testing.T) {
		out := withPaneChrome(box, paneChrome{label: "very/long/path/to/some/deep/nested/file.go"}, lipgloss.Color("8"))
		got := first(out)
		if !strings.Contains(got, "very/") || !strings.Contains(got, "file.go") || !strings.Contains(got, "…") {
			t.Errorf("expected head and tail to survive with an ellipsis between, got %q", got)
		}
		if w := lipgloss.Width(firstRaw(out)); w != boxW {
			t.Errorf("truncated header width %d != box width %d", w, boxW)
		}
	})

	t.Run("narrow pane drops the badge before the path", func(t *testing.T) {
		narrow := makeBox(18)
		narrowW := lipgloss.Width(strings.Split(narrow, "\n")[0])
		out := withPaneChrome(narrow, paneChrome{label: "some/deep/file.go", mode: "v3→v5 SPLIT"}, lipgloss.Color("8"))
		got := first(out)
		if strings.Contains(got, "SPLIT") {
			t.Errorf("badge should be dropped when it cannot fit, got %q", got)
		}
		// Middle truncation: both ends of the name survive, so the label still
		// says what kind of file it is and roughly where it lives.
		if !strings.Contains(got, "…") || !strings.Contains(got, "some") || !strings.Contains(got, "file.go") {
			t.Errorf("expected both ends of the path to survive, got %q", got)
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
		{"full file names its layout", diffViewModel{fullFile: true}, "ALL · UNIFIED"},
		{"split", diffViewModel{style: diffStyleSplit}, "SPLIT"},
		{"full file and split combine", diffViewModel{fullFile: true, style: diffStyleSplit}, "ALL · SPLIT"},
		{"the all-defaults view stays badge-free", diffViewModel{}, ""},
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

func TestWithPaneChrome_Age(t *testing.T) {
	box := makeBox(48)
	boxW := lipgloss.Width(strings.Split(box, "\n")[0])
	first := func(out string) string { return stripANSISeq(strings.Split(out, "\n")[0]) }

	t.Run("age sits next to the file name", func(t *testing.T) {
		out := withPaneChrome(box, paneChrome{label: "pkg/util.go", age: "5m", mode: "ALL"}, lipgloss.Color("8"))
		got := first(out)
		for _, want := range []string{"pkg/util.go", "· 5m", "[ALL]"} {
			if !strings.Contains(got, want) {
				t.Errorf("expected %q in header, got %q", want, got)
			}
		}
		// Order: path, then age, then mode.
		p, a, b := strings.Index(got, "pkg/util.go"), strings.Index(got, "· 5m"), strings.Index(got, "[ALL]")
		if !(p < a && a < b) {
			t.Errorf("expected path < age < mode ordering, got %q", got)
		}
		if w := lipgloss.Width(strings.Split(out, "\n")[0]); w != boxW {
			t.Errorf("header width %d != box width %d", w, boxW)
		}
	})

	t.Run("age is omitted when unknown", func(t *testing.T) {
		if got := first(withPaneChrome(box, paneChrome{label: "pkg/util.go", mode: "ALL"}, lipgloss.Color("8"))); strings.Contains(got, "·") {
			t.Errorf("no age should mean no separator, got %q", got)
		}
	})

	t.Run("age is shed before the mode when narrow", func(t *testing.T) {
		narrow := makeBox(28)
		got := first(withPaneChrome(narrow, paneChrome{label: "some/deep/nested/file.go", age: "12d", mode: "ALL"}, lipgloss.Color("8")))
		if strings.Contains(got, "12d") {
			t.Errorf("age should be dropped first, got %q", got)
		}
		if !strings.Contains(got, "[ALL]") {
			t.Errorf("mode should outlive the age, got %q", got)
		}
	})
}

func TestHumanizeAge(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"seconds read as now", 30 * time.Second, "now"},
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 3 * time.Hour, "3h"},
		{"days", 50 * time.Hour, "2d"},
		{"weeks", 21 * 24 * time.Hour, "3w"},
		{"future clamps to now", -time.Hour, "now"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := humanizeAge(tt.d, now); got != tt.want {
				t.Errorf("humanizeAge(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}

	t.Run("zero time is unknown", func(t *testing.T) {
		if got := humanizeAge(time.Hour, time.Time{}); got != "" {
			t.Errorf("zero timestamp should render nothing, got %q", got)
		}
	})
}

func TestFileModTime(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("resolves a repo-relative path against the root", func(t *testing.T) {
		if got := fileModTime(dir, "f.go"); got.IsZero() {
			t.Error("expected a mod time for a repo-relative path")
		}
	})
	t.Run("accepts an absolute path with no root", func(t *testing.T) {
		if got := fileModTime("", filepath.Join(dir, "f.go")); got.IsZero() {
			t.Error("expected a mod time for an absolute path")
		}
	})
	t.Run("missing file is unknown, not an error", func(t *testing.T) {
		if got := fileModTime(dir, "gone.go"); !got.IsZero() {
			t.Error("a missing file should report an unknown (zero) time")
		}
	})
}

func TestScrollChip(t *testing.T) {
	tests := []struct {
		name          string
		lines, chunks int
		short         bool
		want          string
	}{
		{"nothing off screen renders nothing", 0, 0, false, ""},
		{"chunks omitted when there are none", 40, 0, false, "↑ 40 lines"},
		{"lines and chunks", 128, 3, false, "↑ 128 lines · 3 chunks"},
		{"singulars", 1, 1, false, "↑ 1 line · 1 chunk"},
		{"short form", 128, 3, true, "↑128 ·3"},
		{"short form without chunks", 128, 0, true, "↑128"},
		{"zero lines wins over chunks", 0, 5, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scrollChip("↑", tt.lines, tt.chunks, tt.short); got != tt.want {
				t.Errorf("scrollChip() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWithPaneChrome_ScrollMarkers(t *testing.T) {
	box := makeBox(72)
	boxW := lipgloss.Width(strings.Split(box, "\n")[0])
	line := func(out string, i int) string { return stripANSISeq(strings.Split(out, "\n")[i]) }
	rawLine := func(out string, i int) string { return strings.Split(out, "\n")[i] }
	full := paneChrome{
		label:  "pkg/util.go",
		extent: scrollExtent{linesAbove: 128, chunksAbove: 3, linesBelow: 940, chunksBelow: 12},
	}

	t.Run("above goes in the top border, below in the bottom", func(t *testing.T) {
		out := withPaneChrome(box, full, lipgloss.Color("8"))
		if got := line(out, 0); !strings.Contains(got, "↑ 128 lines · 3 chunks") {
			t.Errorf("top border missing the above-chip: %q", got)
		}
		if got := line(out, 2); !strings.Contains(got, "↓ 940 lines · 12 chunks") {
			t.Errorf("bottom border missing the below-chip: %q", got)
		}
		if got := line(out, 0); strings.Contains(got, "↓") {
			t.Errorf("below-chip leaked into the top border: %q", got)
		}
	})

	t.Run("borders keep their width", func(t *testing.T) {
		out := withPaneChrome(box, full, lipgloss.Color("8"))
		for _, i := range []int{0, 2} {
			if w := lipgloss.Width(rawLine(out, i)); w != boxW {
				t.Errorf("border %d width %d != box width %d (%q)", i, w, boxW, line(out, i))
			}
		}
	})

	t.Run("corners survive", func(t *testing.T) {
		out := withPaneChrome(box, full, lipgloss.Color("8"))
		if got := line(out, 0); !strings.HasPrefix(got, "┌") || !strings.HasSuffix(got, "┐") {
			t.Errorf("top border lost a corner: %q", got)
		}
		if got := line(out, 2); !strings.HasPrefix(got, "└") || !strings.HasSuffix(got, "┘") {
			t.Errorf("bottom border lost a corner: %q", got)
		}
	})

	t.Run("a fully visible document shows no chips", func(t *testing.T) {
		out := withPaneChrome(box, paneChrome{label: "pkg/util.go"}, lipgloss.Color("8"))
		if got := line(out, 0); strings.Contains(got, "↑") {
			t.Errorf("no off-screen content should mean no chip: %q", got)
		}
		if got, want := strings.Split(out, "\n")[2], strings.Split(box, "\n")[2]; got != want {
			t.Errorf("bottom border should be untouched, got %q", got)
		}
	})

	t.Run("scroll chip is shed after the age but before the mode", func(t *testing.T) {
		narrow := makeBox(44)
		out := withPaneChrome(narrow, paneChrome{
			label:  "internal/tui/diffview.go",
			age:    "12d",
			mode:   "ALL",
			extent: scrollExtent{linesAbove: 128, chunksAbove: 3},
		}, lipgloss.Color("8"))
		got := line(out, 0)
		if strings.Contains(got, "12d") {
			t.Errorf("age should be shed first, got %q", got)
		}
		if !strings.Contains(got, "↑") {
			t.Errorf("scroll chip should outlive the age, got %q", got)
		}
		if !strings.Contains(got, "[ALL]") {
			t.Errorf("mode should survive, got %q", got)
		}
		if w := lipgloss.Width(rawLine(out, 0)); w != lipgloss.Width(strings.Split(narrow, "\n")[0]) {
			t.Errorf("narrow header width drifted: %q", got)
		}
	})

	t.Run("scroll chip falls back to its short form before vanishing", func(t *testing.T) {
		narrow := makeBox(38)
		out := withPaneChrome(narrow, paneChrome{
			label:  "internal/tui/diffview.go",
			mode:   "ALL",
			extent: scrollExtent{linesAbove: 128, chunksAbove: 3},
		}, lipgloss.Color("8"))
		got := line(out, 0)
		if !strings.Contains(got, "↑128 ·3") {
			t.Errorf("expected the short chip form, got %q", got)
		}
		if strings.Contains(got, "lines") {
			t.Errorf("full form should not fit here, got %q", got)
		}
	})

	t.Run("very narrow pane keeps the path and drops the chips", func(t *testing.T) {
		narrow := makeBox(24)
		out := withPaneChrome(narrow, paneChrome{
			label:  "internal/tui/diffview.go",
			age:    "12d",
			mode:   "ALL",
			extent: scrollExtent{linesAbove: 128, chunksAbove: 3},
		}, lipgloss.Color("8"))
		got := line(out, 0)
		if !strings.Contains(got, "…") || !strings.Contains(got, "intern") || !strings.Contains(got, ".go") {
			t.Errorf("path should be the last thing surrendered, and keep both ends: %q", got)
		}
		if w := lipgloss.Width(rawLine(out, 0)); w != lipgloss.Width(strings.Split(narrow, "\n")[0]) {
			t.Errorf("narrow header width drifted: %q", got)
		}
	})

	t.Run("bottom border drops the chip rather than overflowing", func(t *testing.T) {
		narrow := makeBox(12)
		out := withPaneChrome(narrow, paneChrome{
			label:  "a.go",
			extent: scrollExtent{linesBelow: 940, chunksBelow: 12},
		}, lipgloss.Color("8"))
		if w := lipgloss.Width(rawLine(out, 2)); w != lipgloss.Width(strings.Split(narrow, "\n")[2]) {
			t.Errorf("bottom border width drifted: %q", line(out, 2))
		}
	})
}

func TestWithPaneChrome_StatusAndSides(t *testing.T) {
	box := makeBox(92)
	first := func(out string) string { return stripANSISeq(strings.Split(out, "\n")[0]) }
	full := paneChrome{
		label: "internal/core/engine_impl.go", age: "12m", status: "modified", mode: "ALL",
		extent: scrollExtent{linesAbove: 128, chunksAbove: 3},
	}

	t.Run("identity is left, state is right", func(t *testing.T) {
		got := first(withPaneChrome(box, full, lipgloss.Color("8")))
		iPath := strings.Index(got, "engine_impl.go")
		iAge := strings.Index(got, "· 12m")
		iStatus := strings.Index(got, "· modified")
		iMode := strings.Index(got, "[ALL]")
		iScroll := strings.Index(got, "↑")
		for name, idx := range map[string]int{"path": iPath, "age": iAge, "status": iStatus, "mode": iMode, "scroll": iScroll} {
			if idx < 0 {
				t.Fatalf("%s missing from header: %q", name, got)
			}
		}
		if !(iPath < iAge && iAge < iStatus) {
			t.Errorf("expected path < age < status on the left, got %q", got)
		}
		if !(iStatus < iMode && iMode < iScroll) {
			t.Errorf("expected mode and scroll right of the identity, got %q", got)
		}
		// The mode badge must sit against the right edge, not beside the path.
		if gap := strings.Index(got, "[ALL]") - strings.Index(got, "· modified"); gap < 10 {
			t.Errorf("mode should be right-anchored, not adjacent to the label: %q", got)
		}
	})

	t.Run("width is preserved with every chip present", func(t *testing.T) {
		out := strings.Split(withPaneChrome(box, full, lipgloss.Color("8")), "\n")[0]
		if got, want := lipgloss.Width(out), lipgloss.Width(strings.Split(box, "\n")[0]); got != want {
			t.Errorf("header width %d != box width %d", got, want)
		}
	})

	t.Run("mode alone still right-anchors", func(t *testing.T) {
		got := first(withPaneChrome(box, paneChrome{label: "a.go", mode: "SPLIT"}, lipgloss.Color("8")))
		if !strings.HasSuffix(strings.TrimRight(got, "─┐ "), "[SPLIT]") {
			t.Errorf("expected the badge against the right edge, got %q", got)
		}
	})

	t.Run("status is omitted for artifacts", func(t *testing.T) {
		got := first(withPaneChrome(box, paneChrome{label: "plan: OAuth", age: "now"}, lipgloss.Color("8")))
		for _, s := range []string{"modified", "added", "deleted"} {
			if strings.Contains(got, s) {
				t.Errorf("an artifact has no change kind, got %q", got)
			}
		}
	})

	t.Run("age sheds before status, status before mode", func(t *testing.T) {
		got := first(withPaneChrome(makeBox(58), full, lipgloss.Color("8")))
		if strings.Contains(got, "12m") {
			t.Errorf("age should shed first, got %q", got)
		}
		if !strings.Contains(got, "[ALL]") {
			t.Errorf("mode should outlive the age, got %q", got)
		}
		narrower := first(withPaneChrome(makeBox(48), full, lipgloss.Color("8")))
		if strings.Contains(narrower, "modified") {
			t.Errorf("status should shed before the mode, got %q", narrower)
		}
		if !strings.Contains(narrower, "[ALL]") {
			t.Errorf("mode should outlive the status, got %q", narrower)
		}
	})
}

func TestStatusChipStyleTracksSidebar(t *testing.T) {
	// The sidebar's A/M/D/R letters and this word must never disagree about a
	// file's change kind, so they share the same palette.
	for status, want := range map[string]string{
		"added": "#2ea043", "modified": "#d29922", "deleted": "#f85149", "renamed": "#a371f7",
	} {
		got := statusChipStyle(status).GetForeground()
		if got != lipgloss.Color(want) {
			t.Errorf("%s colour = %v, want %s", status, got, want)
		}
	}
}
