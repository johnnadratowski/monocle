package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

// cursorApp is tourApp with a cursor_command recording each run's environment
// to a file, which it returns.
func cursorApp(t *testing.T) (appModel, string) {
	t.Helper()
	skipWithoutSh(t)
	out := filepath.Join(t.TempDir(), "runs")
	m, _ := tourAppWith(t, testTour(), &types.Config{
		CursorCommand: `printf '%s|%s|%s\n' "$MONOCLE_FILE" "$MONOCLE_LINE" "${MONOCLE_REPO_ROOT:+root}" >> ` + out,
	})
	m.repoRoot = t.TempDir()
	return m, out
}

// settleCursor delivers the settle tick for the latest move, as the timer
// would, and waits for any run it starts.
func settleCursor(t *testing.T, m appModel, seq int) appModel {
	t.Helper()
	next, cmd := m.Update(cursorSettledMsg{seq: seq})
	return driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)
}

func runsIn(t *testing.T, out string) []string {
	t.Helper()
	data, _ := os.ReadFile(out)
	return strings.Fields(string(data))
}

// The command runs once the cursor rests: only the latest move's tick acts,
// with where the cursor is then, and never twice in a row for the same place.
func TestCursorCommandRunsOnceTheCursorSettles(t *testing.T) {
	m, out := cursorApp(t) // a.go:5
	m = settleCursor(t, m, m.cursor.seq)
	m = pressN(t, m, "j", 3) // a.go:8, three moves in quick succession
	stale := m.cursor.seq - 1
	if m = settleCursor(t, m, stale); len(runsIn(t, out)) != 1 {
		t.Fatalf("a tick from before the last move ran the command: %q", runsIn(t, out))
	}
	m = settleCursor(t, m, m.cursor.seq)
	want := []string{"a.go|5|root", "a.go|8|root"}
	if got := runsIn(t, out); !reflect.DeepEqual(got, want) {
		t.Fatalf("runs %q, want %q", got, want)
	}
	// The same place again — resting again, or leaving and coming back
	// before resting — is not reported again.
	m = settleCursor(t, m, m.cursor.seq)
	m = pressKey(t, pressKey(t, m, "j"), "k")
	m = settleCursor(t, m, m.cursor.seq)
	if got := runsIn(t, out); len(got) != 2 {
		t.Errorf("runs %q: the same file:line was reported twice in a row", got)
	}
	// A move to another file is reported.
	m = updateApp(t, m, gotoLineMsg{path: "b.go", line: 12})
	m = settleCursor(t, m, m.cursor.seq)
	if got := runsIn(t, out); len(got) != 3 || got[2] != "b.go|12|root" {
		t.Errorf("runs %q, want b.go:12 last", got)
	}
}

// Nothing runs while a modal is open; where the cursor rests once it closes
// is reported then.
func TestCursorCommandWaitsForModals(t *testing.T) {
	m, out := cursorApp(t)
	m = settleCursor(t, m, m.cursor.seq) // a.go:5
	m = pressKey(t, m, "j")              // a.go:6
	m = pressKey(t, m, ":")              // the command line opens before it settles
	m = settleCursor(t, m, m.cursor.seq)
	if got := runsIn(t, out); len(got) != 1 {
		t.Fatalf("runs %q: the command ran with the command line open", got)
	}
	m = updateApp(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = settleCursor(t, m, m.cursor.seq)
	if got := runsIn(t, out); len(got) != 2 || got[1] != "a.go|6|root" {
		t.Errorf("runs %q, want a.go:6 once the command line closed", got)
	}
}

// With no cursor_command nothing is watched at all.
func TestNoCursorCommandWatchesNothing(t *testing.T) {
	m, _ := tourApp(t)
	m = pressN(t, m, "j", 3)
	if m.cursor.seq != 0 {
		t.Errorf("cursor moves were tracked (seq %d) with no cursor_command", m.cursor.seq)
	}
}

// A row with no new-file line — a removed line, a hunk header — is line 0.
func TestCursorKeyLineZero(t *testing.T) {
	m := appModel{diffView: mixedDiffView(diffStyleUnified)}
	for i, l := range m.diffView.lines {
		if l.kind == types.DiffLineRemoved {
			m.diffView.cursor = i
			break
		}
	}
	if key, file, line := m.cursorKey(); key != "x.go:0" || file != "x.go" || line != 0 {
		t.Errorf("a removed line is %q (%s, %d), want x.go:0", key, file, line)
	}
}

// The run never holds up the TUI: settling hands back a command and returns
// at once, however long the command takes.
func TestCursorCommandNeverBlocks(t *testing.T) {
	skipWithoutSh(t)
	m, _ := tourAppWith(t, testTour(), &types.Config{CursorCommand: "sleep 1"})
	m.repoRoot = t.TempDir()
	start := time.Now()
	_, cmd := m.Update(cursorSettledMsg{seq: m.cursor.seq})
	if took := time.Since(start); took > 300*time.Millisecond || cmd == nil {
		t.Errorf("settling took %s (cmd %v): the run must happen in the background", took, cmd != nil)
	}
}
