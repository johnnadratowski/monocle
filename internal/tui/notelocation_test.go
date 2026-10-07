package tui

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

// A location shortens from the left, a directory at a time, and never below
// its file name and line range.
func TestNoteLocationShortensFromTheLeft(t *testing.T) {
	deep := noteLocation{path: "server/api/v1/finalWrite.ts", lines: "528–548"}
	flat := noteLocation{path: "main.go", lines: "3"}
	for _, c := range []struct {
		loc   noteLocation
		width int
		want  string
	}{
		{deep, 100, "server/api/v1/finalWrite.ts:528–548"},
		{deep, 35, "server/api/v1/finalWrite.ts:528–548"},
		{deep, 34, "…/api/v1/finalWrite.ts:528–548"},
		{deep, 29, "…/v1/finalWrite.ts:528–548"},
		{deep, 25, "…/finalWrite.ts:528–548"},
		{deep, 22, ""},
		{flat, 9, "main.go:3"},
		{flat, 8, ""},
		{noteLocation{path: "docs/a.md"}, 20, "docs/a.md"},
		{noteLocation{}, 20, ""},
	} {
		if got := c.loc.shorten(c.width); got != c.want {
			t.Errorf("%+v in %d columns = %q, want %q", c.loc, c.width, got, c.want)
		}
	}
}

// noteBar is a doc pane holding a stop's note — long enough to scroll, so its
// title bar carries the scroll hint — drawn at a width; it returns the title
// row as drawn, with and without its styling, and the hint.
func noteBar(t *testing.T, width int, loc noteLocation) (raw, row, hint string) {
	t.Helper()
	th := DefaultTheme()
	d := docPaneModel{width: width, height: 6, theme: &th}
	d.openNote("tour:1.2", "1.2 · Where it lands", strings.Repeat("A line of the note.\n\n", 4), nil, nil)
	d.titleMark = stopVisitMark(false)
	d.titleLoc = loc
	d.reflow()
	hint = d.noteScrollHint()
	if !regexp.MustCompile(`^↓ \d lines$`).MatchString(hint) {
		t.Fatalf("scroll hint %q, want one like ↓ 4 lines", hint)
	}
	raw = strings.SplitN(d.View(), "\n", 2)[0]
	return raw, stripANSISeq(raw), hint
}

// After a stop's title and its mark comes, dim, where the stop is. A narrow
// pane shortens the location from the left before it cuts the title, and the
// scroll hint keeps its place at the right end.
func TestTheNoteHeaderSaysWhereTheStopIs(t *testing.T) {
	loc := noteLocation{path: "server/api/v1/finalWrite.ts", lines: "528–548"}
	for _, c := range []struct {
		width     int
		start     string
		locShown  string
		wholeHead bool
	}{
		{100, " 1.2 · Where it lands ✓  server/api/v1/finalWrite.ts:528–548 ", "server/api/v1/finalWrite.ts:528–548", true},
		{64, " 1.2 · Where it lands ✓  …/v1/finalWrite.ts:528–548 ", "…/v1/finalWrite.ts:528–548", true},
		{60, " 1.2 · Where it lands ✓  …/finalWrite.ts:528–548 ", "…/finalWrite.ts:528–548", true},
		{58, " 1.2 · Where it lan ✓  …/finalWrite.ts:528–548 ", "…/finalWrite.ts:528–548", false},
		// Too narrow for the title and any of the location: the title is what
		// the bar is for.
		{40, " 1.2 · Where it lands ✓ ", "", true},
	} {
		raw, row, hint := noteBar(t, c.width, loc)
		if !strings.HasPrefix(row, c.start) || !strings.HasSuffix(row, " "+hint+" ") || lipgloss.Width(row) != c.width {
			t.Errorf("at %d columns the title bar is %q (%d wide), want it to start %q and end with %q",
				c.width, row, lipgloss.Width(row), c.start, hint)
		}
		if c.locShown == "" {
			if strings.Contains(row, "finalWrite") {
				t.Errorf("at %d columns %q shows the location", c.width, row)
			}
			continue
		}
		if dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(c.locShown); !strings.Contains(raw, dim) {
			t.Errorf("at %d columns the location %q is not drawn dim: %q", c.width, c.locShown, raw)
		}
	}
	// No location, no change: the title, its mark, the hint.
	_, row, hint := noteBar(t, 64, noteLocation{})
	if !strings.HasPrefix(row, " 1.2 · Where it lands ✓ ") || strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(row), hint)) != "1.2 · Where it lands ✓" {
		t.Errorf("with no location the title bar is %q", row)
	}
}

// The header names the stop's file as the review does — repo-relative — with
// its lines: a range, a single line, or none for a stop with no file.
func TestAStopsHeaderNamesItsFileAndLines(t *testing.T) {
	tour := testTour()
	tour.Stops = append(tour.Stops,
		types.WalkthroughStop{ID: "3", Title: "One line", File: "a.go", LineStart: 9, LineEnd: 9},
		types.WalkthroughStop{ID: "4", Title: "Elsewhere", File: "/repo/docs/design.md", LineStart: 4, LineEnd: 12},
	)
	m, _ := tourAppWith(t, tour, &types.Config{})
	m.repoRoot = "/repo"
	for _, c := range []struct{ stop, want, not string }{
		{"1.1", "a.go:5–7", ""},
		{"1.2", "b.go:30–31", ""},
		{"2", "", ".go"},
		{"3", "a.go:9", "a.go:9–"},
		{"4", "docs/design.md:4–12", "/repo/"},
	} {
		m = typeCommand(t, m, "stop "+c.stop)
		got := noteTitle(m)
		if !strings.Contains(got, c.want) || (c.not != "" && strings.Contains(got, c.not)) {
			t.Errorf("stop %s header %q, want %q in it and not %q", c.stop, got, c.want, c.not)
		}
	}
}
