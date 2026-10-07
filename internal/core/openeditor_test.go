package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/protocol"
)

// open_editor asks the TUI to open any file of the repo at a line in the
// editor beside it — not only the review's files. The TUI hears of it through
// an event with the repo-relative path, the line and whether to fill the
// window. A path outside the repo, a missing file and a line before the first
// are refused.
func TestOpenEditor(t *testing.T) {
	e, _ := summaryEngine(t)
	log := &eventLog{}
	e.On(EventOpenEditor, func(p EventPayload) {
		log.mu.Lock()
		log.events = append(log.events, p)
		log.mu.Unlock()
	})
	repo := e.current.RepoRoot
	if err := os.MkdirAll(filepath.Join(repo, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "lib", "util.go"), []byte("package lib\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "elsewhere.go")
	if err := os.WriteFile(outside, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	open := func(path string, line int, full bool) *protocol.OpenEditorResponse {
		return e.handleOpenEditor(&protocol.OpenEditorMsg{Type: protocol.TypeOpenEditor, Path: path, Line: line, Full: full})
	}

	if r := open("lib/util.go", 12, false); !r.Success || !strings.Contains(r.Message, "lib/util.go:12") {
		t.Fatalf("a repo file outside the review: %+v", r)
	}
	if r := open(filepath.Join(repo, "hello.go"), 3, true); !r.Success {
		t.Fatalf("an absolute path in the repo: %+v", r)
	}
	got := log.all()
	if len(got) != 2 || got[0].Path != "lib/util.go" || got[0].Line != 12 || got[0].Full ||
		got[1].Path != "hello.go" || got[1].Line != 3 || !got[1].Full {
		t.Errorf("events = %+v, want lib/util.go:12, then hello.go:3 full", got)
	}

	for _, c := range []struct {
		path string
		line int
		says string
	}{
		{outside, 1, "outside the repo"},
		{"../escape.go", 1, "outside the repo"},
		{"nope.go", 1, "no such file"},
		{"lib", 1, "not a file"},
		{"lib/util.go", 0, "line"},
		{"", 1, "path"},
	} {
		if r := open(c.path, c.line, false); r.Success || !strings.Contains(r.Message, c.says) {
			t.Errorf("open %q:%d gave %+v, want a refusal about %s", c.path, c.line, r, c.says)
		}
	}
	if got := log.all(); len(got) != 2 {
		t.Errorf("refusals announced %+v", got[2:])
	}
	if isReviewSend(&protocol.OpenEditorMsg{}) {
		t.Error("open_editor counted as handing work to the reviewer")
	}
}
