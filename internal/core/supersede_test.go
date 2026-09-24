package core

import (
	"strings"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
)

// The hazard this replaces, seen on a real lane: an approval was queued for one
// round, the agent staged an entirely different review, and the next
// get_feedback would have returned "Approved" for work nobody had looked at.
func TestStagingANewRoundRetiresAnOlderVerdict(t *testing.T) {
	q := NewFeedbackQueue()
	q.Submit(&FormattedReview{Formatted: "## Review — Approved", Action: "approve", Round: 48}, false)

	dropped := q.SupersedeBefore(49)
	if len(dropped) != 1 || dropped[0].Round != 48 {
		t.Fatalf("dropped = %+v, want the round-48 verdict", dropped)
	}
	if got := q.PollWithInfo(""); got != nil && len(got.Reviews) > 0 {
		t.Fatalf("a retired verdict must not be deliverable, got %+v", got.Reviews)
	}
	if q.GetStatus() != "none" {
		t.Errorf("status = %q, want none once nothing is queued", q.GetStatus())
	}
}

// The reviewer's verdict on the round being staged is not stale — only earlier
// ones are. Dropping the current round's verdict would lose a real review.
func TestTheCurrentRoundsVerdictSurvives(t *testing.T) {
	q := NewFeedbackQueue()
	q.Submit(&FormattedReview{Formatted: "changes", Action: "request_changes", Round: 49}, false)
	if dropped := q.SupersedeBefore(49); len(dropped) != 0 {
		t.Fatalf("dropped = %+v, want nothing — this verdict is about the round being staged", dropped)
	}
	got := q.PollWithInfo("")
	if got == nil || len(got.Reviews) != 1 {
		t.Fatal("the current round's verdict must still be deliverable")
	}
}

// A verdict with no round predates the stamping, so its age is unknown.
// Dropping is the destructive answer; keep it and let provenance say nothing.
func TestAnUnstampedVerdictIsNeverDropped(t *testing.T) {
	q := NewFeedbackQueue()
	q.Submit(&FormattedReview{Formatted: "old", Action: "approve"}, false)
	if dropped := q.SupersedeBefore(50); len(dropped) != 0 {
		t.Fatalf("dropped = %+v, want nothing for an unstamped verdict", dropped)
	}
}

// An in-flight batch is a verdict handed out but not acknowledged. It must be
// retired too, or a lease expiry hands the stale verdict back after the fact.
func TestSupersedeAlsoRetiresAnInFlightBatch(t *testing.T) {
	q := NewFeedbackQueue()
	q.Submit(&FormattedReview{Formatted: "approved", Action: "approve", Round: 10}, false)
	if r := q.PollWithInfo("delivery-1"); r == nil || len(r.Reviews) != 1 {
		t.Fatal("expected the verdict to be handed out")
	}
	if dropped := q.SupersedeBefore(11); len(dropped) != 1 {
		t.Fatalf("dropped = %+v, want the in-flight verdict retired", dropped)
	}
	// A reclaim is what a lease expiry or a retrying client triggers; the
	// retired batch must not reappear through it.
	q.ReclaimInFlight("delivery-1")
	if r := q.PollWithInfo(""); r != nil && len(r.Reviews) > 0 {
		t.Fatalf("a retired in-flight verdict came back: %+v", r.Reviews)
	}
}

// Provenance is what makes a delivered verdict checkable. Without it an agent
// cannot tell which review an "Approved" is about.
func TestDeliveredFeedbackNamesItsRound(t *testing.T) {
	when := time.Date(2026, 9, 24, 15, 43, 49, 0, time.UTC)
	r := &PollResult{Reviews: []*FormattedReview{
		{Formatted: "## Review — Approved", Action: "approve", Round: 48, SubmittedAt: when},
	}}
	text, _, _ := r.CombinedFeedback()
	if !strings.Contains(text, "review round 48") {
		t.Errorf("feedback does not name its round:\n%s", text)
	}
	if !strings.Contains(text, "2026-09-24 15:43:49") {
		t.Errorf("feedback does not name when it was submitted:\n%s", text)
	}
	if !strings.HasSuffix(strings.TrimSpace(text), "Approved") {
		t.Errorf("the verdict itself must still be there:\n%s", text)
	}
}

// End to end through the engine: approve, stage a new round, then collect.
func TestEngineRetiresAndReportsAStaleVerdict(t *testing.T) {
	e, _ := summaryEngine(t)
	if err := e.Submit(types.ActionApprove, ""); err != nil {
		t.Fatalf("submit: %v", err)
	}
	e.mu.Lock()
	e.current.ReviewRound++
	e.current.SentAt = time.Time{}
	e.mu.Unlock()

	e.noteReviewSent() // what any first send of a new round does

	resp := e.handlePollFeedback(&protocol.PollFeedbackMsg{Type: protocol.TypePollFeedback}, nil)
	if resp.HasFeedback {
		t.Fatalf("a verdict from the previous round was delivered as this round's: %q", resp.Feedback)
	}
	if len(resp.Superseded) != 1 || !strings.Contains(resp.Superseded[0], "superseded") {
		t.Fatalf("superseded = %v, want the retirement reported rather than silent", resp.Superseded)
	}

	// Reported exactly once — a note that repeats is noise the agent learns to skip.
	again := e.handlePollFeedback(&protocol.PollFeedbackMsg{Type: protocol.TypePollFeedback}, nil)
	if len(again.Superseded) != 0 {
		t.Errorf("superseded repeated: %v", again.Superseded)
	}
}

// The phantom verdict's origin: approving a review with nothing in it. It reads
// as harmless — nobody is looking at anything — but it queues a verdict that a
// later, unrelated round collects as its own approval.
func TestSubmitRefusesAnEmptyReview(t *testing.T) {
	e, _ := summaryEngine(t)
	e.mu.Lock()
	e.current.ChangedFiles = nil
	e.current.ContentItems = nil
	e.current.AdditionalFiles = nil
	e.current.Comments = nil
	e.mu.Unlock()

	err := e.Submit(types.ActionApprove, "")
	if err == nil {
		t.Fatal("approving an empty review must be refused")
	}
	if !strings.Contains(err.Error(), "nothing staged") {
		t.Errorf("error = %q, want it to say why", err)
	}
	if e.feedback.QueuedCount() != 0 {
		t.Error("a refused submission must queue nothing")
	}
}

// An approval still needs no comments — that was always true and must stay so.
func TestApprovingAStagedReviewNeedsNoComments(t *testing.T) {
	e, _ := summaryEngine(t)
	e.mu.Lock()
	e.current.ChangedFiles = []types.ChangedFile{{Path: "hello.go"}}
	e.current.Comments = nil
	e.mu.Unlock()

	if err := e.Submit(types.ActionApprove, ""); err != nil {
		t.Fatalf("approve with no comments must be allowed: %v", err)
	}
}

// A restart must not launder a stale verdict into an unattributable one: an
// unstamped verdict prints no round and is deliberately never retired, so
// dropping the stamps on reload would undo both halves of the fix.
func TestReloadKeepsAVerdictsProvenance(t *testing.T) {
	e, _ := summaryEngine(t)
	e.mu.Lock()
	e.current.ChangedFiles = []types.ChangedFile{{Path: "hello.go"}}
	e.current.ReviewRound = 7
	e.mu.Unlock()
	if err := e.Submit(types.ActionApprove, ""); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Drop the in-memory queue the way a restart does, then reload from the DB.
	e.feedback = NewFeedbackQueue()
	e.ReloadPendingFeedback()

	r := e.feedback.PollWithInfo("")
	if r == nil || len(r.Reviews) != 1 {
		t.Fatal("expected the verdict to be reloaded")
	}
	if got := r.Reviews[0].Round; got != 7 {
		t.Errorf("round = %d after reload, want 7", got)
	}
	if r.Reviews[0].SubmittedAt.IsZero() {
		t.Error("submitted-at was lost across the reload")
	}
	text, _, _ := r.CombinedFeedback()
	if !strings.Contains(text, "review round 7") {
		t.Errorf("reloaded verdict does not name its round:\n%s", text)
	}
}
