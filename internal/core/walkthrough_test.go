package core

import (
	"strings"
	"sync"
	"testing"

	"github.com/josephschmitt/monocle/internal/db"
	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
)

func tourMsg(stops ...types.WalkthroughStop) *protocol.SetWalkthroughMsg {
	return &protocol.SetWalkthroughMsg{Type: protocol.TypeSetWalkthrough, Walkthrough: types.Walkthrough{Title: "Tour", Stops: stops}}
}

// eventLog records walkthrough events, so a test can tell which moves the
// engine announced and which it only recorded.
type eventLog struct {
	mu     sync.Mutex
	events []EventPayload
}

func watchTour(e *Engine) *eventLog {
	l := &eventLog{}
	e.On(EventWalkthroughChanged, func(p EventPayload) {
		l.mu.Lock()
		l.events = append(l.events, p)
		l.mu.Unlock()
	})
	return l
}

func (l *eventLog) all() []EventPayload {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]EventPayload(nil), l.events...)
}

func TestSetWalkthrough(t *testing.T) {
	t.Run("stores the tour and starts on the first stop", func(t *testing.T) {
		e, _ := summaryEngine(t)
		log := watchTour(e)
		r := e.handleSetWalkthrough(tourMsg(
			types.WalkthroughStop{ID: "1.1", File: "hello.go", LineStart: 3},
			types.WalkthroughStop{ID: "1.2", File: "world.go"},
		))
		if !r.Success || r.Count != 2 || r.Current != "1.1" {
			t.Fatalf("response = %+v", r)
		}
		if e.current.Walkthrough == nil || len(e.current.Walkthrough.Stops) != 2 || e.current.WalkthroughStop != "1.1" {
			t.Fatalf("session tour = %+v at %q", e.current.Walkthrough, e.current.WalkthroughStop)
		}
		// The TUI learns of a tour only through this event.
		if got := log.all(); len(got) != 1 || got[0].Status != WalkthroughEventSet || got[0].ItemID != "1.1" {
			t.Errorf("events = %+v, want one set event for 1.1", got)
		}
	})

	// A re-send is how the agent fixes a typo in stop 3 while the reviewer is on
	// stop 5. Throwing them back to the start for it would punish the fix.
	t.Run("a replace keeps the reviewer's stop when its id survives", func(t *testing.T) {
		e, _ := summaryEngine(t)
		e.handleSetWalkthrough(tourMsg(types.WalkthroughStop{ID: "1.1"}, types.WalkthroughStop{ID: "1.2"}))
		if err := e.SetWalkthroughStop("1.2"); err != nil {
			t.Fatal(err)
		}
		r := e.handleSetWalkthrough(tourMsg(types.WalkthroughStop{ID: "0.9"}, types.WalkthroughStop{ID: "1.2", Note: "fixed"}))
		if r.Current != "1.2" || e.current.WalkthroughStop != "1.2" {
			t.Errorf("current = %q / %q, want 1.2 kept", r.Current, e.current.WalkthroughStop)
		}
		if e.current.Walkthrough.Stops[1].Note != "fixed" {
			t.Error("the replace should carry the new content")
		}
	})

	t.Run("a replace that drops the reviewer's stop starts from the top", func(t *testing.T) {
		e, _ := summaryEngine(t)
		e.handleSetWalkthrough(tourMsg(types.WalkthroughStop{ID: "1.1"}, types.WalkthroughStop{ID: "1.2"}))
		_ = e.SetWalkthroughStop("1.2")
		r := e.handleSetWalkthrough(tourMsg(types.WalkthroughStop{ID: "2.1"}, types.WalkthroughStop{ID: "2.2"}))
		if r.Current != "2.1" {
			t.Errorf("current = %q, want the first stop", r.Current)
		}
	})

	t.Run("an empty tour withdraws it", func(t *testing.T) {
		e, _ := summaryEngine(t)
		e.handleSetWalkthrough(tourMsg(types.WalkthroughStop{ID: "1.1"}))
		log := watchTour(e)
		r := e.handleSetWalkthrough(tourMsg())
		if !r.Success || e.current.Walkthrough != nil || e.current.WalkthroughStop != "" {
			t.Fatalf("response %+v, tour %+v", r, e.current.Walkthrough)
		}
		if got := log.all(); len(got) != 1 || got[0].Status != WalkthroughEventCleared {
			t.Errorf("events = %+v, want one cleared event", got)
		}
	})

	t.Run("a duplicate id is refused and the old tour stands", func(t *testing.T) {
		e, _ := summaryEngine(t)
		e.handleSetWalkthrough(tourMsg(types.WalkthroughStop{ID: "keep"}))
		r := e.handleSetWalkthrough(tourMsg(types.WalkthroughStop{ID: "a"}, types.WalkthroughStop{ID: "a"}))
		if r.Success || !strings.Contains(r.Message, "duplicate") {
			t.Errorf("response = %+v, want a duplicate-id refusal", r)
		}
		if e.current.Walkthrough.Stops[0].ID != "keep" {
			t.Error("a refused tour must not replace the standing one")
		}
	})

	t.Run("warns about what the reviewer cannot be shown", func(t *testing.T) {
		e, _ := summaryEngine(t)
		r := e.handleSetWalkthrough(tourMsg(
			types.WalkthroughStop{ID: "ok", File: "hello.go", Related: []types.DocRef{{Doc: "world.go"}}},
			types.WalkthroughStop{ID: "bad", File: "nope.go", Related: []types.DocRef{{Doc: "missing.go"}}},
			types.WalkthroughStop{ID: "video"}, // no file anchor is a stop, not a mistake
		))
		if !r.Success {
			t.Fatalf("a tour with warnings is still set: %+v", r)
		}
		if len(r.Warnings) != 2 || !strings.Contains(r.Warnings[0], "nope.go") || !strings.Contains(r.Warnings[1], "missing.go") {
			t.Errorf("warnings = %q, want the off-review file and the missing related file", r.Warnings)
		}
		if !strings.Contains(r.Message, "nope.go") {
			t.Error("the warnings belong in the message the agent reads")
		}
	})

	t.Run("survives a restart, stop and all", func(t *testing.T) {
		e, dbPath := summaryEngine(t)
		repo := e.current.RepoRoot
		e.handleSetWalkthrough(tourMsg(
			types.WalkthroughStop{ID: "1.1", Views: []types.StopView{{Kind: types.StopViewVideo, Target: "demo.webm"}}},
			types.WalkthroughStop{ID: "1.2"},
		))
		_ = e.SetWalkthroughStop("1.2")
		sessionID := e.current.ID

		database2, err := db.Open(dbPath)
		if err != nil {
			t.Fatalf("reopen db: %v", err)
		}
		defer database2.Close()
		e2, err := NewEngine(DefaultConfig(), database2, repo, false)
		if err != nil {
			t.Fatalf("new engine: %v", err)
		}
		resumed, err := e2.ResumeSession(sessionID)
		if err != nil {
			t.Fatalf("resume: %v", err)
		}
		if resumed.Walkthrough == nil || len(resumed.Walkthrough.Stops) != 2 || resumed.WalkthroughStop != "1.2" {
			t.Fatalf("resumed %+v at %q, want both stops at 1.2", resumed.Walkthrough, resumed.WalkthroughStop)
		}
		if resumed.Walkthrough.Stops[0].Views[0].Target != "demo.webm" {
			t.Error("views should round-trip through the database")
		}
	})

	// A tour describes one review; clearing the review must not leave the next
	// one opening on stale stops.
	t.Run("clearing the review drops it", func(t *testing.T) {
		e, _ := summaryEngine(t)
		e.handleSetWalkthrough(tourMsg(types.WalkthroughStop{ID: "1.1"}))
		if err := e.ClearReview(); err != nil {
			t.Fatal(err)
		}
		if e.current.Walkthrough != nil {
			t.Error("tour survived a clear")
		}
		if w, _, _ := e.database.GetWalkthrough(e.current.ID); w != nil {
			t.Error("tour survived a clear in the database")
		}
	})

	t.Run("approving the review drops it", func(t *testing.T) {
		e, _ := summaryEngine(t)
		e.handleSetWalkthrough(tourMsg(types.WalkthroughStop{ID: "1.1"}))
		if err := e.Submit(types.ActionApprove, ""); err != nil {
			t.Fatal(err)
		}
		if e.current.Walkthrough != nil {
			t.Error("tour survived an approval")
		}
	})

	t.Run("refuses without a session", func(t *testing.T) {
		e := &Engine{}
		if r := e.handleSetWalkthrough(tourMsg(types.WalkthroughStop{ID: "1"})); r.Success {
			t.Error("there is nothing to tour without a session")
		}
	})
}

func TestGotoStop(t *testing.T) {
	setup := func(t *testing.T) (*Engine, *eventLog) {
		e, _ := summaryEngine(t)
		e.handleSetWalkthrough(tourMsg(
			types.WalkthroughStop{ID: "1.1", Title: "Entry"},
			types.WalkthroughStop{ID: "1.2", Title: "Where it lands"},
		))
		return e, watchTour(e)
	}

	t.Run("the agent's move is announced to the TUI", func(t *testing.T) {
		e, log := setup(t)
		r := e.handleGotoStop(&protocol.GotoStopMsg{Type: protocol.TypeGotoStop, ID: " 1.2 "})
		if !r.Success || !strings.Contains(r.Message, "1.2 · Where it lands") || !strings.Contains(r.Message, "was on 1.1") {
			t.Fatalf("response = %+v", r)
		}
		if e.current.WalkthroughStop != "1.2" {
			t.Errorf("current = %q", e.current.WalkthroughStop)
		}
		if got := log.all(); len(got) != 1 || got[0].Status != WalkthroughEventGoto || got[0].ItemID != "1.2" {
			t.Errorf("events = %+v, want one goto 1.2", got)
		}
	})

	// The TUI reports a move it already made. Announcing it would bounce the TUI
	// back to where it is, re-running every side effect of arriving there.
	t.Run("the reviewer's own move is recorded silently", func(t *testing.T) {
		e, log := setup(t)
		r := e.handleSetWalkthroughStop(&protocol.SetWalkthroughStopMsg{Type: protocol.TypeSetWalkthroughStop, ID: "1.2"})
		if r.Error != "" || e.current.WalkthroughStop != "1.2" {
			t.Fatalf("response %+v, current %q", r, e.current.WalkthroughStop)
		}
		if got := log.all(); len(got) != 0 {
			t.Errorf("events = %+v, want none", got)
		}
	})

	t.Run("an unknown id names the ones that exist", func(t *testing.T) {
		e, log := setup(t)
		r := e.handleGotoStop(&protocol.GotoStopMsg{Type: protocol.TypeGotoStop, ID: "9.9"})
		if r.Success || !strings.Contains(r.Message, "1.1, 1.2") {
			t.Errorf("response = %+v, want the valid ids listed", r)
		}
		if e.current.WalkthroughStop != "1.1" || len(log.all()) != 0 {
			t.Error("a refused move must not move anything")
		}
	})

	t.Run("no tour is an error, not a silent no-op", func(t *testing.T) {
		e, _ := summaryEngine(t)
		if r := e.handleGotoStop(&protocol.GotoStopMsg{Type: protocol.TypeGotoStop, ID: "1"}); r.Success {
			t.Error("goto without a tour should fail")
		}
	})
}

// A tour is work put in front of the reviewer, so it starts the review's clock
// like a summary or an artifact does. Pointing at a stop is not.
func TestWalkthroughHandover(t *testing.T) {
	e, _ := summaryEngine(t)
	srv := &SocketServer{engine: e}
	srv.handleMessage(tourMsg(types.WalkthroughStop{ID: "1.1"}))
	if e.current.SentAt.IsZero() {
		t.Error("sending a tour should stamp the review as handed over")
	}
	if isReviewSend(&protocol.GotoStopMsg{}) || isReviewSend(&protocol.SetWalkthroughStopMsg{}) {
		t.Error("moving between stops is not a handover")
	}
}

func TestWalkthroughWarnings(t *testing.T) {
	w := types.Walkthrough{Stops: []types.WalkthroughStop{
		{ID: "a", File: "in.go", Related: []types.DocRef{{Doc: "there.go"}, {Doc: "gone.go"}}},
		{ID: "b", File: "out.go"},
		{ID: "c"},
	}}
	got := walkthroughWarnings(w,
		func(p string) bool { return p == "in.go" },
		func(p string) bool { return p != "gone.go" },
	)
	if len(got) != 2 || !strings.Contains(got[0], "stop a: related file gone.go") || !strings.Contains(got[1], "stop b: out.go") {
		t.Errorf("warnings = %q", got)
	}
}
