package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/josephschmitt/monocle/internal/core"
	"github.com/josephschmitt/monocle/internal/types"
)

// longPath is a file of the review the uncovered stop's hunks name, long
// enough that the usual file list would cut it short.
const longPath = "internal/payments/settlement/reconcile_withdrawals.go"

// uncoveredTour is two stops, then the "Not covered" stop stage adds — its
// hunks two in a.go and one in longPath, b.go fully covered — then a stop
// after it, so leaving it forwards has somewhere to go.
func uncoveredTour() *types.Walkthrough {
	return &types.Walkthrough{Title: "Tour", Stops: []types.WalkthroughStop{
		{ID: "1.1", Title: "Where it starts", File: "a.go", LineStart: 5, Note: "The entry."},
		{ID: "1.2", Title: "Where it lands", File: "b.go", LineStart: 30, Note: "The write."},
		{ID: "uncovered", Title: "Not covered", Kind: types.StopKindUncovered,
			Note:  "80% of the PR's changed lines are covered by a stop; these are not.",
			Hunks: []types.StopHunk{{File: "a.go", Start: 10, End: 12}, {File: "a.go", Start: 20, End: 22}, {File: longPath, Start: 3, End: 4}}},
		{ID: "9", Title: "After", File: "b.go", LineStart: 50, Note: "Last."},
	}}
}

// uncoveredApp is a review of a.go, b.go and longPath carrying tour, on its
// first stop, with the file list hidden as a tour hides it.
func uncoveredApp(t *testing.T, tour *types.Walkthrough) appModel {
	t.Helper()
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	files := []types.ChangedFile{{Path: "a.go", Status: types.FileAdded}, {Path: "b.go", Status: types.FileAdded}, {Path: longPath, Status: types.FileAdded}}
	e := &tourEngine{
		stubEngine: stubEngine{
			cfg:          inertViewers(&types.Config{}),
			changedFiles: files,
			session:      &types.ReviewSession{ID: "s", ChangedFiles: files, Walkthrough: tour, WalkthroughStop: tour.Stops[0].ID},
		},
		diffs: map[string]*types.DiffResult{"a.go": fileDiff("a.go", 40), "b.go": fileDiff("b.go", 60), longPath: fileDiff(longPath, 10)},
	}
	m := NewApp(e)
	m = updateApp(t, m, tea.WindowSizeMsg{Width: 200, Height: 50})
	m = updateApp(t, m, initialLoadMsg{files: files})
	if !m.sidebarHidden {
		t.Fatal("the tour did not hide the file list as it started")
	}
	return m
}

// listedFiles are the paths the file list shows.
func listedFiles(m appModel) []string {
	var out []string
	for _, f := range m.sidebar.displayFiles() {
		out = append(out, f.Path)
	}
	return out
}

// at is where the reviewer is: the stop, the file on screen, the marked range
// and the line under the cursor.
type at struct {
	stop       string
	file       string
	start, end int
	line       int
}

func whereAt(m appModel) at {
	stop, _ := m.currentStop()
	return at{stop.ID, m.diffView.path, m.diffView.stopStart, m.diffView.stopEnd, cursorLine(m)}
}

func wantAt(t *testing.T, m appModel, step string, want at) {
	t.Helper()
	if got := whereAt(m); got != want {
		t.Errorf("%s: at %+v, want %+v", step, got, want)
	}
}

// On the uncovered stop the file list comes up, holding only the files its
// hunks are in; leaving the stop puts the list back as the tour had it.
func TestTheUncoveredStopListsOnlyItsFiles(t *testing.T) {
	m := typeCommand(t, uncoveredApp(t, uncoveredTour()), "stop uncovered")
	if m.sidebarHidden {
		t.Fatal("the file list is hidden on the uncovered stop")
	}
	if got, want := listedFiles(m), []string{"a.go", longPath}; !slices.Equal(got, want) {
		t.Errorf("the file list shows %v, want %v", got, want)
	}
	if !strings.Contains(screenText(m), "reconcile_withdrawals.go") {
		t.Error("the uncovered file is not on screen")
	}
	m = typeCommand(t, m, "stop 1.2")
	if !m.sidebarHidden {
		t.Error("the file list stayed up after leaving the uncovered stop")
	}
	if got := listedFiles(m); len(got) != 3 {
		t.Errorf("after leaving, the file list holds %v, want all three files", got)
	}
}

// Switching the tour off leaves the stop too, and so the list goes back.
func TestTourOffPutsTheFileListBack(t *testing.T) {
	m := typeCommand(t, uncoveredApp(t, uncoveredTour()), "stop uncovered")
	m = pressKey(t, m, "W")
	if !m.sidebarHidden || len(listedFiles(m)) != 3 {
		t.Errorf("tour off: file list hidden %v holding %v, want it hidden and whole again", m.sidebarHidden, listedFiles(m))
	}
}

// The reviewer's own file list stays theirs: shown before the stop, it is
// still shown after it.
func TestAFileListTheReviewerShowedStaysShown(t *testing.T) {
	m := uncoveredApp(t, uncoveredTour())
	m = pressKey(t, m, ";")
	if m.sidebarHidden {
		t.Fatal("; did not show the file list")
	}
	m = typeCommand(t, m, "stop uncovered")
	m = typeCommand(t, m, "stop 1.2")
	if m.sidebarHidden {
		t.Error("leaving the uncovered stop hid a file list the reviewer had shown")
	}
}

// Hiding and showing the list on the stop makes it the reviewer's: leaving
// the stop keeps it shown.
func TestAListTheReviewerToggledOnTheStopStaysShown(t *testing.T) {
	m := typeCommand(t, uncoveredApp(t, uncoveredTour()), "stop uncovered")
	m = pressKey(t, m, ";")
	m = pressKey(t, m, ";")
	m = typeCommand(t, m, "stop 1.2")
	if m.sidebarHidden {
		t.Error("leaving the stop hid a list the reviewer had shown on it")
	}
}

// A tour re-sent while the reviewer is on the stop keeps the change they are
// on, held to the hunks it still has, and the list follows the new hunks.
func TestAReSentTourKeepsTheHunk(t *testing.T) {
	tour := uncoveredTour()
	m := uncoveredApp(t, tour)
	e := m.engine.(*tourEngine)
	m = typeCommand(t, m, "stop uncovered")
	m = pressKey(t, m, ".")
	m = updateApp(t, m, tourEventMsg{status: core.WalkthroughEventSet, id: "uncovered"})
	wantAt(t, m, "the same tour again", at{"uncovered", "a.go", 20, 22, 20})

	m = pressKey(t, m, ".")
	fewer := uncoveredTour()
	fewer.Stops[2].Hunks = fewer.Stops[2].Hunks[:2]
	e.session.Walkthrough = fewer
	m = updateApp(t, m, tourEventMsg{status: core.WalkthroughEventSet, id: "uncovered"})
	wantAt(t, m, "a tour with fewer hunks", at{"uncovered", "a.go", 20, 22, 20})
	if got := m.statusBar.tourLabel; !strings.Contains(got, "hunk 2 of 2") {
		t.Errorf("status %q, want hunk 2 of 2", got)
	}
	if got := listedFiles(m); !slices.Equal(got, []string{"a.go"}) {
		t.Errorf("the file list shows %v, want only a.go", got)
	}
}

// The agent can still show any file of the review while the list holds only
// the uncovered ones.
func TestGotoLineReachesACoveredFile(t *testing.T) {
	m := typeCommand(t, uncoveredApp(t, uncoveredTour()), "stop uncovered")
	m = updateApp(t, m, gotoLineMsg{path: "b.go", line: 42})
	if m.diffView.path != "b.go" || cursorLine(m) != 42 {
		t.Errorf("goto_line b.go:42 is at %s:%d (%q)", m.diffView.path, cursorLine(m), m.statusBar.searchInfo)
	}
}

// The list is as wide as the longest file it holds needs, so no path is cut.
func TestTheUncoveredFileListFitsItsPaths(t *testing.T) {
	m := typeCommand(t, uncoveredApp(t, uncoveredTour()), "stop uncovered")
	if m.layout != layoutHorizontal {
		t.Fatalf("layout %v, want side by side at 200 columns", m.layout)
	}
	if need := len(longPath) + sidebarRowChrome; m.sidebar.width < need {
		t.Errorf("file list %d columns wide, want at least %d for %s", m.sidebar.width, need, longPath)
	}
	if m.diffView.width < m.minDiffWidth {
		t.Errorf("the diff is %d columns, under its minimum %d", m.diffView.width, m.minDiffWidth)
	}
}

func TestUncoveredListWidth(t *testing.T) {
	for _, c := range []struct {
		longest, room, want int
	}{
		{4, 200, 30},                      // short paths: the list's usual floor
		{40, 200, 40 + sidebarRowChrome},  // a long one: room for it
		{120, 200, maxUncoveredListWidth}, // a very long one: held to the most
		{40, 45, 45},                      // little room: what the diff leaves
	} {
		if got := uncoveredListWidth(c.longest, c.room); got != c.want {
			t.Errorf("uncoveredListWidth(%d, %d) = %d, want %d", c.longest, c.room, got, c.want)
		}
	}
}

// . and , step through the hunks, one at a time, each marked as a stop's
// lines are; past the last hunk . is the tour's ., and before the first , is.
// Stepping back into the stop lands on its last hunk, as reading backwards.
func TestDotAndCommaStepThroughTheHunks(t *testing.T) {
	m := typeCommand(t, uncoveredApp(t, uncoveredTour()), "stop uncovered")
	wantAt(t, m, "arriving", at{"uncovered", "a.go", 10, 12, 10})
	m = pressKey(t, m, ".")
	wantAt(t, m, "., hunk 2", at{"uncovered", "a.go", 20, 22, 20})
	m = pressKey(t, m, ".")
	wantAt(t, m, "., hunk 3", at{"uncovered", longPath, 3, 4, 3})
	m = pressKey(t, m, ",")
	wantAt(t, m, ", back to hunk 2", at{"uncovered", "a.go", 20, 22, 20})
	m = pressKey(t, m, ".")
	m = pressKey(t, m, ".")
	wantAt(t, m, ". past the last hunk", at{"9", "b.go", 50, 50, 50})
	m = pressKey(t, m, ",")
	wantAt(t, m, ", back into the stop: its last hunk", at{"uncovered", longPath, 3, 4, 3})
	m = pressKey(t, m, ",")
	m = pressKey(t, m, ",")
	m = pressKey(t, m, ",")
	wantAt(t, m, ", before the first hunk", at{"1.2", "b.go", 30, 30, 30})
}

// > and < step a file at a time, landing on the file's first hunk; at either
// end they stay and say so.
func TestAngleBracketsStepByFile(t *testing.T) {
	m := typeCommand(t, uncoveredApp(t, uncoveredTour()), "stop uncovered")
	m = pressKey(t, m, ">")
	wantAt(t, m, "> to the next file", at{"uncovered", longPath, 3, 4, 3})
	m = pressKey(t, m, ">")
	wantAt(t, m, "> on the last file", at{"uncovered", longPath, 3, 4, 3})
	if !strings.Contains(m.statusBar.searchInfo, "last file") {
		t.Errorf("> on the last file says %q", m.statusBar.searchInfo)
	}
	m = pressKey(t, m, "<")
	wantAt(t, m, "< to the file before", at{"uncovered", "a.go", 10, 12, 10})
	m = pressKey(t, m, ".")
	m = pressKey(t, m, "<")
	wantAt(t, m, "< on the first file", at{"uncovered", "a.go", 20, 22, 20})
	if !strings.Contains(m.statusBar.searchInfo, "first file") {
		t.Errorf("< on the first file says %q", m.statusBar.searchInfo)
	}
}

// Off the uncovered stop, < and > move nothing.
func TestAngleBracketsElsewhereMoveNothing(t *testing.T) {
	m := uncoveredApp(t, uncoveredTour())
	before := whereAt(m)
	m = pressKey(t, m, ">")
	if got := whereAt(m); got != before {
		t.Errorf("> on stop 1.1 moved from %+v to %+v", before, got)
	}
}

// The tour keys reach the hunks with the note focused too, as they reach the
// stops from there.
func TestTheHunkKeysWorkFromTheNote(t *testing.T) {
	m := typeCommand(t, uncoveredApp(t, uncoveredTour()), "stop uncovered")
	m.setFocus(focusDoc)
	m = pressKey(t, m, ">")
	wantAt(t, m, "> from the note", at{"uncovered", longPath, 3, 4, 3})
	m = pressKey(t, m, "<")
	wantAt(t, m, "< from the note", at{"uncovered", "a.go", 10, 12, 10})
}

// The note's header and the status bar say which hunk is on screen.
func TestTheHeaderAndStatusNameTheHunk(t *testing.T) {
	m := typeCommand(t, uncoveredApp(t, uncoveredTour()), "stop uncovered")
	if loc := m.docPane.titleLoc; loc.path != "a.go" || loc.lines != "10–12" {
		t.Errorf("arriving, the note's header says %+v, want a.go 10–12", loc)
	}
	m = pressKey(t, m, ">")
	if loc := m.docPane.titleLoc; loc.path != longPath || loc.lines != "3–4" {
		t.Errorf("the note's header says %+v, want %s 3–4", loc, longPath)
	}
	if got := m.statusBar.tourLabel; !strings.Contains(got, "hunk 3 of 3") {
		t.Errorf("status %q, want the hunk's place in it", got)
	}
}

// With everything covered the stop is its note alone: the list stays as it
// was, and . leaves it.
func TestAnUncoveredStopWithNoHunks(t *testing.T) {
	tour := uncoveredTour()
	tour.Stops[2].Hunks, tour.Stops[2].Note = nil, "Every change is covered by a stop."
	m := typeCommand(t, uncoveredApp(t, tour), "stop uncovered")
	if !m.sidebarHidden {
		t.Error("an uncovered stop with nothing uncovered showed the file list")
	}
	m = pressKey(t, m, ".")
	if stop, _ := m.currentStop(); stop.ID != "9" {
		t.Errorf(". from an empty uncovered stop is on %q, want 9", stop.ID)
	}
}

// A driver asks the running TUI whether it knows the uncovered stop before
// sending one.
func TestTheTUIReportsTheUncoveredStop(t *testing.T) {
	if !slices.Contains(Features, types.StopKindUncovered) {
		t.Errorf("Features = %v, want %q in it", Features, types.StopKindUncovered)
	}
}
