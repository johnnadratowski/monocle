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

var (
	openEditorKey         = tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl}
	openEditorTakeoverKey = tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl | tea.ModShift}
)

// In a tour, ctrl+g opens the file in the related pane beside Monocle, at the
// cursor's line, rather than in an editor of its own: the related pane is
// where a tour's files are read.
func TestOpenInEditorDuringATourUsesTheRelatedPane(t *testing.T) {
	m, added, plans, _ := addApp(t, `[]`, true)
	next, cmd := m.Update(openEditorKey) // on 1.2: b.go, line 30
	m = driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)

	want := []string{filepath.Join(m.repoRoot, "b.go") + "|30|%99"}
	if got := readLines(t, added); !reflect.DeepEqual(got, want) {
		t.Errorf("related_editor_add ran for %q, want %q", got, want)
	}
	if len(*plans) != 0 {
		t.Errorf("the pane was respawned with %+v", (*plans)[0].files)
	}
}

// ctrl+shift+g still takes over the screen in a tour, and ctrl+g outside one
// opens the editor as it always has.
func TestOpenInEditorLeavesTheRelatedPaneAloneOtherwise(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
		tour bool
	}{
		{"ctrl+shift+g in a tour", openEditorTakeoverKey, true},
		{"ctrl+g with no tour", openEditorKey, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, added, plans, _ := addApp(t, `[]`, true)
			m.tour.on = tc.tour
			_, cmd := m.Update(tc.key)
			if cmd == nil {
				t.Fatal("no command: the key opened nothing")
			}
			if msg := cmd(); msg != nil {
				if _, ok := msg.(relatedPaneMsg); ok {
					t.Errorf("went to the related pane: %+v", msg)
				}
			}
			if got := readLines(t, added); len(got) != 0 {
				t.Errorf("related_editor_add ran for %q", got)
			}
			if len(*plans) != 0 {
				t.Errorf("the related pane was spawned with %+v", (*plans)[0].files)
			}
		})
	}
}

var (
	previewRefsKey    = tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl | tea.ModShift}
	previewRefsF16Key = tea.KeyPressMsg{Code: tea.KeyF16}
)

// previewApp is addApp with related_editor_preview recording, beside the add
// command, each file it is run for.
func previewApp(t *testing.T, answer string, alive bool) (appModel, string, *[]relatedPanePlan, string) {
	t.Helper()
	m, added, plans, log := addApp(t, answer, alive)
	m.engine.GetConfig().RelatedEditorPreview = `printf 'preview|%s|%s|%s\n' {file} {line} {owner} >> ` + added
	return m, added, plans, log
}

func pressRefs(t *testing.T, m appModel, key tea.KeyPressMsg) appModel {
	t.Helper()
	next, cmd := m.Update(key)
	return driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)
}

// ctrl+shift+] previews what the lines reference: the first file found, through
// related_editor_preview, in the live editor beside Monocle — which takes the
// keyboard — and nothing is added to the pane. f16 is the same key, for a
// terminal that cannot send ctrl+shift+].
func TestPreviewReferencesShowsTheFirstInThePreviewCommand(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{previewRefsKey, previewRefsF16Key} {
		m, added, plans, log := previewApp(t, `[{"path": "q.sql", "line": 12}, {"path": "a.go", "line": 5}]`, true)
		m = pressRefs(t, m, key)
		want := []string{"preview|" + filepath.Join(m.repoRoot, "q.sql") + "|12|%99"}
		if got := readLines(t, added); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: ran %q, want only the preview of the first file %q", key, got, want)
		}
		if len(*plans) != 0 {
			t.Errorf("%s: the pane was respawned", key)
		}
		if data, _ := os.ReadFile(log); !strings.Contains(string(data), "select-pane -t %7") {
			t.Errorf("%s: the editor did not get the keyboard:\n%s", key, data)
		}
		if want := "preview: q.sql · 1 more, ctrl+] opens them all"; m.statusBar.searchInfo != want {
			t.Errorf("%s: status %q, want %q", key, m.statusBar.searchInfo, want)
		}
	}
}

// With no preview command, or no live pane to preview in, the key opens the
// references as ctrl+] does.
func TestPreviewReferencesFallsBackToOpening(t *testing.T) {
	m, added, _, _ := addApp(t, `[{"path": "q.sql", "line": 12}]`, true) // no related_editor_preview
	m = pressRefs(t, m, previewRefsKey)
	if got := readLines(t, added); !reflect.DeepEqual(got, []string{filepath.Join(m.repoRoot, "q.sql") + "|12|%99"}) {
		t.Errorf("with no preview command ran %q, want the add command", got)
	}

	m, added, plans, _ := previewApp(t, `[{"path": "q.sql", "line": 12}]`, false)
	m = pressRefs(t, m, previewRefsKey)
	if got := readLines(t, added); len(got) != 0 || len(*plans) != 1 {
		t.Errorf("with no pane alive ran %q and made %d plans, want the pane spawned", got, len(*plans))
	}
}
