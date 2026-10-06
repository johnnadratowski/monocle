package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/josephschmitt/monocle/internal/types"
)

// markLines is b.go for the symbol-mark tests: stop 1 covers 30-32, which use
// readBalance (its related file) and planDraw (its call to stop 2).
var markLines = map[int]string{
	30: "x := deps.readBalance(id)",
	31: "y := planDraw(x) + readBalanceLive()",
	32: "return readBalance",
	33: "readBalance(again)", // past the stop: never marked
}

func markTour() *types.Walkthrough {
	return &types.Walkthrough{Title: "Tour", Stops: []types.WalkthroughStop{
		{ID: "1", Title: "Draw", File: "b.go", LineStart: 30, LineEnd: 32,
			Related: []types.DocRef{{Kind: types.DocRefFile, Doc: "a.go", StartLine: 5, Symbol: "readBalance"}},
			Calls:   []types.StopCall{{Stop: "2", Symbol: "planDraw", Line: 31}}},
		{ID: "2", Title: "Plan", File: "a.go", LineStart: 10, LineEnd: 12},
	}}
}

// markApp is tourAppWith(markTour) over a b.go holding markLines, on stop 1.
func markApp(t *testing.T, cfg *types.Config) appModel {
	t.Helper()
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	b := fileDiff("b.go", 60)
	for i := range b.Hunks[0].Lines {
		if text, ok := markLines[b.Hunks[0].Lines[i].NewLineNum]; ok {
			b.Hunks[0].Lines[i].Content = text
		}
	}
	tour := markTour()
	files := []types.ChangedFile{{Path: "a.go", Status: types.FileAdded}, {Path: "b.go", Status: types.FileAdded}}
	e := &tourEngine{
		stubEngine: stubEngine{cfg: cfg, changedFiles: files,
			session: &types.ReviewSession{ID: "s", ChangedFiles: files, Walkthrough: tour, WalkthroughStop: "1"}},
		diffs: map[string]*types.DiffResult{"a.go": fileDiff("a.go", 40), "b.go": b},
	}
	m := NewApp(e)
	m = updateApp(t, m, tea.WindowSizeMsg{Width: 140, Height: 44})
	return updateApp(t, m, initialLoadMsg{files: files})
}

// marksOn names the marks on the row showing new-file line n: "related
// readBalance" or "call planDraw", in column order.
func marksOn(t *testing.T, m appModel, n int) []string {
	t.Helper()
	for _, ln := range m.diffView.lines {
		if stopLineNum(ln) != n || ln.isComment {
			continue
		}
		var out []string
		for _, r := range m.diffView.symbolRanges(ln, ln.content) {
			kind := "related"
			if r.mark.call {
				kind = "call"
			}
			out = append(out, kind+" "+ln.content[r.start:r.end])
		}
		return out
	}
	t.Fatalf("no row shows line %d", n)
	return nil
}

// In a stop's lines, the symbols its related files and calls are about are
// underlined — the whole identifier only, and nowhere past the stop.
func TestStopSymbolsAreUnderlined(t *testing.T) {
	m := markApp(t, &types.Config{})
	for n, want := range map[int][]string{
		30: {"related readBalance"},
		31: {"call planDraw"}, // readBalanceLive is another identifier
		32: {"related readBalance"},
		33: nil,
	} {
		if got := marksOn(t, m, n); !reflect.DeepEqual(got, want) {
			t.Errorf("line %d: %v, want %v", n, got, want)
		}
	}

	// The underline is styling only: the text is unchanged.
	var row diffViewLine
	for _, ln := range m.diffView.lines {
		if stopLineNum(ln) == 30 {
			row = ln
		}
	}
	plain := m.diffView.hl.highlightLine("b.go", row.content, nil, nil, nil, 60)
	marked := m.diffView.hl.highlightLineMarked("b.go", row.content, nil, nil, nil, m.diffView.symbolRanges(row, row.content), 60)
	if marked == plain || ansi.Strip(marked) != ansi.Strip(plain) {
		t.Errorf("the marked line should differ from the plain one in style alone:\nplain  %q\nmarked %q", plain, marked)
	}

	m = pressKey(t, m, "W") // the tour off
	if got := marksOn(t, m, 30); got != nil {
		t.Errorf("with the tour off line 30 is still marked: %v", got)
	}
}

// u and U move the cursor to the next and previous underlined line of the stop.
func TestUJumpsBetweenUnderlinedLines(t *testing.T) {
	m := markApp(t, &types.Config{})
	var got []string
	for _, key := range []string{"u", "u", "u", "U"} {
		m = pressKey(t, m, key)
		got = append(got, fmt.Sprintf("%s→%d", key, m.diffView.EditorTargetLine()))
	}
	if want := []string{"u→31", "u→32", "u→32", "U→31"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// o on an underlined symbol opens what it is about: its related file in the
// editor beside Monocle, with the keyboard, or the stop its call leads to.
func TestOOnAnUnderlinedSymbolOpensIt(t *testing.T) {
	skipWithoutSh(t)
	added := filepath.Join(t.TempDir(), "added")
	m := markApp(t, &types.Config{Editor: "nvim", RelatedEditorAdd: `printf '%s|%s\n' {file} {line} >> ` + added})
	captureRelated(t)
	log := fakeTmux(t)
	orig := findPane
	findPane = func(string, string) string { return "%7" }
	t.Cleanup(func() { findPane = orig })

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"}) // on 30: readBalance
	m = driveWithin(t, next.(appModel), cmd, 0, 5*time.Second)
	if got := readLines(t, added); !reflect.DeepEqual(got, []string{"a.go|5"}) {
		t.Errorf("o on readBalance ran %q, want its related file", got)
	}
	if data, _ := os.ReadFile(log); !strings.Contains(string(data), "select-pane -t %7") {
		t.Errorf("the editor did not get the keyboard:\n%s", data)
	}

	m = pressKey(t, m, "u") // 31: planDraw
	m = pressKey(t, m, "o")
	if stop, _ := m.currentStop(); stop.ID != "2" {
		t.Errorf("o on planDraw left the tour on %s, want the stop it calls, 2", stop.ID)
	}
}
