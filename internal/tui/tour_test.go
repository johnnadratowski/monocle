package tui

import (
	"fmt"
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
	return e.GetFileDiff(path)
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
	// Never let a test reach a real tmux server: these tests may themselves be
	// running inside one, and a stop's side effects split and kill panes.
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	files := []types.ChangedFile{{Path: "a.go", Status: types.FileAdded}, {Path: "b.go", Status: types.FileAdded}}
	e := &tourEngine{
		stubEngine: stubEngine{
			cfg:          &types.Config{},
			changedFiles: files,
			session:      &types.ReviewSession{ID: "s", ChangedFiles: files, Walkthrough: testTour(), WalkthroughStop: "1.1"},
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
	case <-time.After(50 * time.Millisecond):
		return m
	}
	switch msg := msg.(type) {
	case nil:
		return m
	case tea.BatchMsg:
		for _, c := range msg {
			m = drive(t, m, c, depth+1)
		}
		return m
	default:
		next, c := m.Update(msg)
		return drive(t, next.(appModel), c, depth+1)
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
	if got := stripANSISeq(m.statusBar.View()); !strings.Contains(got, "tour 1.2 / 3") {
		t.Errorf("status bar %q should show tour 1.2 / 3", got)
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
	m = typeCommand(t, m, "stop")
	if m.statusBar.searchInfo != "stops: 1.1 1.2 2" {
		t.Errorf(":stop with no id said %q, want the list of stops", m.statusBar.searchInfo)
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
	m, e := tourApp(t)
	m.tour.loading = "b.go"
	m = updateApp(t, m, loadDiffMsg{path: "a.go", result: e.diffs["a.go"]})
	if m.tour.loading != "b.go" {
		t.Fatal("a load for another file must not clear the wait")
	}
	m.pendingJumpLine = 30
	m = updateApp(t, m, loadDiffMsg{path: "b.go", result: e.diffs["b.go"]})
	if m.tour.loading != "" || m.diffView.path != "b.go" || cursorLine(m) != 30 {
		t.Errorf("after the tour's own load: loading=%q at %s:%d", m.tour.loading, m.diffView.path, cursorLine(m))
	}
}

func TestStopNoteBody(t *testing.T) {
	got := stopNoteBody(types.WalkthroughStop{
		Note:    "  why  ",
		Related: []types.DocRef{{Doc: "a.go", StartLine: 4}, {Doc: "b.go"}},
		Views:   []types.StopView{{Kind: "image", Target: "shots/x.png"}, {Kind: "url", Target: "https://x.test", Label: "Spec"}},
	})
	want := "why\n\n**Related:** a.go:4 · b.go\n**Views:** [1] image x.png · [2] url Spec  (`:view N` opens one)"
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
	d.openNote("tour:1", "1 · T", "one two three four five six seven eight nine ten", nil)
	d.reflow()
	if len(d.lines) < 2 {
		t.Fatalf("lines %q, want the note wrapped to the pane", d.lines)
	}
	for _, l := range d.lines {
		if w := len([]rune(l)); w > 24 {
			t.Errorf("row %q is %d wide in a 24-column pane", l, w)
		}
	}
	if got := d.noteHeight(24); got != len(d.lines)+1 {
		t.Errorf("noteHeight = %d, want its rows plus the title", got)
	}
	d.close()
	if d.note || d.active {
		t.Error("close should leave note mode")
	}
}
