package tui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestParseResolved(t *testing.T) {
	got, err := parseResolved([]byte(` [{"path": "db/queries/a.sql", "line": 12}, {"path": "/abs/b.go"},
		{"path": "  "}, {"path": "c.sql", "line": -4}] `), "/repo")
	want := []relatedFile{{"/repo/db/queries/a.sql", 12}, {"/abs/b.go", 0}, {"/repo/c.sql", 0}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v %v, want %+v", got, err, want)
	}
	if got, err := parseResolved([]byte("\n"), "/repo"); err != nil || got != nil {
		t.Errorf("printing nothing gave %+v %v, want nothing to open", got, err)
	}
	if _, err := parseResolved([]byte(`{"path": "a.sql"}`), "/repo"); err == nil {
		t.Error("an object, not an array, was accepted")
	}
}

func TestMergeRelated(t *testing.T) {
	f := func(names ...string) []relatedFile {
		var out []relatedFile
		for _, n := range names {
			out = append(out, relatedFile{path: n})
		}
		return out
	}
	files, added, left := mergeRelated(f("a", "b"), f("b", "c", "c", "d"), 8)
	if !reflect.DeepEqual(files, f("a", "b", "c", "d")) || !reflect.DeepEqual(added, f("c", "d")) || left != 0 {
		t.Errorf("got %v added %v left %d: files already there are not added again, nor twice", files, added, left)
	}
	files, added, left = mergeRelated(f("a", "b"), f("c", "d", "e"), 3)
	if !reflect.DeepEqual(files, f("a", "b", "c")) || !reflect.DeepEqual(added, f("c")) || left != 2 {
		t.Errorf("got %v added %v left %d: the files already there are kept, and the rest counted", files, added, left)
	}
	files, added, left = mergeRelated(f("a", "b", "c"), f("d"), 2)
	if !reflect.DeepEqual(files, f("a", "b", "c")) || len(added) != 0 || left != 1 {
		t.Errorf("got %v added %v left %d: a stop's own related files are never dropped", files, added, left)
	}
}

var openRefsKey = tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl}

// resolveApp is tourAppWith(testTour) on 1.2 — which has a related file —
// whose walkthrough_resolve records what it is asked and prints answer.
func resolveApp(t *testing.T, answer string) (appModel, string, *[]relatedPanePlan) {
	t.Helper()
	skipWithoutSh(t)
	dir := t.TempDir()
	asked := filepath.Join(dir, "asked.json")
	reply := filepath.Join(dir, "reply.json")
	if err := os.WriteFile(reply, []byte(answer), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ := tourAppWith(t, testTour(), &types.Config{Editor: "nvim",
		WalkthroughResolve: `printf '%s' "$MONOCLE_RESOLVE_JSON" > ` + asked + `; cat ` + reply})
	m.repoRoot = dir
	m.diffView.repoRoot = dir
	m = pressKey(t, m, ".") // 1.2, b.go:30
	plans := captureRelated(t)
	return m, asked, plans
}

func openRefs(t *testing.T, m appModel) appModel {
	t.Helper()
	next, cmd := m.Update(openRefsKey)
	return driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)
}

// ctrl+] asks walkthrough_resolve about the lines @ would send, then opens its
// answer beside the stop's related files, focus moving there with the editor
// on the first file found.
func TestOpenReferences(t *testing.T) {
	m, asked, plans := resolveApp(t, `[{"path": "db/queries/find_by_email.sql", "line": 12}, {"path": "db/queries/reserve.sql"}]`)
	m = pressKey(t, m, "+")                   // b.go:30
	m = pressKey(t, pressKey(t, m, "v"), "j") // b.go:30-31
	m = openRefs(t, m)

	got := readAsk(t, asked)
	if want := (askPayload{Tour: "Tour", Stop: "1.2", Repo: m.repoRoot, Refs: []askRef{{"b.go", 30, 31, "new"}}}); !reflect.DeepEqual(got, want) {
		t.Errorf("MONOCLE_RESOLVE_JSON %+v, want %+v", got, want)
	}
	if m.diffView.visualMode || len(m.diffView.tags) != 0 {
		t.Errorf("after ctrl+]: visual=%v, %d tags left", m.diffView.visualMode, len(m.diffView.tags))
	}
	if len(*plans) != 1 {
		t.Fatalf("%d related-pane plans, want one", len(*plans))
	}
	p := (*plans)[0]
	root := m.repoRoot
	wantFiles := []relatedFile{{filepath.Join(root, "a.go"), 5}, {filepath.Join(root, "db/queries/find_by_email.sql"), 12}, {filepath.Join(root, "db/queries/reserve.sql"), 0}}
	if !reflect.DeepEqual(p.files, wantFiles) {
		t.Errorf("pane files %+v\nwant the stop's a.go, then what was found: %+v", p.files, wantFiles)
	}
	if !p.selectPane || !p.focus || !p.reveal || !strings.HasSuffix(p.argv[len(p.argv)-1], "|2wincmd w") {
		t.Errorf("plan focus=%v select=%v reveal=%v ending %q: want focus on the pane, the editor on file 2", p.focus, p.selectPane, p.reveal, p.argv[len(p.argv)-1])
	}
	if want := "opened 2: find_by_email.sql, reserve.sql"; m.statusBar.searchInfo != want {
		t.Errorf("status %q, want %q", m.statusBar.searchInfo, want)
	}
}

// What the pane already holds stays, files already open are not added again,
// and the pane holds at most maxRelatedFiles, the rest counted in the status.
func TestOpenReferencesAddsToThePane(t *testing.T) {
	var answer []string
	for i := 1; i <= 9; i++ {
		answer = append(answer, fmt.Sprintf(`{"path": "q%d.sql"}`, i))
	}
	m, _, plans := resolveApp(t, "["+strings.Join(answer, ",")+"]")
	m.tour.paneFiles = []relatedFile{{filepath.Join(m.repoRoot, "x.go"), 3}, {filepath.Join(m.repoRoot, "q1.sql"), 0}}
	m = openRefs(t, m)
	if len(*plans) != 1 {
		t.Fatalf("%d plans", len(*plans))
	}
	files := (*plans)[0].files
	if len(files) != maxRelatedFiles || files[0].path != filepath.Join(m.repoRoot, "x.go") || files[1].path != filepath.Join(m.repoRoot, "q1.sql") {
		t.Errorf("pane files %+v: want x.go and q1.sql kept first, %d in all", files, maxRelatedFiles)
	}
	if want := " · 2 left out (at most 8 files)"; !strings.HasPrefix(m.statusBar.searchInfo, "opened 6: q2.sql") || !strings.HasSuffix(m.statusBar.searchInfo, want) {
		t.Errorf("status %q, want 6 opened and %q", m.statusBar.searchInfo, want)
	}
}

func TestOpenReferencesSaysWhenThereIsNothing(t *testing.T) {
	m, _, plans := resolveApp(t, "")
	if m = openRefs(t, m); m.statusBar.searchInfo != "nothing to open on these lines" || len(*plans) != 0 {
		t.Errorf("status %q with %d plans, want nothing opened and the notice", m.statusBar.searchInfo, len(*plans))
	}

	m, _ = tourAppWith(t, testTour(), &types.Config{})
	m = pressKey(t, m, "+")
	if m = openRefs(t, m); !strings.Contains(m.statusBar.searchInfo, "walkthrough_resolve") || len(m.diffView.tags) != 1 {
		t.Errorf("with no command: %q, %d tags; want a hint and the tags kept", m.statusBar.searchInfo, len(m.diffView.tags))
	}

	m, _, _ = resolveApp(t, "not json")
	if m = openRefs(t, m); !strings.Contains(m.statusBar.searchInfo, "walkthrough_resolve failed") {
		t.Errorf("status %q, want the failure", m.statusBar.searchInfo)
	}
}

// ctrl+] never fires while typing.
func TestOpenReferencesNeverFiresWhileTyping(t *testing.T) {
	for _, opens := range []string{":", "/", "!", "c"} {
		m, asked, _ := resolveApp(t, `[{"path": "a.sql"}]`)
		m = openRefs(t, pressKey(t, m, opens))
		if _, err := os.Stat(asked); err == nil {
			t.Errorf("ctrl+] after %q ran walkthrough_resolve", opens)
		}
	}
}

// ctrl+] reaches Monocle as the control byte 0x1d; through Bubble Tea's own
// input reader it must arrive as the open-references key.
func TestCtrlCloseBracketByteIsOpenRefs(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	rec := &keyRecorder{want: 1}
	p := tea.NewProgram(rec, tea.WithInput(r), tea.WithOutput(io.Discard), tea.WithoutSignals(), tea.WithoutRenderer())
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	go func() { _, _ = w.Write([]byte{0x1d}) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		p.Kill()
		t.Fatalf("the program never saw the key; it saw %v", rec.keys)
	}
	if len(rec.keys) != 1 || !Matches(rec.keys[0], DefaultKeyMap().OpenRefs) {
		t.Errorf("0x1d arrived as %q, want the open-references key", rec.keys)
	}
}

// The pane's files are recorded once it opens them, so the next ctrl+] adds to
// them; closing the pane forgets them.
func TestThePanesFilesAreRecorded(t *testing.T) {
	files := []relatedFile{{"/repo/a.go", 5}, {"/repo/q.sql", 0}}
	m := appModel{}.handleRelatedPane(relatedPaneMsg{pane: "%5", files: files})
	if m.tour.pane != "%5" || !reflect.DeepEqual(m.tour.paneFiles, files) {
		t.Errorf("pane %q files %+v, want %%5 holding %+v", m.tour.pane, m.tour.paneFiles, files)
	}
	// Files added to the live editor go on the end of what it holds.
	more := relatedFile{"/repo/r.sql", 3}
	if m = m.handleRelatedPane(relatedPaneMsg{pane: "%5", files: []relatedFile{more}, added: true}); !reflect.DeepEqual(m.tour.paneFiles, append(files, more)) {
		t.Errorf("after an add the pane holds %+v, want %+v and r.sql", m.tour.paneFiles, files)
	}
	if m = m.handleRelatedPane(relatedPaneMsg{closed: true}); m.tour.pane != "" || m.tour.paneFiles != nil {
		t.Errorf("after X: pane %q files %+v, want both gone", m.tour.pane, m.tour.paneFiles)
	}
}

// fakeTmux puts a tmux on PATH that logs each command and answers a split
// with a new pane id, so the commands the pane is opened with can be read
// without a tmux server.
func fakeTmux(t *testing.T) string {
	t.Helper()
	skipWithoutSh(t)
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	script := "#!/bin/sh\necho \"$*\" >> " + log + "\ncase \"$1\" in split-window) echo '%7' ;; display-message) exit 1 ;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// Opening references moves focus to the pane, and the pane's files come back
// with it to be recorded.
func TestShowRelatedSelectsThePaneAndReportsItsFiles(t *testing.T) {
	log := fakeTmux(t)
	files := []relatedFile{{"/repo/a.go", 5}, {"/repo/q.sql", 0}}
	msg := showRelatedCmd("", relatedPanePlan{owner: "%1", dir: "/repo", argv: []string{"nvim", "a"}, files: files, selectPane: true})()
	got, ok := msg.(relatedPaneMsg)
	if !ok || got.err != nil || got.pane != "%7" || !reflect.DeepEqual(got.files, files) {
		t.Fatalf("got %+v, want pane %%7 holding %+v", msg, files)
	}
	data, _ := os.ReadFile(log)
	if !strings.Contains(string(data), "select-pane -t %7") {
		t.Errorf("tmux was not asked to select the pane:\n%s", data)
	}
	// Arriving at a stop does not take focus.
	_ = os.Remove(log)
	_ = showRelatedCmd("", relatedPanePlan{owner: "%1", dir: "/repo", argv: []string{"nvim", "a"}})()
	if data, _ := os.ReadFile(log); strings.Contains(string(data), "select-pane") {
		t.Errorf("a stop's related files took focus:\n%s", data)
	}
}
