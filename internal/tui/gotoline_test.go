package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

// goto_line shows a file at a line: the diff cursor on it, the diff focused, a
// jump recorded so ctrl+o returns. It is not a tour move.
func TestGotoLineShowsTheFileAtTheLine(t *testing.T) {
	m, e := tourApp(t) // on 1.1, a.go:5
	m.setFocus(focusDoc)
	m = updateApp(t, m, gotoLineMsg{path: "b.go", line: 42})
	if m.diffView.path != "b.go" || cursorLine(m) != 42 || m.focus != focusMain {
		t.Fatalf("at %s:%d focus %v, want b.go:42 with the diff focused", m.diffView.path, cursorLine(m), m.focus)
	}
	if m.tour.index != 0 || len(e.reports()) != 0 {
		t.Errorf("the tour moved: index %d, reported %v", m.tour.index, e.reports())
	}
	if !strings.Contains(m.statusBar.searchInfo, "b.go:42") {
		t.Errorf("status %q, want it to say where it went", m.statusBar.searchInfo)
	}
	m = updateApp(t, m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	if m.diffView.path != "a.go" || cursorLine(m) != 5 {
		t.Errorf("ctrl+o went to %s:%d, want back to a.go:5", m.diffView.path, cursorLine(m))
	}

	// In the file already on screen it only moves the cursor.
	m = updateApp(t, m, gotoLineMsg{path: "a.go", line: 30})
	if m.diffView.path != "a.go" || cursorLine(m) != 30 {
		t.Errorf("at %s:%d, want a.go:30", m.diffView.path, cursorLine(m))
	}
}

// An added file is shown the same way; a path the review does not hold is
// refused, and nothing moves.
func TestGotoLineAddedAndUnknownFiles(t *testing.T) {
	notes := filepath.Join(t.TempDir(), "notes.md")
	var b strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&b, "note line %d\n", i)
	}
	if err := os.WriteFile(notes, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	m, e := tourApp(t)
	added := []types.AdditionalFile{{Path: notes, Name: "notes.md"}}
	e.additionalFiles = added
	m = updateApp(t, m, initialLoadMsg{files: e.changedFiles, additionalFiles: added})

	m = updateApp(t, m, gotoLineMsg{path: "notes.md", line: 20})
	if m.diffView.additionalFilePath != notes || cursorLine(m) != 20 {
		t.Errorf("at %q:%d, want the added notes.md at line 20", m.diffView.additionalFilePath, cursorLine(m))
	}

	path, line := m.diffView.additionalFilePath, cursorLine(m)
	m = updateApp(t, m, gotoLineMsg{path: "nowhere.go", line: 3})
	if !strings.Contains(m.statusBar.searchInfo, "nowhere.go is not in the review") {
		t.Errorf("status %q, want the refusal", m.statusBar.searchInfo)
	}
	if m.diffView.additionalFilePath != path || cursorLine(m) != line {
		t.Error("a refused goto moved the diff")
	}
}

func (e *tourEngine) GetAdditionalFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	return string(data), err
}
