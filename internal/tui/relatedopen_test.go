package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/types"
)

// openApp is viewsTour (1.2 has a.go:5 and internal/deep/path/b.go:30) with
// related_open recording each file it is run for, in a repo of its own, still
// on 1.1. resolve, when set, is walkthrough_resolve's answer.
func openApp(t *testing.T, resolve string) (appModel, string, *[]relatedPanePlan) {
	m, opened, plans, _ := openAppLog(t, resolve)
	return m, opened, plans
}

// openAppLog is openApp, with the log of the tmux commands run.
func openAppLog(t *testing.T, resolve string) (appModel, string, *[]relatedPanePlan, string) {
	t.Helper()
	skipWithoutSh(t)
	dir := t.TempDir()
	opened := filepath.Join(dir, "opened")
	cfg := inertViewers(&types.Config{Editor: "nvim",
		RelatedEditorAdd: `printf 'add|%s\n' {file} >> ` + opened,
		RelatedOpen:      `printf '%s|%s|%s\n' {file} {line} {owner} >> ` + opened})
	if resolve != "" {
		reply := filepath.Join(dir, "reply.json")
		if err := os.WriteFile(reply, []byte(resolve), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg.WalkthroughResolve = "cat " + reply
	}
	m, _ := tourAppWith(t, viewsTour(), cfg)
	m.repoRoot, m.diffView.repoRoot = dir, dir
	plans := captureRelated(t)
	log := fakeTmux(t)
	orig := findPane
	findPane = func(string, string) string { return "%7" } // an old pane still there: never used
	t.Cleanup(func() { findPane = orig })
	return m, opened, plans, log
}

// waitForLines waits for path to hold n lines, the command running in the
// background as it does in the app.
func waitForLines(t *testing.T, path string, n int) []string {
	t.Helper()
	var got []string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if got = readLines(t, path); len(got) >= n {
			break
		}
	}
	return got
}

// Arriving at a stop that carries related files opens no pane beside Monocle,
// and runs nothing: Monocle no longer opens a stop's related files.
func TestRelatedOpenArrivingOpensNothing(t *testing.T) {
	m, opened, plans := openApp(t, "")
	m = settle(t, pressKey(t, m, ".")) // 1.2
	if len(*plans) != 0 {
		t.Errorf("arriving split the related pane: %+v", (*plans)[0].files)
	}
	if got := readLines(t, opened); len(got) != 0 {
		t.Errorf("arriving ran %q", got)
	}
}

// What ctrl+] finds goes to related_open, the first file only: one popout
// shows one file. The status says how many more were found.
func TestRelatedOpenResolveRunsItForTheFirstFound(t *testing.T) {
	for _, key := range []struct {
		name    string
		preview bool
	}{{"ctrl+]", false}, {"p", true}} {
		t.Run(key.name, func(t *testing.T) {
			m, opened, plans := openApp(t, `[{"path": "q.sql", "line": 12}, {"path": "a.go", "line": 5}, {"path": "c.go"}]`)
			m.engine.GetConfig().RelatedEditorPreview = `printf 'preview|%s\n' {file} >> ` + opened
			m = settle(t, pressKey(t, m, "."))
			if key.preview {
				next, cmd := m.Update(previewRefsKey)
				m = driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)
			} else {
				m = openRefs(t, m)
			}
			want := []string{filepath.Join(m.repoRoot, "q.sql") + "|12|%99"}
			if got := waitForLines(t, opened, 1); !reflect.DeepEqual(got, want) {
				t.Errorf("ran %q, want related_open for q.sql alone: %q", got, want)
			}
			if len(*plans) != 0 {
				t.Error("the related pane was split")
			}
			if want := "opened q.sql · 2 more found"; m.statusBar.searchInfo != want {
				t.Errorf("status %q, want %q", m.statusBar.searchInfo, want)
			}
		})
	}
}

// ctrl+g in a tour, and `monocle review open-editor` (full or not), go to
// related_open too: no pane beside Monocle is opened or zoomed.
func TestRelatedOpenOpenEditorRunsIt(t *testing.T) {
	for _, full := range []bool{false, true} {
		m, opened, plans, log := openAppLog(t, "")
		m = settle(t, pressKey(t, m, "."))
		next, cmd := m.Update(openEditorMsg{path: filepath.Join(m.repoRoot, "b.go"), line: 30, full: full})
		m = driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)
		if got, want := waitForLines(t, opened, 1), []string{filepath.Join(m.repoRoot, "b.go") + "|30|%99"}; !reflect.DeepEqual(got, want) {
			t.Errorf("full=%v: ran %q, want %q", full, got, want)
		}
		if len(*plans) != 0 {
			t.Errorf("full=%v: the related pane was split", full)
		}
		if data, _ := os.ReadFile(log); strings.Contains(string(data), "window_zoomed_flag") || strings.Contains(string(data), "resize-pane") {
			t.Errorf("full=%v: an old related pane was zoomed:\n%s", full, data)
		}
	}
	m, opened, _ := openApp(t, "")
	m = settle(t, pressKey(t, m, "."))
	next, cmd := m.Update(openEditorKey) // on 1.2: b.go, line 30
	driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)
	if got, want := waitForLines(t, opened, 1), []string{filepath.Join(m.repoRoot, "b.go") + "|30|%99"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ctrl+g ran %q, want %q", got, want)
	}
}

// A related_open that fails says so, and opens nothing in its place.
func TestRelatedOpenFailureIsSaid(t *testing.T) {
	m, opened, plans := openApp(t, "")
	m.engine.GetConfig().RelatedOpen = "echo no popout >&2; exit 3"
	m = settle(t, pressKey(t, m, "."))
	next, cmd := m.Update(openEditorMsg{path: filepath.Join(m.repoRoot, "a.go"), line: 5})
	m = driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)
	if !strings.HasPrefix(m.statusBar.searchInfo, "related_open: ") {
		t.Errorf("status %q, want the failure", m.statusBar.searchInfo)
	}
	if len(*plans) != 0 || len(readLines(t, opened)) != 0 {
		t.Error("a failed related_open fell back to the pane")
	}
}
