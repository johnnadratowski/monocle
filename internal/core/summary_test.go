package core

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/db"
	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
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

// The overview is the round in a sentence; it belongs to the round and has to
// survive a restart and vanish with it, exactly as the items do.
func TestSummaryOverview(t *testing.T) {
	t.Run("is stored and capped", func(t *testing.T) {
		e, _ := summaryEngine(t)
		long := strings.Repeat("word ", 300)
		r := e.handleSetReviewSummary(&protocol.SetReviewSummaryMsg{
			Type: protocol.TypeSetReviewSummary, Overview: long,
			Items: []protocol.SummaryItemEntry{{ID: "a", Text: "a fix"}},
		})
		if !r.Success {
			t.Fatalf("response = %+v", r)
		}
		got := e.current.SummaryOverview
		if len([]rune(got)) != types.SummaryOverviewLimit {
			t.Errorf("overview is %d runes, want it capped at %d", len([]rune(got)), types.SummaryOverviewLimit)
		}
		if !strings.HasSuffix(got, "…") {
			t.Error("a trimmed overview must show that it was cut")
		}
	})

	t.Run("survives a restart", func(t *testing.T) {
		e, dbPath := summaryEngine(t)
		repo := e.current.RepoRoot
		e.handleSetReviewSummary(&protocol.SetReviewSummaryMsg{
			Type: protocol.TypeSetReviewSummary, Overview: "two halves of one fix",
			Items: []protocol.SummaryItemEntry{{ID: "a", Text: "a fix"}},
		})
		sessionID := e.current.ID

		database, err := db.Open(dbPath)
		if err != nil {
			t.Fatalf("reopen db: %v", err)
		}
		defer database.Close()
		e2, err := NewEngine(DefaultConfig(), database, repo, false)
		if err != nil {
			t.Fatalf("new engine: %v", err)
		}
		resumed, err := e2.ResumeSession(sessionID)
		if err != nil {
			t.Fatalf("resume: %v", err)
		}
		if got := resumed.SummaryOverview; got != "two halves of one fix" {
			t.Errorf("overview after restart = %q", got)
		}
	})
}

// An item's own line is capped too: one long item would crowd out the list it
// belongs to.
func TestSummaryItemTextIsCapped(t *testing.T) {
	e, _ := summaryEngine(t)
	e.handleSetReviewSummary(items(protocol.SummaryItemEntry{
		ID: "a", Text: strings.Repeat("verbose ", 60),
	}))
	got := e.current.SummaryItems[0].Text
	if len([]rune(got)) != types.SummaryTextLimit {
		t.Errorf("item text is %d runes, want %d", len([]rune(got)), types.SummaryTextLimit)
	}
}

// A cap that applies silently is a cap the sender cannot correct for — jaa's
// 155-character item came back as success with no sign it had been cut.
func TestTrimmingIsReported(t *testing.T) {
	e, _ := summaryEngine(t)
	r := e.handleSetReviewSummary(&protocol.SetReviewSummaryMsg{
		Type:     protocol.TypeSetReviewSummary,
		Overview: strings.Repeat("long ", 200),
		Items: []protocol.SummaryItemEntry{
			{ID: "short", Text: "a brief line"},
			{ID: "long", Text: strings.Repeat("verbose ", 40)},
		},
	})
	if !r.Success {
		t.Fatalf("response = %+v", r)
	}
	want := map[string]bool{"long": true, "overview": true}
	got := map[string]bool{}
	for _, id := range r.Truncated {
		got[id] = true
	}
	if len(got) != len(want) {
		t.Fatalf("truncated = %v, want exactly %v", r.Truncated, want)
	}
	for id := range want {
		if !got[id] {
			t.Errorf("truncated = %v, missing %q", r.Truncated, id)
		}
	}
	if !strings.Contains(r.Message, "Trimmed to the length cap") {
		t.Errorf("message = %q, want it to say what was trimmed", r.Message)
	}
}

// An absent key reads the same as an engine too old to check, so both lists are
// emitted even when empty.
func TestCleanSummaryStillEmitsBothLists(t *testing.T) {
	e, _ := summaryEngine(t)
	r := e.handleSetReviewSummary(items(protocol.SummaryItemEntry{ID: "a", Text: "a brief line"}))
	if r.Unmatched == nil || r.Truncated == nil {
		t.Fatalf("unmatched=%v truncated=%v, want empty lists rather than nil", r.Unmatched, r.Truncated)
	}
	blob, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"unmatched":[]`, `"truncated":[]`} {
		if !strings.Contains(string(blob), key) {
			t.Errorf("response JSON %s lacks %s", blob, key)
		}
	}
}

// Reflowing whitespace is not truncation; only a line that actually loses text
// should be reported, or the report becomes noise a sender learns to ignore.
func TestWhitespaceCollapseIsNotReportedAsTrimming(t *testing.T) {
	e, _ := summaryEngine(t)
	r := e.handleSetReviewSummary(items(protocol.SummaryItemEntry{
		ID: "a", Text: "  a   line   with   loose   spacing  ",
	}))
	if len(r.Truncated) != 0 {
		t.Errorf("truncated = %v, want none — nothing was cut", r.Truncated)
	}
	if got := e.current.SummaryItems[0].Text; got != "a line with loose spacing" {
		t.Errorf("text = %q, want it collapsed", got)
	}
}
