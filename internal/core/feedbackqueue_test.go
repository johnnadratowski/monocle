package core

import (
	"testing"
	"time"
)

func TestPollNoFeedback(t *testing.T) {
	fq := NewFeedbackQueue()

	review := fq.Poll()
	if review != nil {
		t.Error("expected nil on empty queue")
	}
}

func TestSubmitThenPoll(t *testing.T) {
	fq := NewFeedbackQueue()

	fq.Submit(&FormattedReview{
		Formatted:    "## Review\nFix bug",
		CommentCount: 1,
		Action:       "request_changes",
	}, false)

	if fq.GetStatus() != "queued" {
		t.Errorf("expected status queued, got %q", fq.GetStatus())
	}

	review := fq.Poll()
	if review == nil {
		t.Fatal("expected review from Poll")
	}
	if review.Formatted != "## Review\nFix bug" {
		t.Errorf("unexpected review: %q", review.Formatted)
	}
	if fq.GetStatus() != "delivered" {
		t.Errorf("expected status delivered, got %q", fq.GetStatus())
	}

	// Second poll should return nil
	if fq.Poll() != nil {
		t.Error("expected nil after delivery")
	}
}

func TestDiscardPending(t *testing.T) {
	fq := NewFeedbackQueue()

	// No pending → discards 0.
	if n := fq.DiscardPending(); n != 0 {
		t.Errorf("expected 0 discarded, got %d", n)
	}

	// Two accidental submissions accumulate in queue mode.
	fq.Submit(&FormattedReview{Formatted: "one", Action: "approve"}, false)
	fq.Submit(&FormattedReview{Formatted: "two", Action: "approve"}, false)
	if fq.QueuedCount() != 2 {
		t.Fatalf("expected 2 queued, got %d", fq.QueuedCount())
	}

	// Discard cancels both without delivering.
	if n := fq.DiscardPending(); n != 2 {
		t.Errorf("expected 2 discarded, got %d", n)
	}
	if fq.QueuedCount() != 0 {
		t.Errorf("expected empty queue, got %d", fq.QueuedCount())
	}
	if fq.GetStatus() != "none" {
		t.Errorf("expected status none, got %q", fq.GetStatus())
	}
	if fq.Poll() != nil {
		t.Error("expected nil after discard — feedback must not be delivered")
	}
}

func TestWaitForFeedback(t *testing.T) {
	fq := NewFeedbackQueue()

	var review *FormattedReview
	done := make(chan struct{})

	go func() {
		review = fq.WaitForFeedback()
		close(done)
	}()

	// Give goroutine time to block
	time.Sleep(50 * time.Millisecond)

	// Submit feedback
	fq.Submit(&FormattedReview{
		Formatted:    "## Review\nFix bug",
		CommentCount: 1,
		Action:       "request_changes",
	}, false)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForFeedback did not return")
	}

	if review == nil {
		t.Fatal("expected review")
	}
	if review.Formatted != "## Review\nFix bug" {
		t.Errorf("unexpected review: %q", review.Formatted)
	}
}

func TestWaitForFeedbackWithPending(t *testing.T) {
	fq := NewFeedbackQueue()

	// Submit before waiting
	fq.Submit(&FormattedReview{
		Formatted:    "## Review\nLooks good",
		CommentCount: 1,
		Action:       "approve",
	}, false)

	// WaitForFeedback should return immediately
	review := fq.WaitForFeedback()
	if review == nil {
		t.Fatal("expected review")
	}
	if review.Formatted != "## Review\nLooks good" {
		t.Errorf("unexpected review: %q", review.Formatted)
	}
}

// TestWaitForFeedbackCancellable_PreservesFeedback reproduces the
// disconnect-mid-wait data-loss bug: a wait that is cancelled (its client
// socket died) must NOT drain the queue, so feedback submitted afterwards
// still reaches the next poller instead of being silently consumed and
// marked delivered to a dead connection.
func TestWaitForFeedbackCancellable_PreservesFeedback(t *testing.T) {
	fq := NewFeedbackQueue()

	cancel := make(chan struct{})
	got := make(chan *PollResult, 1)
	go func() {
		got <- fq.WaitForFeedbackCancellable(cancel, "")
	}()

	// Let the waiter park, then simulate the client disconnecting.
	time.Sleep(50 * time.Millisecond)
	close(cancel)

	select {
	case res := <-got:
		if res != nil {
			t.Fatalf("cancelled wait should return nil, got %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled wait did not return")
	}

	// Feedback submitted after the abandoned wait must survive.
	fq.Submit(&FormattedReview{Formatted: "## Review\nFix bug", CommentCount: 1, Action: "request_changes"}, false)
	if review := fq.Poll(); review == nil {
		t.Fatal("feedback was lost — cancelled wait drained the queue")
	} else if review.Formatted != "## Review\nFix bug" {
		t.Errorf("unexpected review: %q", review.Formatted)
	}
}

// TestWaitForFeedbackCancellable_CancelAfterPending verifies that even when
// feedback is already queued, a fired cancel returns nil without consuming
// it — the disconnect wins so the review is kept for the next poller.
func TestWaitForFeedbackCancellable_CancelAfterPending(t *testing.T) {
	fq := NewFeedbackQueue()
	fq.Submit(&FormattedReview{Formatted: "pending", CommentCount: 0, Action: "approve"}, false)

	cancel := make(chan struct{})
	close(cancel)

	if res := fq.WaitForFeedbackCancellable(cancel, ""); res != nil {
		t.Fatalf("pre-cancelled wait should return nil, got %+v", res)
	}
	if !fq.HasPending() {
		t.Fatal("pre-cancelled wait consumed already-pending feedback")
	}
}

func TestPauseRequested(t *testing.T) {
	fq := NewFeedbackQueue()

	if fq.IsPauseRequested() {
		t.Error("expected pause not requested initially")
	}

	fq.SetPauseRequested(true)

	if !fq.IsPauseRequested() {
		t.Error("expected pause requested after set")
	}

	// Submit should clear pause
	fq.Submit(&FormattedReview{
		Formatted:    "review",
		CommentCount: 1,
		Action:       "request_changes",
	}, false)

	if fq.IsPauseRequested() {
		t.Error("expected pause cleared after Submit")
	}
}

func TestHasPending(t *testing.T) {
	fq := NewFeedbackQueue()

	if fq.HasPending() {
		t.Error("expected HasPending=false on new queue")
	}

	fq.Submit(&FormattedReview{
		Formatted:    "review",
		CommentCount: 1,
		Action:       "request_changes",
	}, false)

	if !fq.HasPending() {
		t.Error("expected HasPending=true after Submit")
	}

	fq.Poll()

	if fq.HasPending() {
		t.Error("expected HasPending=false after Poll")
	}
}

// --- Two-phase delivery ---

func submitOne(fq *FeedbackQueue, text string) {
	fq.Submit(&FormattedReview{Formatted: text, CommentCount: 1, Action: "request_changes"}, false)
}

// TestTwoPhase_UnackedIsRedelivered is the core guarantee: a verdict handed to
// an ack-capable client that never confirms must remain recoverable, so the
// next poll delivers it again instead of it being silently dropped.
func TestTwoPhase_UnackedIsRedelivered(t *testing.T) {
	fq := NewFeedbackQueue()
	submitOne(fq, "verdict")

	first := fq.PollWithInfo("delivery-1")
	if first == nil || len(first.Reviews) != 1 {
		t.Fatal("expected the verdict on first poll")
	}
	if first.DeliveryID != "delivery-1" {
		t.Errorf("DeliveryID = %q, want delivery-1", first.DeliveryID)
	}
	// Un-acked delivery is still outstanding.
	if !fq.HasPending() {
		t.Error("expected HasPending=true while a delivery is unacknowledged")
	}

	second := fq.PollWithInfo("delivery-2")
	if second == nil || len(second.Reviews) != 1 {
		t.Fatal("verdict was lost: unacknowledged delivery was not redelivered")
	}
	if second.Reviews[0].Formatted != "verdict" {
		t.Errorf("redelivered %q, want verdict", second.Reviews[0].Formatted)
	}
}

func TestTwoPhase_AckCommits(t *testing.T) {
	fq := NewFeedbackQueue()
	submitOne(fq, "verdict")

	res := fq.PollWithInfo("delivery-1")
	if res == nil {
		t.Fatal("expected a verdict")
	}
	if !fq.AckInFlight("delivery-1") {
		t.Fatal("expected ack to match the in-flight delivery")
	}
	if fq.HasPending() {
		t.Error("expected nothing pending after ack")
	}
	if fq.PollWithInfo("delivery-2") != nil {
		t.Error("acked verdict must not be redelivered")
	}
}

func TestTwoPhase_AckIsIdempotentAndIgnoresUnknownIDs(t *testing.T) {
	fq := NewFeedbackQueue()
	submitOne(fq, "verdict")
	fq.PollWithInfo("delivery-1")

	if fq.AckInFlight("bogus") {
		t.Error("unknown delivery id must not commit")
	}
	if !fq.AckInFlight("delivery-1") {
		t.Fatal("expected first ack to succeed")
	}
	if fq.AckInFlight("delivery-1") {
		t.Error("duplicate ack must be a no-op, not a second commit")
	}
}

// TestOnePhase_BackCompat: a client that does not opt in keeps the historical
// commit-on-send behaviour, so an older binary never loops on redelivery.
func TestOnePhase_BackCompat(t *testing.T) {
	fq := NewFeedbackQueue()
	submitOne(fq, "verdict")

	res := fq.PollWithInfo("")
	if res == nil || res.DeliveryID != "" {
		t.Fatal("expected a one-phase delivery with no DeliveryID")
	}
	if fq.HasPending() {
		t.Error("one-phase delivery should leave nothing outstanding")
	}
	if fq.PollWithInfo("") != nil {
		t.Error("one-phase delivery must not be redelivered")
	}
}

func TestReclaimInFlight_ReturnsVerdictToQueue(t *testing.T) {
	fq := NewFeedbackQueue()
	submitOne(fq, "verdict")
	fq.PollWithInfo("delivery-1")

	if fq.ReclaimInFlight("bogus") {
		t.Error("reclaim with a stale id must not fire")
	}
	if !fq.ReclaimInFlight("delivery-1") {
		t.Fatal("expected lease expiry to reclaim the delivery")
	}
	if fq.GetStatus() != "queued" {
		t.Errorf("status = %q, want queued after reclaim", fq.GetStatus())
	}
	res := fq.PollWithInfo("delivery-2")
	if res == nil || res.Reviews[0].Formatted != "verdict" {
		t.Fatal("reclaimed verdict should be deliverable again")
	}
}

// DiscardPending (the reviewer's :cancel-feedback) must also drop an
// unacknowledged delivery, or a lease expiry could resurrect cancelled feedback.
func TestDiscardPending_DropsInFlight(t *testing.T) {
	fq := NewFeedbackQueue()
	submitOne(fq, "verdict")
	fq.PollWithInfo("delivery-1")

	if n := fq.DiscardPending(); n != 1 {
		t.Errorf("discarded %d, want 1 (the in-flight delivery)", n)
	}
	if fq.ReclaimInFlight("delivery-1") {
		t.Error("cancelled feedback must not be resurrected by lease expiry")
	}
	if fq.PollWithInfo("delivery-2") != nil {
		t.Error("expected nothing after discard")
	}
}
