package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

// gappedDiff is c.go, 60 lines, changed at lines 5 and 45: its compact diff
// shows lines 2-8 and 42-48, its whole-file diff all 60.
func gappedDiff() (compact, full *types.DiffResult) {
	row := func(n int) types.DiffLine {
		if n == 5 || n == 45 {
			return types.DiffLine{Kind: types.DiffLineAdded, NewLineNum: n, Content: fmt.Sprintf("changed %d", n)}
		}
		return types.DiffLine{Kind: types.DiffLineContext, OldLineNum: n, NewLineNum: n, Content: fmt.Sprintf("line %d", n)}
	}
	removed := func(n int) types.DiffLine {
		return types.DiffLine{Kind: types.DiffLineRemoved, OldLineNum: n, Content: fmt.Sprintf("was %d", n)}
	}
	hunk := func(from, to int) types.DiffHunk {
		h := types.DiffHunk{OldStart: from, NewStart: from}
		for n := from; n <= to; n++ {
			if n == 5 || n == 45 {
				h.Lines = append(h.Lines, removed(n))
				h.OldCount++
			}
			r := row(n)
			h.Lines = append(h.Lines, r)
			h.NewCount++
			if r.Kind == types.DiffLineContext {
				h.OldCount++
			}
		}
		h.Header = fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldCount, h.NewStart, h.NewCount)
		return h
	}
	compact = &types.DiffResult{Path: "c.go", Hunks: []types.DiffHunk{hunk(2, 8), hunk(42, 48)}}
	full = &types.DiffResult{Path: "c.go", Hunks: []types.DiffHunk{hunk(1, 60)}}
	return compact, full
}

// gappedApp is tourApp with c.go in the review as gappedDiff, in a pane too
// short to show its diff whole, so a line lands where it is placed.
func gappedApp(t *testing.T) (appModel, *tourEngine) {
	t.Helper()
	m, e := tourApp(t)
	compact, full := gappedDiff()
	files := append(e.changedFiles, types.ChangedFile{Path: "c.go", Status: types.FileModified})
	e.changedFiles, e.session.ChangedFiles = files, files
	e.diffs["c.go"] = compact
	e.fullDiffs = map[string]*types.DiffResult{"c.go": full}
	m = updateApp(t, m, initialLoadMsg{files: files})
	m = updateApp(t, m, tea.WindowSizeMsg{Width: 140, Height: 20})
	return m, e
}

func shows(m appModel, n int) bool { return m.diffView.indexForNewLine(n) >= 0 }

// A goto_line into the context a compact diff hides reveals it — that stretch
// and a margin, not the whole file — and lands on the line, placed as asked.
// A highlight beyond what is shown reveals its range too.
func TestGotoLineRevealsHiddenContext(t *testing.T) {
	m, _ := gappedApp(t)
	three := 3
	m = updateApp(t, m, gotoLineMsg{path: "c.go", line: 20, top: &three})
	if m.diffView.path != "c.go" || cursorLine(m) != 20 || rowsAbove(m.diffView) != 3 {
		t.Fatalf("at %s:%d with %d rows above, want c.go:20 three rows down", m.diffView.path, cursorLine(m), rowsAbove(m.diffView))
	}
	if !shows(m, 17) || !shows(m, 23) || shows(m, 30) || m.diffView.fullFile {
		t.Errorf("revealed 17:%v 23:%v, 30:%v (full %v): want line 20 with a margin, the rest compact", shows(m, 17), shows(m, 23), shows(m, 30), m.diffView.fullFile)
	}

	m = updateApp(t, m, highlightRangeMsg{path: "c.go", start: 18, end: 28})
	for n := 18; n <= 28; n++ {
		if !shows(m, n) || !m.diffView.inHighlight(lineRow(m, n)) {
			t.Fatalf("line %d of the highlight: shown %v, highlighted %v", n, shows(m, n), m.diffView.inHighlight(lineRow(m, n)))
		}
	}
	if cursorLine(m) != 20 || rowsAbove(m.diffView) != 3 {
		t.Errorf("the highlight moved the cursor: line %d, %d rows above", cursorLine(m), rowsAbove(m.diffView))
	}
}

// What was revealed stays revealed for the session: through the whole-file
// view and back, a refresh, a comment on a revealed line written, resolved and
// deleted, and leaving the file and coming back by ctrl+o or the file list.
func TestRevealedLinesStayRevealed(t *testing.T) {
	m, e := gappedApp(t)
	m = updateApp(t, m, gotoLineMsg{path: "c.go", line: 20})
	check := func(after string) {
		t.Helper()
		if m.diffView.path != "c.go" || m.diffView.fullFile || !shows(m, 20) || shows(m, 30) {
			t.Errorf("after %s: in %s (full %v), 20 shown %v, 30 shown %v: want c.go compact with 20 revealed",
				after, m.diffView.path, m.diffView.fullFile, shows(m, 20), shows(m, 30))
		}
	}
	m = pressKey(t, pressKey(t, m, "a"), "a")
	check("the whole-file view and back")
	if cursorLine(m) != 20 {
		t.Errorf("the whole-file view and back moved the cursor to %d", cursorLine(m))
	}
	m = drive(t, m, m.refreshFiles(), 0)
	check("a refresh")
	m.engine = &commentEngine{tourEngine: e}
	m = updateApp(t, m, saveCommentMsg{path: "c.go", lineStart: 20, lineEnd: 20, targetType: types.TargetFile, commentType: types.CommentQuestion, body: "why?"})
	check("a comment")
	m = updateApp(t, m, resolveCommentMsg{commentID: "c"})
	check("resolving it")
	m = updateApp(t, m, deleteCommentMsg{commentID: "c"})
	check("deleting it")
	m = updateApp(t, m, gotoLineMsg{path: "a.go", line: 5})
	m = updateApp(t, m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	check("ctrl+o")
	m = updateApp(t, m, gotoLineMsg{path: "a.go", line: 5})
	m = updateApp(t, m, sidebarSelectMsg{path: "c.go"})
	check("the file list")
}

// A tour stop on lines the compact diff hides shows them.
func TestAStopOnHiddenLinesRevealsThem(t *testing.T) {
	m, e := gappedApp(t)
	tour := testTour()
	tour.Stops = append(tour.Stops, types.WalkthroughStop{ID: "3", Title: "Unchanged", File: "c.go", LineStart: 30, LineEnd: 33})
	e.session.Walkthrough = tour
	m = updateApp(t, m, tourEventMsg{status: "set", id: "1.1"})
	m = typeCommand(t, m, "stop 3")
	if m.diffView.path != "c.go" || cursorLine(m) != 30 || !shows(m, 33) {
		t.Errorf("stop 3 at %s:%d, 33 shown %v: want its lines on screen", m.diffView.path, cursorLine(m), shows(m, 33))
	}
}

func TestRevealHunks(t *testing.T) {
	compact, full := gappedDiff()
	got := revealHunks(compact.Hunks, full.Hunks, []lineRange{{15, 20}, {38, 40}})
	var spans [][2]int
	for _, h := range got {
		spans = append(spans, [2]int{h.NewStart, h.NewStart + h.NewCount - 1})
	}
	// 38-40 runs into the hunk at 42 only if 41 is revealed too; it is not,
	// so they stay apart.
	want := [][2]int{{2, 8}, {15, 20}, {38, 40}, {42, 48}}
	if fmt.Sprint(spans) != fmt.Sprint(want) {
		t.Errorf("hunks cover %v, want %v", spans, want)
	}
	if got[0].Header != compact.Hunks[0].Header {
		t.Errorf("a hunk the compact diff had lost its header: %q", got[0].Header)
	}
	// Removed lines stay, in place, with the rest of their hunk.
	removed := 0
	for _, l := range got[0].Lines {
		if l.Kind == types.DiffLineRemoved {
			removed++
		}
	}
	if removed != 1 || len(got[0].Lines) != len(compact.Hunks[0].Lines) {
		t.Errorf("the first hunk has %d lines, %d removed, want it as the compact diff had it", len(got[0].Lines), removed)
	}
	// A range touching a hunk joins it.
	joined := revealHunks(compact.Hunks, full.Hunks, []lineRange{{9, 12}})
	if len(joined) != 2 || joined[0].NewStart != 2 || joined[0].NewStart+joined[0].NewCount-1 != 12 {
		t.Errorf("a reveal at 9-12 gave %+v, want the first hunk running to 12", joined[0])
	}
}

// step is m.Update without driving the command it returns.
func step(m appModel, msg tea.Msg) (appModel, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(appModel), cmd
}

// A diff that answers a request a newer one has replaced is dropped: a slow
// load must not take away lines a newer one revealed.
func TestAnOlderDiffLoadIsDropped(t *testing.T) {
	m, _ := gappedApp(t)
	m = updateApp(t, m, gotoLineMsg{path: "c.go", line: 5})
	m, older := step(m, requestFileDiffMsg{path: "c.go"})
	m.reveal("c.go", 20, 20)
	m, newer := step(m, requestFileDiffMsg{path: "c.go"})
	m = updateApp(t, m, newer())
	m = updateApp(t, m, older())
	if !shows(m, 20) {
		t.Error("the older load replaced the newer one: line 20 is hidden again")
	}
}

// A jump made while a reveal's reload is on its way lands where it was sent,
// not where the reload would have put the cursor back.
func TestAJumpDuringARevealLandsWhereSent(t *testing.T) {
	m, _ := gappedApp(t)
	m = updateApp(t, m, gotoLineMsg{path: "c.go", line: 5})
	m, request := step(m, highlightRangeMsg{path: "c.go", start: 18, end: 28})
	m, reload := step(m, request())
	m = updateApp(t, m, gotoLineMsg{path: "c.go", line: 44})
	m = updateApp(t, m, reload())
	if cursorLine(m) != 44 || !shows(m, 28) {
		t.Errorf("cursor on %d, 28 shown %v: want 44, with the highlight revealed", cursorLine(m), shows(m, 28))
	}
}

// A highlight in the file on screen does not cancel a jump to another file
// that is on its way: that file still opens, and the highlight's lines show
// when the reviewer comes back.
func TestAHighlightDoesNotCancelAJumpElsewhere(t *testing.T) {
	m, _ := gappedApp(t)
	m = updateApp(t, m, gotoLineMsg{path: "c.go", line: 5})
	m, request := step(m, gotoLineMsg{path: "a.go", line: 5})
	m, load := step(m, request())
	m = updateApp(t, m, highlightRangeMsg{path: "c.go", start: 18, end: 28})
	m = updateApp(t, m, load())
	if m.diffView.path != "a.go" || cursorLine(m) != 5 {
		t.Fatalf("at %s:%d, want the jump to a.go:5", m.diffView.path, cursorLine(m))
	}
	m = updateApp(t, m, gotoLineMsg{path: "c.go", line: 20})
	if !shows(m, 28) {
		t.Error("back in c.go, the highlight's lines are hidden")
	}
}
