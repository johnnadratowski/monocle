package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/protocol"
)

// goto_line shows the reviewer a file of the review at a line: the TUI hears
// of it through an event carrying the path and the line. A file outside the
// review, or a line before the first, is refused and announces nothing.
func TestGotoLine(t *testing.T) {
	setup := func(t *testing.T) (*Engine, *eventLog) {
		e, _ := summaryEngine(t)
		log := &eventLog{}
		e.On(EventGotoLine, func(p EventPayload) {
			log.mu.Lock()
			log.events = append(log.events, p)
			log.mu.Unlock()
		})
		return e, log
	}
	gotoLine := func(e *Engine, path string, line int) *protocol.GotoLineResponse {
		return e.handleGotoLine(&protocol.GotoLineMsg{Type: protocol.TypeGotoLine, Path: path, Line: line})
	}

	t.Run("a changed file, by its repo path or its absolute one", func(t *testing.T) {
		e, log := setup(t)
		r := gotoLine(e, " world.go ", 3)
		if !r.Success || !strings.Contains(r.Message, "world.go:3") {
			t.Fatalf("response = %+v", r)
		}
		abs := filepath.Join(e.current.RepoRoot, "hello.go")
		if r := gotoLine(e, abs, 2); !r.Success {
			t.Fatalf("absolute path refused: %+v", r)
		}
		got := log.all()
		if len(got) != 2 || got[0].Path != "world.go" || got[0].Line != 3 || got[1].Path != "hello.go" || got[1].Line != 2 {
			t.Errorf("events = %+v, want world.go:3 then hello.go:2 (repo-relative)", got)
		}
	})

	t.Run("an added file", func(t *testing.T) {
		e, log := setup(t)
		notes := filepath.Join(t.TempDir(), "notes.md")
		if err := os.WriteFile(notes, []byte("one\ntwo\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := e.AddAdditionalPaths([]string{notes}); err != nil {
			t.Fatal(err)
		}
		if r := gotoLine(e, notes, 2); !r.Success || len(log.all()) != 1 {
			t.Errorf("response %+v, events %+v: want the added file shown", r, log.all())
		}
	})

	t.Run("refusals move nothing", func(t *testing.T) {
		e, log := setup(t)
		for _, c := range []struct {
			path string
			line int
			says string
		}{
			{"nowhere.go", 3, "not in the review"},
			{"world.go", 0, "line"},
			{"", 3, "path"},
		} {
			if r := gotoLine(e, c.path, c.line); r.Success || !strings.Contains(r.Message, c.says) {
				t.Errorf("goto %q:%d gave %+v, want a refusal about %s", c.path, c.line, r, c.says)
			}
		}
		if got := log.all(); len(got) != 0 {
			t.Errorf("refusals announced %+v", got)
		}
	})

	t.Run("pointing at a line is not a handover", func(t *testing.T) {
		if isReviewSend(&protocol.GotoLineMsg{}) {
			t.Error("goto_line counted as handing work to the reviewer")
		}
	})
}
