package tui

import (
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

// twoHunkFile is one file whose two hunks belong to different fixes — the case
// the whole feature exists for.
func summaryModel(t *testing.T) diffViewModel {
	t.Helper()
	th := DefaultTheme()
	km := DefaultKeyMap()
	m := newDiffViewModel(&th, &km)
	m.width, m.height = 120, 40
	m.path = "a.go"
	m.hunks = []types.DiffHunk{
		{OldStart: 10, OldCount: 2, NewStart: 10, NewCount: 2, Lines: []types.DiffLine{
			{Kind: types.DiffLineRemoved, Content: "old one", OldLineNum: 10},
			{Kind: types.DiffLineAdded, Content: "new one", NewLineNum: 10},
		}},
		{OldStart: 90, OldCount: 2, NewStart: 90, NewCount: 2, Lines: []types.DiffLine{
			{Kind: types.DiffLineRemoved, Content: "old two", OldLineNum: 90},
			{Kind: types.DiffLineAdded, Content: "new two", NewLineNum: 90},
		}},
	}
	m.summaryItems = []types.SummaryItem{
		{ID: "fix-a", Text: "first fix", Targets: []types.SummaryTarget{{Path: "a.go", LineStart: 9, LineEnd: 12}}},
		{ID: "fix-b", Text: "second fix", Targets: []types.SummaryTarget{{Path: "a.go", LineStart: 88, LineEnd: 95}}},
	}
	m.buildLines()
	return m
}

func TestSummaryTagsByHunk(t *testing.T) {
	m := summaryModel(t)
	want := map[string]string{
		"old one": "fix-a", "new one": "fix-a",
		"old two": "fix-b", "new two": "fix-b",
	}
	seen := 0
	for _, ln := range m.lines {
		if ln.isHunk {
			continue
		}
		w, ok := want[ln.content]
		if !ok {
			continue
		}
		seen++
		if ln.summaryItemID != w {
			t.Errorf("%q tagged %q, want %q", ln.content, ln.summaryItemID, w)
		}
	}
	if seen != 4 {
		t.Fatalf("checked %d lines, want 4", seen)
	}
}

// A removed line has no new-file number, so per-line matching against the
// agent's ranges would tag the "after" of every edit and not the "before".
func TestRemovedLinesAreTaggedWithTheirHunk(t *testing.T) {
	m := summaryModel(t)
	for _, ln := range m.lines {
		if ln.kind == types.DiffLineRemoved && ln.summaryItemID == "" {
			t.Errorf("removed line untagged: %q", ln.content)
		}
	}
}

// An untagged row must never read as item 0 — the zero value has to mean "none".
func TestUntaggedRowsHaveNoColour(t *testing.T) {
	m := summaryModel(t)
	m.summaryItems = nil
	m.buildLines()
	for _, ln := range m.lines {
		if c := m.summaryColorFor(ln); c != nil {
			t.Fatalf("a review with no summary must colour nothing, got %v on %q", c, ln.content)
		}
	}
}

func TestSummarySelectionHidesOtherHunksInCompactDiff(t *testing.T) {
	m := summaryModel(t)
	m.activeSummaryID = "fix-a"
	for _, ln := range m.lines {
		if ln.isHunk {
			continue
		}
		switch ln.content {
		case "old one", "new one":
			if m.isHiddenBySummary(ln) {
				t.Errorf("the selected item's own line was hidden: %q", ln.content)
			}
		case "old two", "new two":
			if !m.isHiddenBySummary(ln) {
				t.Errorf("a line from another item should be hidden: %q", ln.content)
			}
		}
	}
}

// Whole-file mode exists to show the file as it is, so the other hunks stay —
// faded, not removed — and the reviewer keeps their bearings.
func TestSummarySelectionOnlyFadesInWholeFileMode(t *testing.T) {
	m := summaryModel(t)
	m.activeSummaryID = "fix-a"
	m.fullFile = true
	for _, ln := range m.lines {
		if ln.isHunk {
			continue
		}
		switch ln.content {
		case "old two", "new two":
			if m.isHiddenBySummary(ln) {
				t.Errorf("whole-file mode must not hide %q", ln.content)
			}
			if !m.isFadedBySummary(ln) {
				t.Errorf("whole-file mode should fade %q", ln.content)
			}
		case "old one", "new one":
			if m.isFadedBySummary(ln) {
				t.Errorf("the selected item's own line was faded: %q", ln.content)
			}
		}
	}
}

func TestNoSelectionHidesAndFadesNothing(t *testing.T) {
	m := summaryModel(t)
	for _, ln := range m.lines {
		if m.isHiddenBySummary(ln) || m.isFadedBySummary(ln) {
			t.Errorf("nothing is selected; %q should be untouched", ln.content)
		}
	}
}

// A target with no range claims its file entirely, which is the right default
// for a change that IS the file.
func TestWholeFileTargetClaimsEveryHunk(t *testing.T) {
	m := summaryModel(t)
	m.summaryItems = []types.SummaryItem{
		{ID: "all", Text: "rewrote the file", Targets: []types.SummaryTarget{{Path: "a.go"}}},
	}
	m.buildLines()
	for _, ln := range m.lines {
		if ln.isHunk {
			continue
		}
		if ln.summaryItemID != "all" {
			t.Errorf("%q tagged %q, want the whole-file item", ln.content, ln.summaryItemID)
		}
	}
}

// The gutter bar is what makes the colours readable before anything is selected.
func TestGutterBarColoursByItemPosition(t *testing.T) {
	m := summaryModel(t)
	var first, second string
	for _, ln := range m.lines {
		if ln.isHunk {
			continue
		}
		if ln.content == "new one" {
			first = m.renderDiffLine(ln, 0, 100, false, false)
		}
		if ln.content == "new two" {
			second = m.renderDiffLine(ln, 0, 100, false, false)
		}
	}
	if first == "" || second == "" {
		t.Fatal("expected both lines to render")
	}
	for _, s := range []string{first, second} {
		if !containsRune(s, '▌') {
			t.Errorf("no summary bar in the gutter: %q", s)
		}
	}
	if first == second {
		t.Error("two items should not render identically")
	}
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

// Dropping a selection that no longer exists matters because the agent replaces
// the summary wholesale: a stale id would filter the review to nothing and
// explain nothing.
func TestSelectionIsDroppedWhenTheItemGoes(t *testing.T) {
	var m appModel
	m.activeSummaryID = "gone"
	m.setSummaryItems([]types.SummaryItem{{ID: "still-here", Text: "a fix"}})
	if m.activeSummaryID != "" {
		t.Errorf("activeSummaryID = %q, want it cleared", m.activeSummaryID)
	}
	m.activeSummaryID = "still-here"
	m.setSummaryItems([]types.SummaryItem{{ID: "still-here", Text: "a fix"}})
	if m.activeSummaryID != "still-here" {
		t.Error("a selection that still exists must survive a re-send")
	}
}
