package tui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
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
		if tagOf(ln) != w {
			t.Errorf("%q tagged %q, want %q", ln.content, tagOf(ln), w)
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
		if ln.kind == types.DiffLineRemoved && tagOf(ln) == "" {
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
		if c := m.summaryBarFor(ln).color; c != nil {
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
		if tagOf(ln) != "all" {
			t.Errorf("%q tagged %q, want the whole-file item", ln.content, tagOf(ln))
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

// Whole-file mode is a single hunk spanning the file, so tagging by hunk would
// paint every line one colour and fade nothing. It shows lines, so it tags them.
func TestWholeFileModeTagsByLine(t *testing.T) {
	m := summaryModel(t)
	m.fullFile = true
	// A whole-file view: every line of the file, one hunk.
	var lines []types.DiffLine
	for i := 1; i <= 100; i++ {
		lines = append(lines, types.DiffLine{
			Kind: types.DiffLineContext, Content: "line", OldLineNum: i, NewLineNum: i,
		})
	}
	m.hunks = []types.DiffHunk{{OldStart: 1, OldCount: 100, NewStart: 1, NewCount: 100, Lines: lines}}
	m.buildLines()

	byLine := map[int]string{}
	for _, ln := range m.lines {
		if ln.isHunk {
			continue
		}
		byLine[ln.newLineNum] = tagOf(ln)
	}
	if got := byLine[10]; got != "fix-a" {
		t.Errorf("line 10 tagged %q, want fix-a (its range is 9-12)", got)
	}
	if got := byLine[90]; got != "fix-b" {
		t.Errorf("line 90 tagged %q, want fix-b (its range is 88-95)", got)
	}
	// Between the two ranges belongs to neither, which is what makes fading mean
	// something in whole-file mode.
	if got := byLine[50]; got != "" {
		t.Errorf("line 50 tagged %q, want no item — it is between the two ranges", got)
	}
}

// A removed line has no new-file number even in whole-file mode, so it takes the
// tag of the added line it sits against.
func TestWholeFileModeTagsRemovedLinesFromTheirNeighbour(t *testing.T) {
	m := summaryModel(t)
	m.fullFile = true
	m.buildLines()
	for _, ln := range m.lines {
		if ln.kind != types.DiffLineRemoved {
			continue
		}
		if tagOf(ln) == "" {
			t.Errorf("removed line %q took no tag from its neighbour", ln.content)
		}
	}
}

// A line the agent did not claim must stay unclaimed, even sitting next to one
// that is. Borrowing from a neighbour is only for rows that cannot be asked
// directly — removed lines, which have no new-file number.
func TestLineTagsDoNotBleedPastTheirRange(t *testing.T) {
	m := summaryModel(t)
	m.fullFile = true
	var lines []types.DiffLine
	for i := 1; i <= 100; i++ {
		lines = append(lines, types.DiffLine{
			Kind: types.DiffLineContext, Content: "line", OldLineNum: i, NewLineNum: i,
		})
	}
	m.hunks = []types.DiffHunk{{OldStart: 1, OldCount: 100, NewStart: 1, NewCount: 100, Lines: lines}}
	m.buildLines()

	byLine := map[int]string{}
	for _, ln := range m.lines {
		if !ln.isHunk {
			byLine[ln.newLineNum] = tagOf(ln)
		}
	}
	// fix-a covers 9-12 and fix-b covers 88-95; the lines just outside each edge
	// are the ones a careless fill would capture.
	for _, n := range []int{8, 13, 87, 96} {
		if got := byLine[n]; got != "" {
			t.Errorf("line %d tagged %q, want no item — it is outside every range", n, got)
		}
	}
	for _, tc := range []struct {
		line int
		want string
	}{{9, "fix-a"}, {12, "fix-a"}, {88, "fix-b"}, {95, "fix-b"}} {
		if got := byLine[tc.line]; got != tc.want {
			t.Errorf("line %d tagged %q, want %q", tc.line, got, tc.want)
		}
	}
}

// tagOf is the first item claiming a row — what a row used to carry when it
// could carry only one. Tests that predate shared rows still read it.
func tagOf(ln diffViewLine) string {
	if len(ln.summaryItemIDs) == 0 {
		return ""
	}
	return ln.summaryItemIDs[0]
}

// sharedLineModel is the case a real review produced on its first outing: one
// line of prose carrying two separate fixes, so two items claim it.
func sharedLineModel(t *testing.T) diffViewModel {
	t.Helper()
	th := DefaultTheme()
	km := DefaultKeyMap()
	m := newDiffViewModel(&th, &km)
	m.width, m.height = 120, 40
	m.path = "standards.md"
	m.hunks = []types.DiffHunk{
		{OldStart: 532, OldCount: 1, NewStart: 532, NewCount: 1, Lines: []types.DiffLine{
			{Kind: types.DiffLineRemoved, Content: "old rule", OldLineNum: 532},
			{Kind: types.DiffLineAdded, Content: "narrowed claim plus the new rule", NewLineNum: 532},
		}},
		{OldStart: 600, OldCount: 1, NewStart: 600, NewCount: 1, Lines: []types.DiffLine{
			{Kind: types.DiffLineAdded, Content: "second item only", NewLineNum: 600},
		}},
	}
	m.summaryItems = []types.SummaryItem{
		{ID: "narrowed", Text: "narrow the test claim",
			Targets: []types.SummaryTarget{{Path: "standards.md", LineStart: 532, LineEnd: 532}}},
		{ID: "no-inline", Text: "no inline js or css",
			Targets: []types.SummaryTarget{
				{Path: "standards.md", LineStart: 532, LineEnd: 532},
				{Path: "standards.md", LineStart: 600, LineEnd: 600},
			}},
	}
	m.buildLines()
	return m
}

// The defect this replaced: a row remembered only its first claimant, so
// selecting the second item dropped the very line that item describes.
func TestSharedLineStaysInBothFilters(t *testing.T) {
	for _, id := range []string{"narrowed", "no-inline"} {
		m := sharedLineModel(t)
		m.activeSummaryID = id
		m.buildLines()
		found := false
		for _, ln := range m.lines {
			if ln.content == "narrowed claim plus the new rule" && !m.isHiddenBySummary(ln) {
				found = true
			}
		}
		if !found {
			t.Errorf("selecting %q hid the line it claims", id)
		}
	}
}

func TestSharedLineIsClaimedByBothItems(t *testing.T) {
	m := sharedLineModel(t)
	for _, ln := range m.lines {
		if ln.content != "narrowed claim plus the new rule" {
			continue
		}
		if len(ln.summaryItemIDs) != 2 {
			t.Fatalf("shared line claimed by %v, want both items", ln.summaryItemIDs)
		}
		if !m.summaryBarFor(ln).shared {
			t.Error("a shared line must be marked shared, or its colour reads as the whole answer")
		}
		return
	}
	t.Fatal("shared line not found")
}

// With nothing selected the bar can only show one colour, and the first
// claimant in reading order is the one it shows. With an item selected the
// question has changed: the bar answers about THAT item.
func TestSelectedItemOwnsTheSharedLinesColour(t *testing.T) {
	m := sharedLineModel(t)
	line := func(m diffViewModel) diffViewLine {
		t.Helper()
		for _, ln := range m.lines {
			if ln.content == "narrowed claim plus the new rule" {
				return ln
			}
		}
		t.Fatal("shared line not found")
		return diffViewLine{}
	}
	if got, want := m.summaryBarFor(line(m)).color, summaryColor(0); got != want {
		t.Errorf("unselected colour = %v, want the first claimant's %v", got, want)
	}
	m.activeSummaryID = "no-inline"
	m.buildLines()
	if got, want := m.summaryBarFor(line(m)).color, summaryColor(1); got != want {
		t.Errorf("colour under selection = %v, want the selected item's %v", got, want)
	}
}

// An unshared row must keep the solid bar; the broken one has to stay rare
// enough to mean something.
func TestSoleClaimKeepsTheSolidBar(t *testing.T) {
	m := sharedLineModel(t)
	for _, ln := range m.lines {
		if ln.content != "second item only" {
			continue
		}
		if b := m.summaryBarFor(ln); b.shared || b.glyph() != summaryGutterBar {
			t.Errorf("sole-claim row drew %q, want the solid bar", b.glyph())
		}
		return
	}
	t.Fatal("sole-claim line not found")
}

// The bar occupies one gutter column. A glyph that measured wider would push
// the gutter out of alignment on every shared row.
func TestSummaryBarsAreOneColumn(t *testing.T) {
	for _, g := range []string{summaryGutterBar, summarySharedBar} {
		if w := ansi.StringWidth(g); w != 1 {
			t.Errorf("%q measures %d columns, want 1", g, w)
		}
	}
}

// With an item selected, [ and ] are for that item's chunks. A jump that landed
// on another item's hunk would undo the filter the reviewer just applied.
func TestChunkJumpsSkipOtherItemsChunks(t *testing.T) {
	m := summaryModel(t)
	m.fullFile = true // whole-file mode keeps the other hunks, greyed
	m.activeSummaryID = "fix-a"
	m.buildLines()

	for i := range m.lines {
		if !m.lineHasChange(i) {
			continue
		}
		if m.outsideActiveSummary(m.lines[i]) {
			t.Fatalf("row %d belongs to another item but counts as a chunk: %q", i, m.lines[i].content)
		}
	}
}

// Crossing into a file should land on the SELECTED item's first chunk, not the
// file's — otherwise the reviewer arrives somewhere the filter says to ignore.
func TestLandingOnAFileGoesToTheItemsFirstChunk(t *testing.T) {
	m := summaryModel(t)
	m.fullFile = true
	m.activeSummaryID = "fix-b" // its range is 88-95; fix-a's is 9-12
	m.buildLines()

	m.LandOnChunkEdge(+1)
	if m.cursor < 0 || m.cursor >= len(m.lines) {
		t.Fatalf("cursor %d out of range", m.cursor)
	}
	landed := m.lines[m.cursor]
	if m.outsideActiveSummary(landed) {
		t.Errorf("landed on a row outside the selected item: %q", landed.content)
	}
	if got := newLineOf(landed); got != 0 && (got < 88 || got > 95) {
		t.Errorf("landed on new-file line %d, want one inside fix-b's range", got)
	}
}

// Without a selection nothing changes — the filter-aware skip must not alter
// ordinary navigation.
func TestChunkJumpsAreUnchangedWithNoSelection(t *testing.T) {
	m := summaryModel(t)
	m.fullFile = true
	m.buildLines()
	changes := 0
	for i := range m.lines {
		if m.lineHasChange(i) {
			changes++
		}
	}
	if changes != 4 {
		t.Errorf("counted %d change rows with no selection, want all 4", changes)
	}
}

// The sidebar walks past files the selected item says nothing about, so [ and ]
// cross straight to a file that has something to show.
func TestFileNavigationSkipsUnclaimedFiles(t *testing.T) {
	km := DefaultKeyMap()
	m := newSidebarModel(&km)
	m.files = []types.ChangedFile{{Path: "a.go"}, {Path: "unrelated.go"}, {Path: "b.go"}}
	m.summaryItems = []types.SummaryItem{
		{ID: "fix", Text: "a fix", Targets: []types.SummaryTarget{
			{Path: "a.go"}, {Path: "b.go"},
		}},
	}
	m.activeSummaryID = "fix"
	m.cursor = 0

	if m.summaryFadesRow(1) != true {
		t.Fatal("unrelated.go should be faded by the selection")
	}
	if m.summaryFadesRow(2) != false {
		t.Fatal("b.go is claimed and should not be faded")
	}

	m.navigateFile(+1)
	if m.cursor != 2 {
		t.Errorf("cursor = %d after one step, want 2 (unrelated.go skipped)", m.cursor)
	}
}

func TestFileNavigationIsUnchangedWithNoSelection(t *testing.T) {
	km := DefaultKeyMap()
	m := newSidebarModel(&km)
	m.files = []types.ChangedFile{{Path: "a.go"}, {Path: "unrelated.go"}, {Path: "b.go"}}
	m.cursor = 0
	m.navigateFile(+1)
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1 — no selection means no skipping", m.cursor)
	}
}

// A landing request must survive a file that has nothing to land on. The message
// for the file being LEFT can arrive first, and under a filter that file often
// has no chunks at all — clearing the request there loses the landing for the
// file being entered, which is what made the feature look inert.
func TestALandingRequestSurvivesAFileWithNoChunks(t *testing.T) {
	th := DefaultTheme()
	km := DefaultKeyMap()
	empty := newDiffViewModel(&th, &km)
	empty.width, empty.height = 120, 40
	if empty.LandOnChunkEdge(+1) {
		t.Error("an empty view cannot have landed on anything")
	}

	m := summaryModel(t)
	if !m.LandOnChunkEdge(+1) {
		t.Error("a view with chunks should report that it landed")
	}
}
