package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestMergeLineTags(t *testing.T) {
	for _, c := range []struct {
		name string
		in   []lineTag
		want []askRef
	}{
		{"one line", []lineTag{{"a.go", "new", 7}}, []askRef{{"a.go", 7, 7, "new"}}},
		{"adjacent lines in any order make one range",
			[]lineTag{{"a.go", "new", 314}, {"a.go", "new", 312}, {"a.go", "new", 313}},
			[]askRef{{"a.go", 312, 314, "new"}}},
		{"a line given twice counts once",
			[]lineTag{{"a.go", "new", 5}, {"a.go", "new", 6}, {"a.go", "new", 5}, {"a.go", "new", 6}},
			[]askRef{{"a.go", 5, 6, "new"}}},
		{"a gap splits the range",
			[]lineTag{{"a.go", "new", 5}, {"a.go", "new", 6}, {"a.go", "new", 8}},
			[]askRef{{"a.go", 5, 6, "new"}, {"a.go", 8, 8, "new"}}},
		{"the two sides never merge",
			[]lineTag{{"a.go", "old", 3}, {"a.go", "old", 4}, {"a.go", "new", 3}, {"a.go", "new", 4}, {"a.go", "new", 5}},
			[]askRef{{"a.go", 3, 5, "new"}, {"a.go", 3, 4, "old"}}},
		{"files never merge, and are ordered by name",
			[]lineTag{{"b.go", "new", 1}, {"a.go", "new", 2}, {"a.go", "new", 1}},
			[]askRef{{"a.go", 1, 2, "new"}, {"b.go", 1, 1, "new"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := mergeLineTags(c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
}

// mixedHunk is a change to x.go: old lines 3-4 replaced by new 3-5, then an
// unchanged line, old 5 / new 6.
func mixedHunk() types.DiffHunk {
	return types.DiffHunk{OldStart: 3, OldCount: 3, NewStart: 3, NewCount: 4, Header: "@@ -3,3 +3,4 @@", Lines: []types.DiffLine{
		{Kind: types.DiffLineRemoved, OldLineNum: 3, Content: "old three"},
		{Kind: types.DiffLineRemoved, OldLineNum: 4, Content: "old four"},
		{Kind: types.DiffLineAdded, NewLineNum: 3, Content: "new three"},
		{Kind: types.DiffLineAdded, NewLineNum: 4, Content: "new four"},
		{Kind: types.DiffLineAdded, NewLineNum: 5, Content: "new five"},
		{Kind: types.DiffLineContext, OldLineNum: 5, NewLineNum: 6, Content: "same"},
	}}
}

func mixedDiffView(style diffStyle) diffViewModel {
	theme := DefaultTheme()
	m := diffViewModel{
		theme: &theme, hl: newHighlighter(), mdStyler: newMarkdownStyler(theme),
		path: "x.go", width: 100, height: 30, style: style, hunks: []types.DiffHunk{mixedHunk()},
	}
	m.buildLines()
	return m
}

// A removed line is sent on the old side, in old-file numbers; everything else
// on the new side. Selecting the whole hunk sends both, in either view.
func TestSendSides(t *testing.T) {
	want := []askRef{{"x.go", 3, 6, "new"}, {"x.go", 3, 4, "old"}}
	for _, style := range []diffStyle{diffStyleUnified, diffStyleSplit} {
		m := mixedDiffView(style)
		m.visualMode, m.visualStart, m.cursor = true, 0, len(m.lines)-1
		if got := mergeLineTags(m.sendTags()); !reflect.DeepEqual(got, want) {
			t.Errorf("style %v: the whole hunk sent %+v, want %+v", style, got, want)
		}
	}
	// One removed row on its own, in the unified view, is its old-file line.
	m := mixedDiffView(diffStyleUnified)
	for i, l := range m.lines {
		if l.kind == types.DiffLineRemoved && l.oldLineNum == 4 {
			m.cursor = i
		}
	}
	if got := mergeLineTags(m.sendTags()); !reflect.DeepEqual(got, []askRef{{"x.go", 4, 4, "old"}}) {
		t.Errorf("the removed line 4 sent %+v", got)
	}
	// A hunk header is not a line of the file.
	m.cursor = 0
	if got := m.sendTags(); len(got) != 0 {
		t.Errorf("the hunk header sent %+v", got)
	}
	// Nor is an artifact's row.
	m = mixedDiffView(diffStyleUnified)
	m.contentMode, m.cursor = true, 1
	if got := m.sendTags(); len(got) != 0 {
		t.Errorf("an artifact row sent %+v", got)
	}
}

// askApp is tourAppWith(testTour) whose walkthrough_ask writes what it is sent
// to a file, which it returns.
func askApp(t *testing.T) (appModel, string) {
	t.Helper()
	skipWithoutSh(t)
	out := filepath.Join(t.TempDir(), "ask.json")
	m, _ := tourAppWith(t, testTour(), &types.Config{WalkthroughAsk: `printf '%s' "$MONOCLE_ASK_JSON" > ` + out})
	m.repoRoot = t.TempDir()
	return m, out
}

// send presses the send key and waits for walkthrough_ask to finish.
func send(t *testing.T, m appModel) appModel {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg{Code: '@', Text: "@"})
	return driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)
}

func readAsk(t *testing.T, out string) askPayload {
	t.Helper()
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("walkthrough_ask did not run: %v", err)
	}
	var p askPayload
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("MONOCLE_ASK_JSON %q: %v", data, err)
	}
	return p
}

func pressN(t *testing.T, m appModel, key string, n int) appModel {
	t.Helper()
	for i := 0; i < n; i++ {
		m = pressKey(t, m, key)
	}
	return m
}

// The selection and the tags go together; then the tags alone; then the line
// under the cursor. Tags outlive moving between files and stops.
func TestSendPrecedence(t *testing.T) {
	t.Run("the selection and the tags", func(t *testing.T) {
		m, out := askApp(t) // 1.1, a.go:5
		m = pressN(t, pressKey(t, m, "v"), "j", 2)
		if m = pressKey(t, m, "+"); m.diffView.visualMode || len(m.diffView.tags) != 3 {
			t.Fatalf("tagging a selection of 3 left visual=%v and %d tags", m.diffView.visualMode, len(m.diffView.tags))
		}
		m = pressKey(t, pressN(t, m, "j", 3), "+") // a.go:10
		m = pressKey(t, m, ".")                    // 1.2, b.go:30
		m = pressKey(t, pressKey(t, m, "v"), "j")  // b.go:30-31
		m = send(t, m)
		want := askPayload{Tour: "Tour", Stop: "1.2", Repo: m.repoRoot, Refs: []askRef{
			{"a.go", 5, 7, "new"}, {"a.go", 10, 10, "new"}, {"b.go", 30, 31, "new"},
		}}
		if got := readAsk(t, out); !reflect.DeepEqual(got, want) {
			t.Errorf("sent %+v\nwant %+v", got, want)
		}
		if m.diffView.visualMode || len(m.diffView.tags) != 0 {
			t.Errorf("after sending: visual=%v, %d tags left", m.diffView.visualMode, len(m.diffView.tags))
		}
		if want := "sent 1.2 a.go:5-7, a.go:10, b.go:30-31 to the agent"; m.statusBar.searchInfo != want {
			t.Errorf("status %q, want %q", m.statusBar.searchInfo, want)
		}
	})

	t.Run("the tags without the cursor's line", func(t *testing.T) {
		m, out := askApp(t)
		m = pressKey(t, m, "+")  // a.go:5
		m = pressN(t, m, "j", 4) // cursor a.go:9, not tagged
		m = send(t, m)
		if got := readAsk(t, out).Refs; !reflect.DeepEqual(got, []askRef{{"a.go", 5, 5, "new"}}) {
			t.Errorf("sent %+v, want only the tagged a.go:5", got)
		}
	})

	t.Run("else the cursor's line", func(t *testing.T) {
		m, out := askApp(t)
		m = send(t, m)
		if got := readAsk(t, out).Refs; !reflect.DeepEqual(got, []askRef{{"a.go", 5, 5, "new"}}) {
			t.Errorf("sent %+v, want the cursor's a.go:5", got)
		}
	})

	t.Run("outside tour mode the tour and stop are empty", func(t *testing.T) {
		m, out := askApp(t)
		m = send(t, pressKey(t, m, "W"))
		if got := readAsk(t, out); got.Tour != "" || got.Stop != "" || len(got.Refs) != 1 {
			t.Errorf("sent %+v with the tour off", got)
		}
	})

	t.Run("no command: a hint, and the tags stay", func(t *testing.T) {
		m, _ := tourAppWith(t, testTour(), &types.Config{})
		m = pressKey(t, m, "+")
		m = send(t, m)
		if !strings.Contains(m.statusBar.searchInfo, "walkthrough_ask") || len(m.diffView.tags) != 1 {
			t.Errorf("with no command: %q, %d tags", m.statusBar.searchInfo, len(m.diffView.tags))
		}
	})

	t.Run("a failing command says so", func(t *testing.T) {
		skipWithoutSh(t)
		m, _ := tourAppWith(t, testTour(), &types.Config{WalkthroughAsk: "echo no agent window >&2; exit 3"})
		if m = send(t, m); !strings.Contains(m.statusBar.searchInfo, "failed") || !strings.Contains(m.statusBar.searchInfo, "no agent window") {
			t.Errorf("status %q, want the failure and its reason", m.statusBar.searchInfo)
		}
	})
}

// + on lines all tagged untags them; on a selection only partly tagged it
// tags the rest. The gutter marks a tagged line in the tag colour.
func TestTagToggling(t *testing.T) {
	m, _ := tourAppWith(t, testTour(), &types.Config{}) // a.go:5
	m = pressKey(t, m, "+")
	m = pressKey(t, pressKey(t, pressKey(t, m, "k"), "v"), "j") // 4-5, 5 tagged
	if m = pressKey(t, m, "+"); len(m.diffView.tags) != 2 {
		t.Fatalf("a partly tagged selection left %d tags, want both lines tagged", len(m.diffView.tags))
	}
	m = pressKey(t, pressKey(t, m, "v"), "k") // 5-4 again, all tagged
	if m = pressKey(t, m, "+"); len(m.diffView.tags) != 0 {
		t.Errorf("a wholly tagged selection left %d tags, want none", len(m.diffView.tags))
	}

	m = pressKey(t, m, "+") // a.go:4
	base := lipgloss.NewStyle()
	tagged := base.Foreground(lipgloss.Color("0")).Background(lipgloss.Color(tagGutterColor)).Bold(true)
	if tagged.Render("12") == base.Render("12") {
		t.Fatal("setup: the tag style renders like no style at all")
	}
	for _, l := range m.diffView.lines {
		got := m.diffView.markGutter(l, base).Render("12")
		switch {
		case l.newLineNum == 4 && got != tagged.Render("12"):
			t.Error("the tagged line 4 is not marked in the tag colour")
		case l.newLineNum == 3 && got != base.Render("12"):
			t.Error("the untagged line 3 is marked")
		}
	}
}

// Typing never tags or sends: in every text input + and @ are text.
func TestTagAndSendKeysNeverFireWhileTyping(t *testing.T) {
	for _, in := range []struct {
		name  string
		opens string
		typed func(appModel) string
	}{
		{"command", ":", func(m appModel) string { return m.commandBuffer }},
		{"search", "/", func(m appModel) string { return m.searchBuffer }},
		{"shell", "!", func(m appModel) string { return m.shellBuffer }},
		{"comment", "c", func(m appModel) string { return m.commentEditor.body }},
	} {
		t.Run(in.name, func(t *testing.T) {
			m, out := askApp(t)
			m = pressKey(t, m, in.opens)
			m = pressKey(t, m, "+")
			m = send(t, m)
			if len(m.diffView.tags) != 0 {
				t.Errorf("+ typed into the %s tagged a line", in.name)
			}
			if _, err := os.Stat(out); err == nil {
				t.Errorf("@ typed into the %s sent to the agent", in.name)
			}
			if !strings.Contains(in.typed(m), "+@") {
				t.Errorf("the %s holds %q, want the typed +@", in.name, in.typed(m))
			}
		})
	}
}

// An attached file is shown under its absolute path; the agent names files
// from the repo root, so one inside the repo is sent relative to it.
func TestRepoRelative(t *testing.T) {
	for _, c := range []struct{ root, path, want string }{
		{"/repo", "/repo/docs/a.md", "docs/a.md"},
		{"/repo", "/elsewhere/a.md", "/elsewhere/a.md"},
		{"/repo", "/repository/a.md", "/repository/a.md"},
		{"/repo", "src/a.go", "src/a.go"},
		{"", "/repo/a.go", "/repo/a.go"},
	} {
		if got := repoRelative(c.root, c.path); got != c.want {
			t.Errorf("repoRelative(%q, %q) = %q, want %q", c.root, c.path, got, c.want)
		}
	}
}
