package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

// artifactModel is the diff view showing a sent artifact, which is where a
// round's plan or ruling lives — the part that had no item at all until targets
// could name one.
func artifactModel(t *testing.T) diffViewModel {
	t.Helper()
	th := DefaultTheme()
	km := DefaultKeyMap()
	m := newDiffViewModel(&th, &km)
	m.width, m.height = 120, 40
	m.contentMode = true
	m.contentID = "feat-10-plan"
	m.path = "content.md" // synthetic; a file target must never match it
	m.summaryItems = []types.SummaryItem{
		{ID: "ruling", Text: "decide the bigint rounding rule",
			Targets: []types.SummaryTarget{{Artifact: "feat-10-plan", LineStart: 2, LineEnd: 3}}},
		{ID: "code", Text: "fix the overflow",
			Targets: []types.SummaryTarget{{Path: "content.md"}}},
		{ID: "other", Text: "unrelated",
			Targets: []types.SummaryTarget{{Artifact: "some-other-plan"}}},
	}
	m.buildContentLines("one\ntwo\nthree\nfour")
	return m
}

func TestArtifactTargetTagsTheArtifact(t *testing.T) {
	m := artifactModel(t)
	byLine := map[int][]string{}
	for _, ln := range m.lines {
		byLine[ln.newLineNum] = ln.summaryItemIDs
	}
	if got := byLine[2]; len(got) != 1 || got[0] != "ruling" {
		t.Errorf("line 2 tagged %v, want the artifact's item", got)
	}
	if got := byLine[4]; len(got) != 0 {
		t.Errorf("line 4 tagged %v, want nothing — it is outside the range", got)
	}
}

// m.path holds a synthetic name in content mode. A file target that happened to
// match it would claim an artifact it says nothing about.
func TestFileTargetDoesNotMatchAnArtifact(t *testing.T) {
	m := artifactModel(t)
	for _, ln := range m.lines {
		for _, id := range ln.summaryItemIDs {
			if id == "code" {
				t.Fatalf("a file target claimed artifact content on line %d", ln.newLineNum)
			}
		}
	}
}

func TestArtifactTargetDoesNotMatchAnotherArtifact(t *testing.T) {
	m := artifactModel(t)
	for _, ln := range m.lines {
		for _, id := range ln.summaryItemIDs {
			if id == "other" {
				t.Fatal("an item targeting a different artifact claimed this one")
			}
		}
	}
}

// The reverse direction: viewing a FILE, an artifact target must claim nothing.
func TestArtifactTargetDoesNotMatchAFile(t *testing.T) {
	m := summaryModel(t)
	m.summaryItems = []types.SummaryItem{
		{ID: "plan", Text: "a plan item", Targets: []types.SummaryTarget{{Artifact: "a.go"}}},
	}
	m.buildLines()
	for _, ln := range m.lines {
		if len(ln.summaryItemIDs) > 0 {
			t.Fatalf("an artifact target claimed file rows: %v", ln.summaryItemIDs)
		}
	}
}

func TestSummaryModalNamesWhatTheItemPointsAt(t *testing.T) {
	m := newSummaryModalModel(DefaultTheme())
	m.width, m.height = 120, 40
	m.open(openSummaryMsg{
		overview: "One round, two halves: a defect fix and the ruling behind it.",
		items: []types.SummaryItem{
			{ID: "ruling", Text: "decide the bigint rounding rule for partial deposits, which the plan spells out at length",
				Targets: []types.SummaryTarget{{Artifact: "feat-10-plan"}}},
		},
	})
	out := m.View()
	if !strings.Contains(out, "a defect fix and the ruling behind it") {
		t.Error("the overview must be shown above the items")
	}
	if !strings.Contains(out, "artifact feat-10-plan") {
		t.Error("the detail block must name the artifact the item points at")
	}
	if !strings.Contains(out, "1 artifact") {
		t.Errorf("an artifact-only item must not be counted as a file\n%s", out)
	}
	// The row is truncated to fit; the detail block is where the whole line lives,
	// wrapped, so the comparison is against the text with its layout flattened.
	flat := stripANSISeq(out)
	for _, edge := range []string{"│", "╭", "╮", "╰", "╯", "─"} {
		flat = strings.ReplaceAll(flat, edge, " ")
	}
	flat = strings.Join(strings.Fields(flat), " ")
	if !strings.Contains(flat, "which the plan spells out at length") {
		t.Errorf("the cursor item's full text must appear somewhere\n%s", out)
	}
}

// Selecting an item used to rebuild the pane from m.hunks, which are nil while
// an artifact is on screen — so choosing any item blanked the artifact, the one
// place the item most often points.
func TestSelectingAnItemKeepsTheArtifactOnScreen(t *testing.T) {
	th := DefaultTheme()
	km := DefaultKeyMap()
	app := appModel{diffView: newDiffViewModel(&th, &km)}
	app.diffView.width, app.diffView.height = 120, 40
	app.diffView.contentMode = true
	app.diffView.contentID = "feat-10-plan"
	app.diffView.path = "content.md"
	app.diffView.contentDiffContent = "one\ntwo\nthree"
	app.diffView.buildContentLines(app.diffView.contentDiffContent)
	app.summaryItems = []types.SummaryItem{
		{ID: "ruling", Text: "decide it", Targets: []types.SummaryTarget{{Artifact: "feat-10-plan"}}},
	}
	app.diffView.summaryItems = app.summaryItems
	before := len(app.diffView.lines)

	app.selectSummaryItem("ruling")

	if got := len(app.diffView.lines); got != before {
		t.Fatalf("artifact has %d rows after selecting, had %d — the pane was emptied", got, before)
	}
}

// A filter that removes every row used to render a blank pane, which reads as a
// broken view rather than as an answer.
func TestFilteredFileWithNothingToShowSaysSo(t *testing.T) {
	m := summaryModel(t)
	m.summaryItems = append(m.summaryItems, types.SummaryItem{
		ID: "elsewhere", Text: "a fix in another file",
		Targets: []types.SummaryTarget{{Path: "other.go"}},
	})
	m.activeSummaryID = "elsewhere"
	m.buildLines()
	out := stripANSISeq(m.View())
	if !strings.Contains(out, "Nothing here for the selected summary item") {
		t.Errorf("an entirely filtered-out file must explain itself\n%s", out)
	}
	if !strings.Contains(out, "esc clears the filter") {
		t.Error("the note must name the key that undoes the filter")
	}
}

func TestUnfilteredViewNeverShowsTheEmptyNote(t *testing.T) {
	m := summaryModel(t)
	if m.summaryHidesEverything() {
		t.Error("nothing is hidden when no item is selected")
	}
}

// The modal opens on i and closes on i — a glance costs one keystroke each way.
func TestTheOpenKeyAlsoClosesTheSummary(t *testing.T) {
	m := newSummaryModalModel(DefaultTheme())
	m.width, m.height = 120, 40
	m.open(openSummaryMsg{
		items:    []types.SummaryItem{{ID: "a", Text: "a fix"}},
		openKeys: []string{"i"},
	})
	if !m.active {
		t.Fatal("modal should be open")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if m.active {
		t.Error("the key that opened the modal must also close it")
	}
}

// Closing with that key must not clear an active filter — esc is the key that
// does both, and a toggle that silently dropped the filter would be a trap.
func TestTheOpenKeyDoesNotClearTheFilter(t *testing.T) {
	m := newSummaryModalModel(DefaultTheme())
	m.width, m.height = 120, 40
	m.activeID = "a"
	m.open(openSummaryMsg{
		items:    []types.SummaryItem{{ID: "a", Text: "a fix"}},
		openKeys: []string{"i"},
	})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if cmd == nil {
		t.Fatal("expected a close command")
	}
	if _, clears := cmd().(selectSummaryItemMsg); clears {
		t.Error("the toggle key cleared the filter; only esc should do that")
	}
}

// A rebound open key must carry the toggle with it.
func TestTheToggleFollowsARebindingOfTheOpenKey(t *testing.T) {
	m := newSummaryModalModel(DefaultTheme())
	m.width, m.height = 120, 40
	m.open(openSummaryMsg{
		items:    []types.SummaryItem{{ID: "a", Text: "a fix"}},
		openKeys: []string{"w"},
	})
	got, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if !got.active {
		t.Error("i closed a modal bound to w")
	}
	got, _ = got.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	if got.active {
		t.Error("the rebound key did not close the modal")
	}
}
