package tui

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/josephschmitt/monocle/internal/core"
	"github.com/josephschmitt/monocle/internal/types"
)

// tourEngine is the stub engine plus what a tour needs: diffs to land in, and
// a record of the stops the TUI reported.
type tourEngine struct {
	stubEngine
	diffs map[string]*types.DiffResult
	// fullDiffs, when set for a path, are its whole-file diffs; otherwise the
	// whole-file diff is the same as the compact one.
	fullDiffs map[string]*types.DiffResult

	mu       sync.Mutex
	reported []string
}

func (e *tourEngine) GetFileDiff(path string) (*types.DiffResult, error) {
	if d, ok := e.diffs[path]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("no diff for %s", path)
}
func (e *tourEngine) GetFileDiffFull(path string) (*types.DiffResult, error) {
	if d, ok := e.fullDiffs[path]; ok {
		return d, nil
	}
	return e.GetFileDiff(path)
}
func (e *tourEngine) RefreshChangedFiles() ([]types.ChangedFile, error) {
	return e.changedFiles, nil
}
func (e *tourEngine) GetFileContent(path string) (string, error) {
	return "the contents of " + path, nil
}
func (e *tourEngine) GetSocketPath() string   { return "" }
func (e *tourEngine) GetSubscriberCount() int { return 0 }
func (e *tourEngine) SetWalkthroughStop(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.reported = append(e.reported, id)
	if e.session != nil {
		e.session.WalkthroughStop = id
	}
	return nil
}
func (e *tourEngine) reports() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.reported...)
}

// fileDiff is an all-added file of n lines, so every new-file line is on screen.
func fileDiff(path string, n int) *types.DiffResult {
	h := types.DiffHunk{OldStart: 0, OldCount: 0, NewStart: 1, NewCount: n, Header: "@@ -0,0 +1," + fmt.Sprint(n) + " @@"}
	for i := 1; i <= n; i++ {
		h.Lines = append(h.Lines, types.DiffLine{Kind: types.DiffLineAdded, NewLineNum: i, Content: fmt.Sprintf("line %d of %s", i, path)})
	}
	return &types.DiffResult{Path: path, Hunks: []types.DiffHunk{h}}
}

func testTour() *types.Walkthrough {
	return &types.Walkthrough{Title: "Tour", Stops: []types.WalkthroughStop{
		{ID: "1.1", Title: "Where it starts", File: "a.go", LineStart: 5, LineEnd: 7, Note: "The **entry** point."},
		{ID: "1.2", Title: "Where it lands", File: "b.go", LineStart: 30, LineEnd: 31, Note: "The write.",
			Related: []types.DocRef{{Kind: types.DocRefFile, Doc: "a.go", StartLine: 5}},
			Views:   []types.StopView{{Kind: types.StopViewVideo, Target: "demo.webm", Label: "Demo"}}},
		{ID: "2", Title: "Just a recording"},
	}}
}

// tourApp is a sized app over a review of a.go and b.go carrying testTour, with
// the initial load already processed.
func tourApp(t *testing.T) (appModel, *tourEngine) {
	t.Helper()
	return tourAppWith(t, testTour(), &types.Config{})
}

// tourAppWith is tourApp over another tour and config.
func tourAppWith(t *testing.T, tour *types.Walkthrough, cfg *types.Config) (appModel, *tourEngine) {
	t.Helper()
	// Never let a test reach a real tmux server: these tests may themselves be
	// running inside one, and a stop's side effects split and kill panes.
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	files := []types.ChangedFile{{Path: "a.go", Status: types.FileAdded}, {Path: "b.go", Status: types.FileAdded}}
	e := &tourEngine{
		stubEngine: stubEngine{
			cfg:          cfg,
			changedFiles: files,
			session:      &types.ReviewSession{ID: "s", ChangedFiles: files, Walkthrough: tour, WalkthroughStop: tour.Stops[0].ID},
		},
		diffs: map[string]*types.DiffResult{"a.go": fileDiff("a.go", 40), "b.go": fileDiff("b.go", 60)},
	}
	m := NewApp(e)
	m = updateApp(t, m, tea.WindowSizeMsg{Width: 140, Height: 44})
	m = updateApp(t, m, initialLoadMsg{files: files})
	return m, e
}

// updateApp feeds one message and then drives every command it produces to
// completion, the way the Bubble Tea runtime would. A command that has not
// answered within a moment is a timer (a refresh tick, a debounce) and is
// dropped: nothing here waits on one.
func updateApp(t *testing.T, m appModel, msg tea.Msg) appModel {
	t.Helper()
	next, cmd := m.Update(msg)
	m = next.(appModel)
	return drive(t, m, cmd, 0)
}

func drive(t *testing.T, m appModel, cmd tea.Cmd, depth int) appModel {
	t.Helper()
	return driveWithin(t, m, cmd, depth, 50*time.Millisecond)
}

// driveWithin is drive, waiting up to wait for each command: long enough for
// one that runs a real process, which drive would take for a timer and drop.
func driveWithin(t *testing.T, m appModel, cmd tea.Cmd, depth int, wait time.Duration) appModel {
	t.Helper()
	if cmd == nil {
		return m
	}
	if depth > 20 {
		t.Fatal("command chain did not settle")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(wait):
		return m
	}
	switch msg := msg.(type) {
	case nil:
		return m
	case tea.BatchMsg:
		for _, c := range msg {
			m = driveWithin(t, m, c, depth+1, wait)
		}
		return m
	default:
		next, c := m.Update(msg)
		return driveWithin(t, next.(appModel), c, depth+1, wait)
	}
}

func pressKey(t *testing.T, m appModel, key string) appModel {
	t.Helper()
	var msg tea.KeyPressMsg
	switch key {
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	default:
		msg = tea.KeyPressMsg{Code: rune(key[0]), Text: key}
	}
	return updateApp(t, m, msg)
}

func typeCommand(t *testing.T, m appModel, command string) appModel {
	t.Helper()
	m = pressKey(t, m, ":")
	for _, r := range command {
		if r == ' ' {
			m = updateApp(t, m, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
			continue
		}
		m = pressKey(t, m, string(r))
	}
	return pressKey(t, m, "enter")
}

// cursorLine is the new-file line under the diff cursor.
func cursorLine(m appModel) int { return m.diffView.lineNumAt(m.diffView.cursor) }

func TestTourRestoresOnLoad(t *testing.T) {
	m, e := tourApp(t)
	if !m.tour.on || m.tour.index != 0 {
		t.Fatalf("tour on=%v index=%d, want on at the first stop", m.tour.on, m.tour.index)
	}
	if m.diffView.path != "a.go" || cursorLine(m) != 5 {
		t.Errorf("diff at %s:%d, want a.go:5", m.diffView.path, cursorLine(m))
	}
	// Restoring is not moving: the engine already has this stop.
	if got := e.reports(); len(got) != 0 {
		t.Errorf("reported %v on restore, want nothing", got)
	}
}

func TestEnteringAStopShowsIt(t *testing.T) {
	m, e := tourApp(t)
	m = pressKey(t, m, ".")

	if m.diffView.path != "b.go" || cursorLine(m) != 30 {
		t.Fatalf("diff at %s:%d, want b.go:30 (the stop's LineStart)", m.diffView.path, cursorLine(m))
	}
	if !m.docPane.active || !m.docPane.note {
		t.Fatal("the stop's note should be open in the doc pane")
	}
	doc := stripANSISeq(m.docPane.View())
	for _, want := range []string{"1.2 · Where it lands", "The write.", "a.go:5", "video Demo"} {
		if !strings.Contains(doc, want) {
			t.Errorf("doc pane %q is missing %q", doc, want)
		}
	}
	if got := stripANSISeq(m.statusBar.View()); !strings.Contains(got, "tour 1.2 · 2 of 3") {
		t.Errorf("status bar %q should show tour 1.2 · 2 of 3", got)
	}
	// The stop's range is marked, and only the range.
	marked := map[int]bool{}
	for _, ln := range m.diffView.lines {
		if m.diffView.inStopRange(ln) {
			marked[ln.newLineNum] = true
		}
	}
	if len(marked) != 2 || !marked[30] || !marked[31] {
		t.Errorf("marked lines %v, want exactly 30-31", marked)
	}
	if got := e.reports(); len(got) != 1 || got[0] != "1.2" {
		t.Errorf("reported %v, want the move to 1.2", got)
	}
}

func TestTourSteppingClampsAtTheEnds(t *testing.T) {
	m, _ := tourApp(t)

	m = pressKey(t, m, ",")
	if m.tour.index != 0 || m.statusBar.searchInfo != "start of tour" {
		t.Errorf("at the first stop , gave index %d %q, want 0 and a notice", m.tour.index, m.statusBar.searchInfo)
	}

	m = pressKey(t, m, ".")
	m = pressKey(t, m, ".")
	if m.tour.index != 2 {
		t.Fatalf("index %d after two steps, want 2", m.tour.index)
	}
	// A stop with no file leaves the diff where it was.
	if m.diffView.path != "b.go" {
		t.Errorf("a stop with no anchor moved the diff to %q", m.diffView.path)
	}
	if !strings.Contains(stripANSISeq(m.docPane.View()), "2 · Just a recording") {
		t.Error("an anchorless stop still shows its note")
	}

	m = pressKey(t, m, ".")
	if m.tour.index != 2 || m.statusBar.searchInfo != "end of tour" {
		t.Errorf("at the last stop . gave index %d %q, want 2 and a notice", m.tour.index, m.statusBar.searchInfo)
	}

	m = pressKey(t, m, ",")
	if m.tour.index != 1 || m.diffView.path != "b.go" {
		t.Errorf(", gave index %d at %s, want 1 at b.go", m.tour.index, m.diffView.path)
	}
}

func TestStopCommand(t *testing.T) {
	m, _ := tourApp(t)
	m = typeCommand(t, m, "stop 1.2")
	if m.tour.index != 1 || m.diffView.path != "b.go" || cursorLine(m) != 30 {
		t.Errorf(":stop 1.2 left index %d at %s:%d", m.tour.index, m.diffView.path, cursorLine(m))
	}
	m = typeCommand(t, m, "stop 9")
	if m.tour.index != 1 || !strings.Contains(m.statusBar.searchInfo, `no stop "9"`) {
		t.Errorf(":stop 9 gave index %d %q, want no move and a notice", m.tour.index, m.statusBar.searchInfo)
	}
	// A chapter's number goes to its first stop; an exact id still wins (2 is a stop of its own).
	m = typeCommand(t, m, "stop 1")
	if m.tour.index != 0 {
		t.Errorf(":stop 1 gave index %d, want 1.1, the chapter's first stop", m.tour.index)
	}
	m = typeCommand(t, m, "stop 2")
	if m.tour.index != 2 {
		t.Errorf(":stop 2 gave index %d, want the stop 2 itself", m.tour.index)
	}
	// No id goes to the tour's first stop.
	m = typeCommand(t, m, "stop")
	if m.tour.index != 0 {
		t.Errorf(":stop with no id gave index %d, want the first stop", m.tour.index)
	}
}

// A chapter prefix stops at a dot: :stop 1 is never 10.1.
func TestStopChapterNeedsTheDot(t *testing.T) {
	tour := &types.Walkthrough{Title: "Tour", Stops: []types.WalkthroughStop{
		{ID: "10.1", Title: "Ten", File: "a.go", LineStart: 5},
		{ID: "1.4", Title: "One four", File: "b.go", LineStart: 30},
	}}
	m, _ := tourAppWith(t, tour, &types.Config{})
	m = typeCommand(t, m, "stop 1")
	if m.tour.index != 1 {
		t.Errorf(":stop 1 gave index %d, want 1.4, not 10.1", m.tour.index)
	}
}

func TestToggleTour(t *testing.T) {
	m, _ := tourApp(t)
	m = pressKey(t, m, ".")

	m = pressKey(t, m, "W")
	if m.tour.on || m.docPane.active || m.statusBar.tourLabel != "" || m.diffView.stopPath != "" {
		t.Fatalf("tour off left on=%v doc=%v label=%q marks=%q", m.tour.on, m.docPane.active, m.statusBar.tourLabel, m.diffView.stopPath)
	}
	// Off keeps the stop, so back on resumes there rather than at the start.
	m = pressKey(t, m, "W")
	if !m.tour.on || m.tour.index != 1 || !m.docPane.active {
		t.Errorf("tour back on at index %d doc=%v, want 1 with its note", m.tour.index, m.docPane.active)
	}

	// With the tour off, a step resumes it where it was instead of moving on.
	m = pressKey(t, m, "W")
	m = pressKey(t, m, ".")
	if !m.tour.on || m.tour.index != 1 {
		t.Errorf(". with the tour off gave on=%v index %d, want the tour back on at 1", m.tour.on, m.tour.index)
	}
}

func TestTourKeysWorkFromTheNotePane(t *testing.T) {
	m, _ := tourApp(t)
	m.setFocus(focusDoc)
	m = pressKey(t, m, ".")
	if m.tour.index != 1 {
		t.Errorf("index %d, want . to step while the note has focus", m.tour.index)
	}
}

func TestNoTourIsANotice(t *testing.T) {
	m, e := tourApp(t)
	e.session.Walkthrough = nil
	m.syncTour(e.session)
	for _, key := range []string{".", ",", "W"} {
		m = pressKey(t, m, key)
		if !strings.Contains(m.statusBar.searchInfo, "no tour") {
			t.Errorf("%s with no tour said %q", key, m.statusBar.searchInfo)
		}
	}
}

func TestAgentMovesTheTour(t *testing.T) {
	t.Run("goto_stop shows the stop without echoing it back", func(t *testing.T) {
		m, e := tourApp(t)
		e.session.WalkthroughStop = "1.2"
		m = updateApp(t, m, tourEventMsg{status: core.WalkthroughEventGoto, id: "1.2"})
		if m.tour.index != 1 || m.diffView.path != "b.go" || cursorLine(m) != 30 {
			t.Errorf("goto left index %d at %s:%d", m.tour.index, m.diffView.path, cursorLine(m))
		}
		if got := e.reports(); len(got) != 0 {
			t.Errorf("reported %v back to the engine, want nothing", got)
		}
	})

	// Found end to end: the agent's move arrived under the previous key's
	// "end of tour", which only a keypress would have cleared.
	t.Run("an agent's move clears a stale notice", func(t *testing.T) {
		m, _ := tourApp(t)
		m = pressKey(t, m, ",")
		if m.statusBar.searchInfo != "start of tour" {
			t.Fatalf("setup: notice %q", m.statusBar.searchInfo)
		}
		m = updateApp(t, m, tourEventMsg{status: core.WalkthroughEventGoto, id: "1.2"})
		if m.statusBar.searchInfo != "" {
			t.Errorf("notice %q survived the move to 1.2", m.statusBar.searchInfo)
		}
	})

	t.Run("a replaced tour keeps the reviewer's stop and its new note", func(t *testing.T) {
		m, e := tourApp(t)
		m = pressKey(t, m, ".")
		tour := testTour()
		tour.Stops[1].Note = "Rewritten."
		e.session.Walkthrough = tour
		m = updateApp(t, m, tourEventMsg{status: core.WalkthroughEventSet, id: "1.2"})
		if m.tour.index != 1 || !strings.Contains(stripANSISeq(m.docPane.View()), "Rewritten.") {
			t.Errorf("index %d, doc %q", m.tour.index, stripANSISeq(m.docPane.View()))
		}
	})

	t.Run("a withdrawn tour leaves the screen", func(t *testing.T) {
		m, e := tourApp(t)
		e.session.Walkthrough = nil
		m = updateApp(t, m, tourEventMsg{status: core.WalkthroughEventCleared})
		if m.hasTour() || m.tour.on || m.docPane.active || m.statusBar.tourLabel != "" {
			t.Errorf("tour still showing: has=%v on=%v doc=%v label=%q", m.hasTour(), m.tour.on, m.docPane.active, m.statusBar.tourLabel)
		}
	})
}

// A refresh the agent triggered a moment before a tour jump can finish after
// it. Landing that load would put the stop's note under the wrong file.
func TestAStaleLoadCannotBeatTheTourJump(t *testing.T) {
	m, e := tourApp(t) // on 1.1, showing a.go
	// A stop is waiting on a.go when a load for b.go — a refresh the agent
	// triggered a moment earlier — finishes first.
	m.tour.loading = "a.go"
	m = updateApp(t, m, loadDiffMsg{path: "b.go", result: e.diffs["b.go"]})
	if m.diffView.path != "a.go" || m.tour.loading != "a.go" {
		t.Fatalf("the stale b.go load landed: showing %s, waiting on %q", m.diffView.path, m.tour.loading)
	}
	m.pendingJumpLine = 7
	m = updateApp(t, m, loadDiffMsg{path: "a.go", result: e.diffs["a.go"]})
	if m.tour.loading != "" || m.diffView.path != "a.go" || cursorLine(m) != 7 {
		t.Errorf("after the tour's own load: loading=%q at %s:%d", m.tour.loading, m.diffView.path, cursorLine(m))
	}
	// With nothing waiting, loads land as they always did.
	m = updateApp(t, m, loadDiffMsg{path: "b.go", result: e.diffs["b.go"]})
	if m.diffView.path != "b.go" {
		t.Errorf("an ordinary load was dropped: showing %s", m.diffView.path)
	}
}

func TestStopNoteBody(t *testing.T) {
	got := stopNoteBody(types.WalkthroughStop{
		Note:    "  why  ",
		Related: []types.DocRef{{Doc: "a.go", StartLine: 4}, {Doc: "b.go"}},
		Views:   []types.StopView{{Kind: "image", Target: "shots/x.png"}, {Kind: "url", Target: "https://x.test", Label: "Spec"}},
	})
	want := "why" // the related files and views follow as labels
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if got := stopNoteBody(types.WalkthroughStop{}); got != "_(no note)_" {
		t.Errorf("an empty stop says %q", got)
	}
}

func TestNotePaneWrapsAndSizesToItsNote(t *testing.T) {
	th := DefaultTheme()
	d := docPaneModel{width: 24, height: 10, theme: &th}
	d.openNote("tour:1", "1 · T", "one two three four five six seven eight nine ten", nil, nil)
	d.reflow()
	if len(d.lines) < 2 {
		t.Fatalf("lines %q, want the note wrapped to the pane", d.lines)
	}
	for _, l := range d.lines {
		if w := len([]rune(l)); w > 24 {
			t.Errorf("row %q is %d wide in a 24-column pane", l, w)
		}
	}
	if got := d.noteHeight(24); got != len(d.lines)+2 {
		t.Errorf("noteHeight = %d, want its rows plus the title and its rule", got)
	}
	d.close()
	if d.note || d.active {
		t.Error("close should leave note mode")
	}
}

// commentEngine records the targets comments were added with.
type commentEngine struct {
	*tourEngine
	targets []core.CommentTarget
}

func (e *commentEngine) AddComment(target core.CommentTarget, ct types.CommentType, body string) (*types.ReviewComment, error) {
	e.targets = append(e.targets, target)
	return &types.ReviewComment{ID: "c", StopID: target.StopID}, nil
}
func (e *commentEngine) ResolveComment(string) error { return nil }
func (e *commentEngine) DeleteComment(string) error  { return nil }

func TestCommentsAreTaggedWithTheStop(t *testing.T) {
	m, e := tourApp(t)
	ce := &commentEngine{tourEngine: e}
	m.engine = ce
	save := saveCommentMsg{path: "a.go", lineStart: 10, lineEnd: 10, targetType: types.TargetFile, commentType: types.CommentQuestion, body: "why?"}

	m.handleSaveComment(save)()
	m = pressKey(t, m, "W") // tour off
	m.handleSaveComment(save)()

	if len(ce.targets) != 2 || ce.targets[0].StopID != "1.1" || ce.targets[1].StopID != "" {
		t.Errorf("targets = %+v, want 1.1 during the tour and nothing after it", ce.targets)
	}
}

func TestInlineCommentShowsItsStop(t *testing.T) {
	got := formatInlineComment(&types.ReviewComment{Type: types.CommentQuestion, Body: "why?", StopID: "1.2"})
	if !strings.Contains(got, "[1.2] QUESTION") {
		t.Errorf("inline comment %q should name its stop", got)
	}
}

// The label carries the stop's id AND its position, because an id is not a
// position: "3.2 / 9" read as a fraction and left the reader asking what the 9
// counted.
func TestTourLabelIsIDThenPosition(t *testing.T) {
	m, _ := tourApp(t)
	for i, want := range []string{"tour 1.1 · 1 of 3", "tour 1.2 · 2 of 3", "tour 2 · 3 of 3"} {
		if i > 0 {
			m = pressKey(t, m, ".")
		}
		if got := m.statusBar.tourLabel; got != want {
			t.Errorf("stop %d: label %q, want %q", i+1, got, want)
		}
	}
	m = pressKey(t, m, "W")
	if m.statusBar.tourLabel != "" {
		t.Errorf("tour off still labelled %q", m.statusBar.tourLabel)
	}
}

// noTourApp is tourApp's review with no tour yet: the agent sends one later.
func noTourApp(t *testing.T) (appModel, *tourEngine) {
	t.Helper()
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	files := []types.ChangedFile{{Path: "a.go", Status: types.FileAdded}, {Path: "b.go", Status: types.FileAdded}}
	e := &tourEngine{
		stubEngine: stubEngine{
			cfg:          &types.Config{},
			changedFiles: files,
			session:      &types.ReviewSession{ID: "s", ChangedFiles: files},
		},
		diffs: map[string]*types.DiffResult{"a.go": fileDiff("a.go", 40), "b.go": fileDiff("b.go", 60)},
	}
	m := NewApp(e)
	m = updateApp(t, m, tea.WindowSizeMsg{Width: 140, Height: 44})
	m = updateApp(t, m, initialLoadMsg{files: files})
	return m, e
}

func TestATourStartingHidesTheFileList(t *testing.T) {
	m, e := noTourApp(t)
	m.setFocus(focusSidebar)
	if m.sidebarHidden {
		t.Fatal("the file list is hidden before any tour")
	}
	sendTour := func(m appModel) appModel {
		e.session.Walkthrough, e.session.WalkthroughStop = testTour(), "1.1"
		return updateApp(t, m, tourEventMsg{status: core.WalkthroughEventSet, id: "1.1"})
	}
	m = sendTour(m)
	if !m.sidebarHidden || m.focus == focusSidebar {
		t.Fatalf("a tour starting left the file list hidden=%v, focus %v", m.sidebarHidden, m.focus)
	}
	// Items arriving must not bring it back: that is the empty-review
	// auto-hide's undo, not ours.
	if m.autoToggleSidebar(); m.sidebarHidden != true {
		t.Error("a refresh brought the file list back")
	}
	// The reviewer's toggle shows it, and from then on it is theirs: a
	// re-sent tour, a goto, and W off and on all leave it shown.
	m = pressKey(t, m, ";")
	if m.sidebarHidden {
		t.Fatal("; did not show the file list")
	}
	m = sendTour(m)
	m = updateApp(t, m, tourEventMsg{status: core.WalkthroughEventGoto, id: "1.2"})
	m = pressKey(t, pressKey(t, m, "W"), "W")
	if m.sidebarHidden {
		t.Error("a re-sent tour, a goto or W hid the file list again")
	}
	// W off does not hide it either.
	if m = pressKey(t, m, "W"); m.sidebarHidden {
		t.Error("W off hid the file list")
	}
}

func TestATourRestoredOnLaunchHidesTheFileList(t *testing.T) {
	if m, _ := tourApp(t); !m.sidebarHidden {
		t.Error("restoring a tour left the file list shown")
	}
	if m, _ := noTourApp(t); m.sidebarHidden {
		t.Error("a review with no tour hid the file list")
	}
}

// A stop with no file does not move focus to the diff, so hiding the list must:
// focus left on a hidden list would swallow every key.
func TestHidingTheFileListTakesFocusOffIt(t *testing.T) {
	m, e := noTourApp(t)
	m.setFocus(focusSidebar)
	e.session.Walkthrough, e.session.WalkthroughStop = testTour(), "2"
	m = updateApp(t, m, tourEventMsg{status: core.WalkthroughEventSet, id: "2"})
	if stop, _ := m.currentStop(); stop.File != "" || !m.sidebarHidden || m.focus == focusSidebar {
		t.Errorf("on stop %q (file %q): hidden=%v focus=%v, want hidden and focus off it", stop.ID, stop.File, m.sidebarHidden, m.focus)
	}
}

func TestStopHistory(t *testing.T) {
	exists := func(string) bool { return true }
	var h stopHistory
	if _, ok := h.walk(-1, exists); ok {
		t.Fatal("an empty history went back")
	}
	for _, id := range []string{"1.1", "1.2", "1.2", "2.1"} {
		h.visit(id)
	}
	if want := []string{"1.1", "1.2", "2.1"}; !reflect.DeepEqual(h.ids, want) {
		t.Fatalf("history %v, want %v (re-entering the current stop records nothing)", h.ids, want)
	}
	walk := func(dir int) string {
		id, ok := h.walk(dir, exists)
		if !ok {
			return "-"
		}
		return id
	}
	if got := []string{walk(-1), walk(-1), walk(-1), walk(+1), walk(+1), walk(+1)}; !reflect.DeepEqual(got, []string{"1.2", "1.1", "-", "1.2", "2.1", "-"}) {
		t.Errorf("back, back, back, forward, forward, forward walked %v", got)
	}

	// A stop entered after going back drops what forward could reach.
	walk(-1)
	walk(-1)
	h.visit("3")
	if want := []string{"1.1", "3"}; !reflect.DeepEqual(h.ids, want) || walk(+1) != "-" {
		t.Errorf("history %v after a new stop from 1.1, want %v and nothing forward", h.ids, want)
	}
	// Going back to a stop and entering it again is not a new stop.
	walk(-1)
	h.visit("1.1")
	if walk(+1) != "3" {
		t.Error("re-entering the stop went back to dropped the forward side")
	}

	// A stop a re-sent tour dropped is stepped over, and so is the stop the
	// reviewer is on when dropping one left it on both sides.
	h = stopHistory{}
	for _, id := range []string{"1.1", "gone", "1.1", "gone", "2"} {
		h.visit(id)
	}
	alive := func(id string) bool { return id != "gone" }
	if id, ok := h.walk(-1, alive); !ok || id != "1.1" {
		t.Errorf("back from 2 over a dropped stop gave %q %v, want 1.1", id, ok)
	}
	if id, ok := h.walk(-1, alive); ok {
		t.Errorf("back from 1.1 gave %q: the only earlier stops are dropped or 1.1 itself", id)
	}

	h = stopHistory{}
	for i := 0; i < maxStopHistory+20; i++ {
		h.visit(fmt.Sprint(i))
	}
	if len(h.ids) != maxStopHistory || h.ids[h.at] != fmt.Sprint(maxStopHistory+19) {
		t.Errorf("history of %d holding %q, want the newest %d", len(h.ids), h.ids[h.at], maxStopHistory)
	}
}

var (
	backspaceKey = tea.KeyPressMsg{Code: tea.KeyBackspace}
	f18Key       = tea.KeyPressMsg{Code: tea.KeyF18}
	f19Key       = tea.KeyPressMsg{Code: tea.KeyF19}
)

// Every way of entering a stop is recorded — stepping, :stop, the agent's goto
// — and backspace and F19 walk them, saying where they went.
func TestBackAndForwardWalkTheStopsEntered(t *testing.T) {
	m, e := tourApp(t) // restored on 1.1
	m = pressKey(t, m, ".")
	m = typeCommand(t, m, "stop 2")
	m = updateApp(t, m, tourEventMsg{status: core.WalkthroughEventGoto, id: "1.1"})

	type step struct {
		key   tea.Msg
		stop  string
		where string
	}
	for _, s := range []step{
		{backspaceKey, "2", "back to 2"},
		{f18Key, "1.2", "back to 1.2"},
		{backspaceKey, "1.1", "back to 1.1"},
		{backspaceKey, "1.1", "no earlier stop"},
		{f19Key, "1.2", "forward to 1.2"},
		{f19Key, "2", "forward to 2"},
		{f19Key, "1.1", "forward to 1.1"},
		{f19Key, "1.1", "no later stop"},
	} {
		m = updateApp(t, m, s.key)
		if stop, _ := m.currentStop(); stop.ID != s.stop || m.statusBar.searchInfo != s.where {
			t.Fatalf("%s: on %s saying %q, want %s saying %q", s.key, stop.ID, m.statusBar.searchInfo, s.stop, s.where)
		}
	}
	// A walk is a move like any other: the stop shows and the engine hears of it.
	m = updateApp(t, updateApp(t, m, backspaceKey), backspaceKey) // 1.1 → 2 → 1.2
	if m.diffView.path != "b.go" || cursorLine(m) != 30 {
		t.Errorf("back on 1.2 shows %s:%d, want its b.go:30", m.diffView.path, cursorLine(m))
	}
	if got := e.reports(); got[len(got)-1] != "1.2" {
		t.Errorf("reported %v, want the walk back to 1.2 last", got)
	}

	// The commands do what the keys do.
	m = typeCommand(t, m, "back")
	if stop, _ := m.currentStop(); stop.ID != "1.1" || m.statusBar.searchInfo != "back to 1.1" {
		t.Errorf(":back left %s saying %q", stop.ID, m.statusBar.searchInfo)
	}
	m = typeCommand(t, m, "forward")
	if stop, _ := m.currentStop(); stop.ID != "1.2" || m.statusBar.searchInfo != "forward to 1.2" {
		t.Errorf(":forward left %s saying %q", stop.ID, m.statusBar.searchInfo)
	}
}

// The keys belong to tour mode, and to no text input: in each, backspace still
// deletes and F18/F19 do nothing to the tour.
func TestBackKeysOnlyInTourModeAndNeverWhileTyping(t *testing.T) {
	on := func(t *testing.T) appModel {
		m, _ := tourApp(t)
		return pressKey(t, pressKey(t, m, "."), ".") // 1.1 → 1.2 → 2
	}
	stopID := func(m appModel) string { s, _ := m.currentStop(); return s.ID }

	t.Run("the note pane passes them through", func(t *testing.T) {
		m := on(t)
		m.setFocus(focusDoc)
		if m = updateApp(t, m, backspaceKey); stopID(m) != "1.2" {
			t.Errorf("backspace from the note pane left the tour on %s", stopID(m))
		}
	})

	t.Run("tour off", func(t *testing.T) {
		m := pressKey(t, on(t), "W")
		for _, k := range []tea.Msg{backspaceKey, f18Key, f19Key} {
			if m = updateApp(t, m, k); m.tour.on || stopID(m) != "2" {
				t.Errorf("%s with the tour off: on=%v at %s", k, m.tour.on, stopID(m))
			}
		}
	})

	inputs := []struct {
		name  string
		opens string // the key that starts typing
		typed func(appModel) string
	}{
		{"command", ":", func(m appModel) string { return m.commandBuffer }},
		{"search", "/", func(m appModel) string { return m.searchBuffer }},
		{"shell", "!", func(m appModel) string { return m.shellBuffer }},
		{"comment", "c", func(m appModel) string { return m.commentEditor.body }},
	}
	for _, in := range inputs {
		t.Run(in.name, func(t *testing.T) {
			m := pressKey(t, on(t), in.opens)
			m = pressKey(t, pressKey(t, m, "x"), "y")
			before := in.typed(m)
			for _, k := range []tea.Msg{f18Key, f19Key} {
				m = updateApp(t, m, k)
			}
			m = updateApp(t, m, backspaceKey)
			if stopID(m) != "2" {
				t.Errorf("a key typed into the %s moved the tour to %s", in.name, stopID(m))
			}
			if after := in.typed(m); len(after) != len(before)-1 {
				t.Errorf("backspace in the %s: %q → %q, want one character deleted", in.name, before, after)
			}
		})
	}
}

// keyRecorder is a Bubble Tea model that keeps the keys it is sent and quits
// once it has want of them.
type keyRecorder struct {
	want int
	keys []string
}

func (r *keyRecorder) Init() tea.Cmd { return nil }
func (r *keyRecorder) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		r.keys = append(r.keys, k.String())
		if len(r.keys) == r.want {
			return r, tea.Quit
		}
	}
	return r, nil
}
func (r *keyRecorder) View() tea.View { return tea.NewView("") }

// F18 and F19, as a key remapper sends them, are the vt220 sequences ESC[32~
// and ESC[33~. Fed through Bubble Tea's own input reader, they must arrive as
// the keys the tour binds to back and forward.
func TestF18AndF19BytesAreTheTourKeys(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	rec := &keyRecorder{want: 2}
	p := tea.NewProgram(rec, tea.WithInput(r), tea.WithOutput(io.Discard), tea.WithoutSignals(), tea.WithoutRenderer())
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	go func() { _, _ = w.Write([]byte("\x1b[32~\x1b[33~")) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		p.Kill()
		t.Fatalf("the program never saw two keys; it saw %v", rec.keys)
	}
	km := DefaultKeyMap()
	if len(rec.keys) != 2 || !Matches(rec.keys[0], km.TourBack) || !Matches(rec.keys[1], km.TourForward) {
		t.Errorf("ESC[32~ ESC[33~ arrived as %q, want the tour's back then forward keys", rec.keys)
	}
}

// In tour mode o toggles the stop's note, so a note o closed is never out of
// reach. Bringing it back is not arriving: nothing is reported and no side
// effects are scheduled.
func TestOTogglesTheStopsNote(t *testing.T) {
	noteOpen := func(m appModel, id string) bool {
		return m.docPane.active && m.docPane.note && m.docPane.annotationID == tourNoteKeyPrefix+id
	}
	for _, from := range []struct {
		name  string
		focus focusTarget
	}{{"from the diff", focusMain}, {"from the note pane", focusDoc}} {
		t.Run(from.name, func(t *testing.T) {
			m, e := tourApp(t)
			m = pressKey(t, m, ".") // 1.2
			reports, settle := len(e.reports()), m.tour.settle
			m.setFocus(from.focus)
			if m = pressKey(t, m, "d"); m.docPane.active {
				t.Fatal("d did not close the stop's note")
			}
			if m = pressKey(t, m, "d"); !noteOpen(m, "1.2") {
				t.Fatalf("d did not bring back 1.2's note: active=%v note=%v id=%q", m.docPane.active, m.docPane.note, m.docPane.annotationID)
			}
			if !strings.Contains(stripANSISeq(m.docPane.View()), "The write.") || m.tour.index != 1 {
				t.Errorf("the note brought back is not 1.2's, or the stop moved (index %d)", m.tour.index)
			}
			if len(e.reports()) != reports || m.tour.settle != settle {
				t.Errorf("bringing the note back reported %v and scheduled %d settles: it is not arriving", e.reports()[reports:], m.tour.settle-settle)
			}
		})
	}

	t.Run("outside tour mode d is unchanged", func(t *testing.T) {
		m, _ := tourApp(t)
		m = pressKey(t, m, "W")
		if m = pressKey(t, m, "d"); m.docPane.active {
			t.Error("d opened a note with the tour off")
		}
	})

	// The cursor on an annotation is the narrower ask, so d opens its doc
	// links there, tour or not; off it, d is the note's toggle again.
	t.Run("an annotation under the cursor wins", func(t *testing.T) {
		m, _ := tourApp(t) // 1.1, cursor on a.go:5
		m.diffView.annotations = []types.Annotation{{ID: "x1", TargetRef: "a.go", LineStart: 5, LineEnd: 5, Summary: "why",
			Refs: []types.DocRef{{Kind: types.DocRefFile, Doc: "NOTES.md"}}}}
		m.diffView.buildLines()
		m.diffView.GoToLine(5)
		if m = pressKey(t, m, "d"); m.docPane.annotationID != "x1" {
			t.Fatalf("d on an annotation showed %q, want its doc links", m.docPane.annotationID)
		}
		m.diffView.GoToLine(20)
		if m = pressKey(t, m, "d"); m.docPane.active {
			t.Fatal("d off the annotation did not close its doc pane")
		}
		// With the pane closed, on the annotation, d still means its links.
		m.diffView.GoToLine(5)
		if m = pressKey(t, m, "d"); m.docPane.annotationID != "x1" {
			t.Fatalf("d on an annotation with the pane closed showed %q, want its doc links", m.docPane.annotationID)
		}
		m.diffView.GoToLine(20)
		m = pressKey(t, m, "d")
		if m = pressKey(t, m, "d"); !noteOpen(m, "1.1") {
			t.Errorf("o with the pane closed did not bring back 1.1's note: id=%q", m.docPane.annotationID)
		}
	})
}
