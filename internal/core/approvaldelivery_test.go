package core

import (
	"testing"
)

func approved() *FormattedReview {
	return &FormattedReview{Formatted: "## Review — Approved", Action: "approve"}
}

// TestInFlightIsNotDelivered pins what the status means while a verdict is out
// on loan. A two-phase delivery is recoverable until it is acked, so reporting
// it as delivered told review_status to answer "no feedback pending" about a
// verdict that was still owed to someone.
func TestInFlightIsNotDelivered(t *testing.T) {
	fq := NewFeedbackQueue()
	fq.Submit(approved(), false)
	if got := fq.GetStatus(); got != "queued" {
		t.Fatalf("status after submit = %q, want queued", got)
	}

	result := fq.PollWithInfo("delivery-1")
	if result == nil || len(result.Reviews) != 1 {
		t.Fatalf("poll returned %+v, want the submitted review", result)
	}
	if got := fq.GetStatus(); got != "queued" {
		t.Errorf("status while in flight = %q; it is not delivered until acked", got)
	}

	if !fq.AckInFlight("delivery-1") {
		t.Fatal("ack should commit the in-flight delivery")
	}
	if got := fq.GetStatus(); got != "delivered" {
		t.Errorf("status after ack = %q, want delivered", got)
	}
}

// One-phase delivery has no recovery step, so it commits as it hands over.
func TestOnePhaseDeliveryCommitsImmediately(t *testing.T) {
	fq := NewFeedbackQueue()
	fq.Submit(approved(), false)
	if fq.PollWithInfo("") == nil {
		t.Fatal("expected the review")
	}
	if got := fq.GetStatus(); got != "delivered" {
		t.Errorf("status = %q, want delivered", got)
	}
}

// TestUnackedApprovalIsRecoverable is the shape of the reported bug: the Stop
// hook drains the queue at the end of every turn, and on an approval it emits
// nothing. If it does not ack, the verdict has to still be there for the agent's
// own get_feedback to find.
func TestUnackedApprovalIsRecoverable(t *testing.T) {
	fq := NewFeedbackQueue()
	fq.Submit(approved(), false)

	// The hook's drain.
	if r := fq.PollWithInfo("hook-delivery"); r == nil || len(r.Reviews) != 1 {
		t.Fatalf("hook poll returned %+v, want the approval", r)
	}
	// It emits nothing and does not ack.

	// The agent asks next.
	again := fq.PollWithInfo("agent-delivery")
	if again == nil || len(again.Reviews) != 1 {
		t.Fatalf("agent poll returned %+v, want the approval back", again)
	}
	if again.Reviews[0].Action != "approve" {
		t.Errorf("action = %q, want approve", again.Reviews[0].Action)
	}
}

// And the converse: a hook that DOES ack has committed it, so the verdict is
// gone. This is what made an approval vanish — the fix is at the call site, and
// the queue behaviour it relies on is pinned here.
func TestAckedDeliveryIsGone(t *testing.T) {
	fq := NewFeedbackQueue()
	fq.Submit(approved(), false)
	r := fq.PollWithInfo("hook-delivery")
	if r == nil {
		t.Fatal("expected the approval")
	}
	fq.AckInFlight("hook-delivery")

	if again := fq.PollWithInfo("agent-delivery"); again != nil {
		t.Errorf("poll after ack returned %+v, want nothing left", again)
	}
}

// The lease is the backstop for a client that dies instead of acking.
func TestLeaseReturnsAnUnackedDelivery(t *testing.T) {
	fq := NewFeedbackQueue()
	fq.Submit(approved(), false)
	r := fq.PollWithInfo("dead-client")
	if r == nil {
		t.Fatal("expected the approval")
	}
	if !fq.ReclaimInFlight("dead-client") {
		t.Fatal("the lease should be able to reclaim an unacked delivery")
	}
	if got := fq.GetStatus(); got != "queued" {
		t.Errorf("status after reclaim = %q, want queued", got)
	}
	if again := fq.PollWithInfo(""); again == nil || len(again.Reviews) != 1 {
		t.Errorf("reclaimed poll = %+v, want the approval back", again)
	}
}
