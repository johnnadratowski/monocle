package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/josephschmitt/monocle/internal/core"
	"github.com/josephschmitt/monocle/internal/types"
)

const schemaPath = "api/src/schema.gen.ts"

// lockfileDiff bumps one package and adds another in a pnpm lockfile.
func lockfileDiff() *types.DiffResult {
	lines := []types.DiffLine{
		{Kind: types.DiffLineContext, OldLineNum: 10, NewLineNum: 10, Content: "packages:"},
		{Kind: types.DiffLineRemoved, OldLineNum: 11, Content: "  '@types/node@20.1.0':"},
		{Kind: types.DiffLineAdded, NewLineNum: 11, Content: "  '@types/node@20.2.0':"},
		{Kind: types.DiffLineAdded, NewLineNum: 12, Content: "  zod@3.23.8:"},
	}
	return &types.DiffResult{Path: "pnpm-lock.yaml", Hunks: []types.DiffHunk{{OldStart: 10, OldCount: 2, NewStart: 10, NewCount: 3, Lines: lines}}}
}

// generatedApp is a review of a hand-written file and two generated ones — a
// typed client the tour has a note for, and a lockfile — on a tour whose
// first stop is in the hand-written file. The engine has judged them, so the
// generated files come after the rest.
func generatedApp(t *testing.T) (appModel, *tourEngine) {
	t.Helper()
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	files := []types.ChangedFile{
		{Path: "a.go", Status: types.FileModified, Additions: 4},
		{Path: schemaPath, Status: types.FileModified, Additions: 6870, Deletions: 12, Generated: types.GeneratedName,
			GeneratedBy: "openapi-typescript"},
		{Path: "pnpm-lock.yaml", Status: types.FileModified, Additions: 1872, Deletions: 40, Generated: types.GeneratedLockfile},
	}
	tour := &types.Walkthrough{Title: "Tour",
		Stops: []types.WalkthroughStop{
			{ID: "1", Title: "Where it starts", File: "a.go", LineStart: 3, Note: "The entry."},
			{ID: "2", Title: "One line of the client", File: schemaPath, LineStart: 20, LineEnd: 22, Note: "The call."},
		},
		Generated: map[string]string{schemaPath: "Typed client generated from docs/swagger.json; regenerate with pnpm gen; CI guard G-16."},
	}
	e := &tourEngine{
		stubEngine: stubEngine{
			cfg:          inertViewers(&types.Config{}),
			changedFiles: files,
			session:      &types.ReviewSession{ID: "s", ChangedFiles: files, Walkthrough: tour, WalkthroughStop: "1"},
		},
		diffs: map[string]*types.DiffResult{"a.go": fileDiff("a.go", 10), schemaPath: fileDiff(schemaPath, 40), "pnpm-lock.yaml": lockfileDiff()},
	}
	m := NewApp(e)
	m = updateApp(t, m, tea.WindowSizeMsg{Width: 160, Height: 50})
	m = updateApp(t, m, initialLoadMsg{files: files})
	return m, e
}

// plainView is the diff pane as text, without its colours.
func plainView(m appModel) string { return ansi.Strip(m.diffView.View()) }

func openFile(t *testing.T, m appModel, path string) appModel {
	t.Helper()
	m.sidebar.selectPath(path)
	return updateApp(t, m, sidebarSelectMsg{path: path})
}

func TestAGeneratedFileOpensFoldedToACard(t *testing.T) {
	m, _ := generatedApp(t)
	m = openFile(t, m, schemaPath)
	view := plainView(m)
	for _, want := range []string{
		"Generated", schemaPath,
		"Typed client generated from docs/swagger.json", // (a) the tour's note
		"openapi-typescript",                            // (b) the header's generator
		"+6870", "-12",                                  // (d) the line counts
		"Z shows the diff",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the card does not say %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "line 1 of "+schemaPath) {
		t.Errorf("the folded file shows its diff:\n%s", view)
	}
}

func TestALockfilesCardListsItsPackageChanges(t *testing.T) {
	m, _ := generatedApp(t)
	m = openFile(t, m, "pnpm-lock.yaml")
	view := plainView(m)
	for _, want := range []string{"1 added", "1 bumped", "zod 3.23.8", "@types/node 20.1.0 → 20.2.0", "a lockfile"} {
		if !strings.Contains(view, want) {
			t.Errorf("the lockfile's card does not say %q:\n%s", want, view)
		}
	}
}

func TestOneKeyUnfoldsAGeneratedFileAndFoldsItBack(t *testing.T) {
	m, _ := generatedApp(t)
	m = openFile(t, m, schemaPath)
	m.setFocus(focusMain)
	m = pressKey(t, m, "Z")
	if view := plainView(m); !strings.Contains(view, "line 1 of "+schemaPath) {
		t.Fatalf("Z did not unfold the diff:\n%s", view)
	}
	// It stays unfolded when the reviewer comes back to it.
	m = openFile(t, m, "a.go")
	m = openFile(t, m, schemaPath)
	if view := plainView(m); !strings.Contains(view, "line 1 of "+schemaPath) {
		t.Errorf("the file folded again on coming back:\n%s", view)
	}
	m = pressKey(t, m, "Z")
	if view := plainView(m); !strings.Contains(view, "Z shows the diff") {
		t.Errorf("Z did not fold it back:\n%s", view)
	}
}

func TestZOnAHandWrittenFileSaysSo(t *testing.T) {
	m, _ := generatedApp(t)
	m = openFile(t, m, "a.go")
	if view := plainView(m); !strings.Contains(view, "line 1 of a.go") {
		t.Fatalf("a hand-written file did not open to its diff:\n%s", view)
	}
	m.setFocus(focusMain)
	m = pressKey(t, m, "Z")
	if !strings.Contains(m.statusBar.searchInfo, "not generated") {
		t.Errorf("status %q, want it to say the file is not generated", m.statusBar.searchInfo)
	}
}

// A stop (or any jump) to lines of a generated file shows those lines: the
// author chose to stop there.
func TestAStopInAGeneratedFileShowsItsLines(t *testing.T) {
	m, _ := generatedApp(t)
	m = pressKey(t, m, ".")
	if stop, _ := m.currentStop(); stop.ID != "2" {
		t.Fatalf("on stop %q, want 2", stop.ID)
	}
	if view := plainView(m); strings.Contains(view, "Z shows the diff") || cursorLine(m) != 20 {
		t.Errorf("the stop landed on the card (cursor at %d):\n%s", cursorLine(m), view)
	}
}

func TestTheFileListBadgesGeneratedFilesAfterTheRest(t *testing.T) {
	m, _ := generatedApp(t)
	for _, mode := range []string{"flat", "grouped"} {
		m.sidebar.groupMode = mode == "grouped"
		m.sidebar.treeMode = false
		m.sidebar.rebuildGroups()
		got := listedFiles(m)
		if i, j := slices.Index(got, "a.go"), slices.Index(got, schemaPath); i < 0 || j < 0 || i > j {
			t.Errorf("%s: list %q, want a.go before the generated files", mode, got)
		}
	}
	row := m.sidebar.renderFileItem(m.engine.GetChangedFiles()[1], false)
	if !strings.Contains(row, generatedBadge) {
		t.Errorf("a generated file's row %q has no %s badge", row, generatedBadge)
	}
	if row := m.sidebar.renderFileItem(m.engine.GetChangedFiles()[0], false); strings.Contains(row, generatedBadge) {
		t.Errorf("a hand-written file's row %q has the badge", row)
	}
}

// Within a group too: a generated file sorts after the hand-written files of
// its group, whatever its churn.
func TestGroupingPutsGeneratedFilesLastInTheirGroup(t *testing.T) {
	files := []types.ChangedFile{
		{Path: "src/big.gen.ts", Additions: 9000, Generated: types.GeneratedName, GroupLabel: "API"},
		{Path: "src/small.ts", Additions: 1, GroupLabel: "API"},
	}
	got, _ := groupFiles(files)
	if got[0].Path != "src/small.ts" {
		t.Errorf("group order %v, want the hand-written file first", []string{got[0].Path, got[1].Path})
	}
}

func TestAGeneratedFileNeverCountsAsUnreviewed(t *testing.T) {
	m, _ := generatedApp(t)
	m.sidebar.reviewTracking = true
	m.sidebar.reviewFilter = "unreviewed"
	m.sidebar.files = m.engine.GetChangedFiles()
	m.sidebar.applyReviewedFilter()
	if got := listedFiles(m); !slices.Equal(got, []string{"a.go"}) {
		t.Errorf("the unreviewed filter lists %q, want only a.go", got)
	}
	// Next unreviewed passes over them.
	m.sidebar.reviewFilter = ""
	m.sidebar.files = append(m.engine.GetChangedFiles(), types.ChangedFile{Path: "z.go"})
	m.sidebar.applyReviewedFilter()
	m.sidebar.selectPath("a.go")
	m.sidebar.nextUnreviewed()
	if f := m.sidebar.selectedFile(); f == nil || f.Path != "z.go" {
		t.Errorf("next unreviewed from a.go is %v, want z.go past the generated files", f)
	}
	// The status bar's progress counts them as done.
	m.sidebar.files = m.engine.GetChangedFiles()
	if m.reviewedCountForStatus() != 2 {
		t.Errorf("reviewed count %d, want the two generated files", m.reviewedCountForStatus())
	}
}

// The engine re-judges the files when a tour arrives (its notes can make a
// file generated), so the list is read again with it.
func TestATourArrivingReadsTheFilesAgain(t *testing.T) {
	m, e := generatedApp(t)
	e.changedFiles = append([]types.ChangedFile(nil), e.changedFiles...)
	e.changedFiles[0].Generated = types.GeneratedTour
	m = updateApp(t, m, tourEventMsg{status: core.WalkthroughEventSet, id: "1"})
	if f := m.sidebar.files[0]; f.Path != "a.go" || f.Generated != types.GeneratedTour {
		t.Errorf("the list holds %+v after the tour arrived, want a.go now generated", f)
	}
}

func TestTheTUIReportsGenerated(t *testing.T) {
	if !slices.Contains(Features, "generated") {
		t.Errorf("Features = %v, want generated in it, so stage sends the tour's notes", Features)
	}
}
