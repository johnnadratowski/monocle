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

func TestExpandAddCommand(t *testing.T) {
	got := expandAddCommand("open {file} +{line} --owner {owner}", relatedFile{path: "/repo/my dir/q.sql", line: 12}, "%3")
	if want := `open '/repo/my dir/q.sql' +12 --owner '%3'`; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if got := expandAddCommand("open {file}:{line}", relatedFile{path: "/repo/a.go"}, "%3"); got != "open /repo/a.go:0" {
		t.Errorf("a file with no line: %s", got)
	}
}

// addApp is resolveApp's tour (on 1.2, related a.go:5) with related_editor_add
// recording each file it is run for. alive says whether the related pane is.
func addApp(t *testing.T, answer string, alive bool) (appModel, string, *[]relatedPanePlan, string) {
	t.Helper()
	skipWithoutSh(t)
	dir := t.TempDir()
	reply, added := filepath.Join(dir, "reply.json"), filepath.Join(dir, "added")
	if err := os.WriteFile(reply, []byte(answer), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ := tourAppWith(t, testTour(), &types.Config{Editor: "nvim",
		WalkthroughResolve: "cat " + reply,
		RelatedEditorAdd:   `printf '%s|%s|%s\n' {file} {line} {owner} >> ` + added})
	m.repoRoot, m.diffView.repoRoot = dir, dir
	m = pressKey(t, m, ".") // 1.2
	plans := captureRelated(t)
	log := fakeTmux(t)
	orig := findPane
	findPane = func(string, string) string {
		if alive {
			return "%7"
		}
		return ""
	}
	t.Cleanup(func() { findPane = orig })
	return m, added, plans, log
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Fields(string(data))
}

// With related_editor_add and a live pane, what ctrl+] finds is added to the
// editor there, every file in order — one already open included, since the
// editor knows what is really open — and the pane is not respawned, so files
// the reviewer closed stay closed.
func TestOpenReferencesAddsToTheLiveEditor(t *testing.T) {
	m, added, plans, log := addApp(t, `[{"path": "q.sql", "line": 12}, {"path": "a.go", "line": 5}]`, true)
	root := m.repoRoot
	m.tour.paneFiles = []relatedFile{{filepath.Join(root, "a.go"), 5}, {filepath.Join(root, "closed.go"), 0}}
	m = openRefs(t, m)

	want := []string{filepath.Join(root, "q.sql") + "|12|%99", filepath.Join(root, "a.go") + "|5|%99"}
	if got := readLines(t, added); !reflect.DeepEqual(got, want) {
		t.Errorf("related_editor_add ran for %q, want %q", got, want)
	}
	if len(*plans) != 0 {
		t.Errorf("the pane was respawned with %+v", (*plans)[0].files)
	}
	if data, _ := os.ReadFile(log); !strings.Contains(string(data), "select-pane -t %7") {
		t.Errorf("focus did not move to the pane:\n%s", data)
	}
	if want := "opened 2: q.sql, a.go"; m.statusBar.searchInfo != want {
		t.Errorf("status %q, want %q", m.statusBar.searchInfo, want)
	}
}

// With no live pane there is no editor to add to: the pane is spawned with the
// stop's related files and the files found — not with what it last held,
// which may have been closed.
func TestOpenReferencesSpawnsWhenNoPaneIsAlive(t *testing.T) {
	m, added, plans, _ := addApp(t, `[{"path": "q.sql", "line": 12}]`, false)
	root := m.repoRoot
	m.tour.paneFiles = []relatedFile{{filepath.Join(root, "closed.go"), 0}}
	m = openRefs(t, m)
	if got := readLines(t, added); len(got) != 0 {
		t.Errorf("related_editor_add ran with no pane alive: %q", got)
	}
	if len(*plans) != 1 {
		t.Fatalf("%d plans, want the pane spawned", len(*plans))
	}
	want := []relatedFile{{filepath.Join(root, "a.go"), 5}, {filepath.Join(root, "q.sql"), 12}}
	if got := (*plans)[0].files; !reflect.DeepEqual(got, want) {
		t.Errorf("spawned with %+v, want the stop's a.go and q.sql: %+v", got, want)
	}
}

// :related N, and a click on a related file's label, add just that file.
func TestRelatedNAddsOneFile(t *testing.T) {
	tour := viewsTour() // 1.2 has a.go:5 and internal/deep/path/b.go:30
	m, _ := tourAppWith(t, tour, inertViewers(&types.Config{Editor: "nvim"}))
	skipWithoutSh(t)
	added := filepath.Join(t.TempDir(), "added")
	m.engine.GetConfig().RelatedEditorAdd = `printf '%s|%s\n' {file} {line} >> ` + added
	m = pressKey(t, m, ".")
	plans := captureRelated(t)
	fakeTmux(t)
	orig := findPane
	findPane = func(string, string) string { return "%7" }
	t.Cleanup(func() { findPane = orig })

	next, cmd := m.Update(tourRelatedMsg{arg: "2"}) // :related 2, or a click on its label
	m = driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)
	if got := readLines(t, added); !reflect.DeepEqual(got, []string{"internal/deep/path/b.go|30"}) {
		t.Errorf("related_editor_add ran for %q, want only file 2", got)
	}
	if len(*plans) != 0 {
		t.Error(":related 2 respawned the pane")
	}

	// A stop change still respawns with that stop's related files.
	m = pressKey(t, pressKey(t, m, ","), ".")
	m = settle(t, m)
	if len(*plans) != 1 {
		t.Errorf("%d plans after arriving at 1.2 again, want the stop's respawn", len(*plans))
	}
}
