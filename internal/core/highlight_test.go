package core

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/protocol"
)

// highlight_range marks one range of lines in a file of the review: the TUI
// hears of it through an event with the path, the first line and the last.
// Clearing it is an event with no path. Paths resolve as goto_line's do.
func TestHighlightRange(t *testing.T) {
	setup := func(t *testing.T) (*Engine, *eventLog) {
		e, _ := summaryEngine(t)
		log := &eventLog{}
		e.On(EventHighlightRange, func(p EventPayload) {
			log.mu.Lock()
			log.events = append(log.events, p)
			log.mu.Unlock()
		})
		return e, log
	}
	highlight := func(e *Engine, m protocol.HighlightRangeMsg) *protocol.HighlightRangeResponse {
		m.Type = protocol.TypeHighlightRange
		return e.handleHighlightRange(&m)
	}

	t.Run("a range, then a clear", func(t *testing.T) {
		e, log := setup(t)
		abs := filepath.Join(e.current.RepoRoot, "hello.go")
		if r := highlight(e, protocol.HighlightRangeMsg{Path: abs, Start: 2, End: 4}); !r.Success || !strings.Contains(r.Message, "hello.go:2-4") {
			t.Fatalf("response = %+v", r)
		}
		if r := highlight(e, protocol.HighlightRangeMsg{Clear: true}); !r.Success {
			t.Fatalf("clear refused: %+v", r)
		}
		got := log.all()
		if len(got) != 2 || got[0].Path != "hello.go" || got[0].Line != 2 || got[0].LineEnd != 4 || got[1].Path != "" {
			t.Errorf("events = %+v, want hello.go 2-4, then a clear", got)
		}
	})

	t.Run("refusals announce nothing", func(t *testing.T) {
		e, log := setup(t)
		for _, c := range []struct {
			msg  protocol.HighlightRangeMsg
			says string
		}{
			{protocol.HighlightRangeMsg{Path: "nowhere.go", Start: 1, End: 2}, "not in the review"},
			{protocol.HighlightRangeMsg{Path: "world.go", Start: 0, End: 2}, "start"},
			{protocol.HighlightRangeMsg{Path: "world.go", Start: 5, End: 3}, "end"},
			{protocol.HighlightRangeMsg{Start: 1, End: 2}, "path"},
		} {
			if r := highlight(e, c.msg); r.Success || !strings.Contains(r.Message, c.says) {
				t.Errorf("%+v gave %+v, want a refusal about %s", c.msg, r, c.says)
			}
		}
		if got := log.all(); len(got) != 0 {
			t.Errorf("refusals announced %+v", got)
		}
	})

	t.Run("highlighting is not a handover", func(t *testing.T) {
		if isReviewSend(&protocol.HighlightRangeMsg{}) {
			t.Error("highlight_range counted as handing work to the reviewer")
		}
	})
}
