package core

import (
	"path/filepath"
	"testing"

	"github.com/josephschmitt/monocle/internal/db"
	"github.com/josephschmitt/monocle/internal/protocol"
)

func summaryEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	repo, _ := setupTestRepo(t)
	dbPath := filepath.Join(t.TempDir(), "monocle.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	e, err := NewEngine(DefaultConfig(), database, repo, false)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	if _, err := e.StartSession(SessionOptions{Agent: "claude", RepoRoot: repo}); err != nil {
		t.Fatalf("start session: %v", err)
	}
	return e, dbPath
}

func items(entries ...protocol.SummaryItemEntry) *protocol.SetReviewSummaryMsg {
	return &protocol.SetReviewSummaryMsg{Type: protocol.TypeSetReviewSummary, Items: entries}
}

func TestSetReviewSummary(t *testing.T) {
	t.Run("stores items in reading order", func(t *testing.T) {
		e, _ := summaryEngine(t)
		r := e.handleSetReviewSummary(items(
			protocol.SummaryItemEntry{ID: "b", Text: "second thing", Order: 2},
			protocol.SummaryItemEntry{ID: "a", Text: "first thing", Order: 1,
				Targets: []protocol.SummaryTargetEntry{{Path: "hello.go", LineStart: 1, LineEnd: 9}}},
		))
		if !r.Success || r.Count != 2 {
			t.Fatalf("response = %+v", r)
		}
		got := e.current.SummaryItems
		if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
			t.Fatalf("items = %+v, want a then b", got)
		}
		if !got[0].Covers("hello.go", 5) {
			t.Error("the first item should cover its own target range")
		}
	})

	// A re-send is a new account of the round, not an addition to the old one.
	t.Run("a re-send replaces rather than accumulates", func(t *testing.T) {
		e, _ := summaryEngine(t)
		e.handleSetReviewSummary(items(protocol.SummaryItemEntry{ID: "a", Text: "old"}))
		e.handleSetReviewSummary(items(protocol.SummaryItemEntry{ID: "b", Text: "new"}))
		got := e.current.SummaryItems
		if len(got) != 1 || got[0].ID != "b" {
			t.Errorf("items = %+v, want only the newer one", got)
		}
	})

	t.Run("an empty list withdraws it", func(t *testing.T) {
		e, _ := summaryEngine(t)
		e.handleSetReviewSummary(items(protocol.SummaryItemEntry{ID: "a", Text: "something"}))
		r := e.handleSetReviewSummary(items())
		if !r.Success || r.Count != 0 {
			t.Fatalf("response = %+v", r)
		}
		if len(e.current.SummaryItems) != 0 {
			t.Errorf("items = %+v, want none", e.current.SummaryItems)
		}
	})

	t.Run("survives a restart", func(t *testing.T) {
		e, dbPath := summaryEngine(t)
		repo := e.current.RepoRoot
		e.handleSetReviewSummary(items(protocol.SummaryItemEntry{ID: "keep", Text: "kept across restart",
			Targets: []protocol.SummaryTargetEntry{{Path: "hello.go"}}}))
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
		if len(resumed.SummaryItems) != 1 || resumed.SummaryItems[0].ID != "keep" {
			t.Fatalf("items = %+v, want the stored one", resumed.SummaryItems)
		}
		if !resumed.SummaryItems[0].ClaimsFile("hello.go") {
			t.Error("targets should round-trip through the database")
		}
	})

	// The summary describes one round, so it goes when the round does.
	t.Run("clearing the review clears it", func(t *testing.T) {
		e, _ := summaryEngine(t)
		e.handleSetReviewSummary(items(protocol.SummaryItemEntry{ID: "a", Text: "something"}))
		if err := e.ClearReview(); err != nil {
			t.Fatalf("clear: %v", err)
		}
		if len(e.current.SummaryItems) != 0 {
			t.Errorf("items = %+v, want none after a clear", e.current.SummaryItems)
		}
	})

	t.Run("refuses without a session", func(t *testing.T) {
		e := &Engine{}
		if r := e.handleSetReviewSummary(items()); r.Success {
			t.Error("there is nothing to describe without a session")
		}
	})
}

// A summary is the agent handing work over, so it starts the review's clock the
// same way naming it or sending an artifact does.
func TestSummaryCountsAsAHandover(t *testing.T) {
	e, _ := summaryEngine(t)
	srv := &SocketServer{engine: e}
	srv.handleMessage(items(protocol.SummaryItemEntry{ID: "a", Text: "fixed the thing"}))
	if e.current.SentAt.IsZero() {
		t.Error("sending a summary should stamp the review as handed over")
	}
}

func TestReviewCommits(t *testing.T) {
	// A working-tree review has no commits: base and HEAD are the same. That is
	// the normal case, not a failure.
	t.Run("uncommitted work has no commits", func(t *testing.T) {
		e, _ := summaryEngine(t)
		commits, _, err := e.ReviewCommits(50)
		if err != nil {
			t.Fatalf("ReviewCommits: %v", err)
		}
		if len(commits) != 0 {
			t.Errorf("commits = %+v, want none for a working-tree review", commits)
		}
	})

	t.Run("no session is not an error", func(t *testing.T) {
		e := &Engine{}
		commits, base, err := e.ReviewCommits(50)
		if err != nil || len(commits) != 0 || base != "" {
			t.Errorf("got %+v %q %v, want empty and no error", commits, base, err)
		}
	})
}

// The count in review status is the only read-back of an attached summary:
// set_review_summary replaces wholesale, so re-sending to see what is there
// would destroy it. A coordinator checking "did the lane actually send one"
// must be able to ask without writing.
func TestReviewStatusReportsSummaryItemCount(t *testing.T) {
	e, _ := summaryEngine(t)

	if got := e.GetReviewStatusInfo().SummaryItems; got != 0 {
		t.Fatalf("summary items = %d before any summary, want 0", got)
	}

	e.handleSetReviewSummary(items(
		protocol.SummaryItemEntry{ID: "a", Text: "first thing", Order: 1},
		protocol.SummaryItemEntry{ID: "b", Text: "second thing", Order: 2},
	))
	if got := e.GetReviewStatusInfo().SummaryItems; got != 2 {
		t.Errorf("summary items = %d, want 2", got)
	}
	if got := e.handleGetReviewStatus(&protocol.GetReviewStatusMsg{}).SummaryItems; got != 2 {
		t.Errorf("summary items over the wire = %d, want 2", got)
	}

	// Withdrawing has to move the count back down, or a stale "2 attached"
	// reads as a summary that is no longer there.
	e.handleSetReviewSummary(items())
	if got := e.GetReviewStatusInfo().SummaryItems; got != 0 {
		t.Errorf("summary items = %d after withdrawal, want 0", got)
	}
}
