package core

import (
	"fmt"
	"strings"
	"sync"
)

// FormattedReview holds a formatted review ready for delivery.
type FormattedReview struct {
	Formatted    string
	CommentCount int
	Action       string
}

// PollResult holds the result of polling the feedback queue.
type PollResult struct {
	Reviews          []*FormattedReview
	ChannelDelivered bool
	// DeliveryID is non-empty when the caller opted into two-phase delivery.
	// The delivery is uncommitted until AckInFlight is called with this id.
	DeliveryID string
}

// CombinedFeedback returns the reviews combined into a single formatted string.
// If there's only one review, it returns it directly. Multiple reviews are
// joined with headers.
func (r *PollResult) CombinedFeedback() (string, int, string) {
	if len(r.Reviews) == 0 {
		return "", 0, ""
	}
	if len(r.Reviews) == 1 {
		rev := r.Reviews[0]
		return rev.Formatted, rev.CommentCount, rev.Action
	}

	var b strings.Builder
	totalComments := 0
	action := "approve"
	for i, rev := range r.Reviews {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(fmt.Sprintf("--- Review %d of %d ---\n\n", i+1, len(r.Reviews)))
		b.WriteString(rev.Formatted)
		totalComments += rev.CommentCount
		if rev.Action == "request_changes" {
			action = "request_changes"
		}
	}
	return b.String(), totalComments, action
}

// ReviewStatusInfo holds the current review status for MCP channel queries.
type ReviewStatusInfo struct {
	Status       string // "no_feedback" | "pending" | "pause_requested"
	CommentCount int
	Summary      string
	RepoRoot     string // repo the answering engine is bound to (empty if no session)
	ReviewName   string // agent-supplied review name, if set
}

// FeedbackQueue manages the synchronization between user review actions
// and MCP channel/tool feedback retrieval. Supports both non-blocking and
// blocking wait (pause flow) models, and both push (channel) and queue modes.
//
// In push mode (channelDelivered=true), pending is replaced on each submit.
// In queue mode (channelDelivered=false), reviews accumulate until polled.
type FeedbackQueue struct {
	mu   sync.Mutex
	cond *sync.Cond

	// pending holds reviews waiting to be delivered (slice for queue mode)
	pending []*FormattedReview

	// inFlight holds a batch handed to an ack-capable client but not yet
	// acknowledged. It is deliberately NOT discarded: until the client acks (or
	// the lease expires and it is reclaimed into pending), the verdict is still
	// recoverable, so a client that dies between the engine's write and its own
	// processing cannot silently lose it.
	inFlight   []*FormattedReview
	inFlightID string

	// channelDelivered is true when the latest submit was already delivered
	// via channel push (so handlePollFeedback should not advance the round)
	channelDelivered bool

	// pauseRequested is set when the user wants the agent to stop and wait
	pauseRequested bool

	// status tracks delivery state
	status string // "none" | "queued" | "delivered"
}

// NewFeedbackQueue creates a new FeedbackQueue.
func NewFeedbackQueue() *FeedbackQueue {
	fq := &FeedbackQueue{status: "none"}
	fq.cond = sync.NewCond(&fq.mu)
	return fq
}

// Submit stores a review for delivery. If a wait handler is blocking,
// it wakes it to deliver immediately.
//
// channelDelivered controls accumulation behavior:
//   - true (push mode): replaces any pending review (channel delivers immediately)
//   - false (queue mode): appends to the pending queue
func (fq *FeedbackQueue) Submit(review *FormattedReview, channelDelivered bool) {
	fq.mu.Lock()
	defer fq.mu.Unlock()

	fq.channelDelivered = channelDelivered
	if channelDelivered {
		// Push mode: replace pending (will be cleared by ClearStatus shortly)
		fq.pending = []*FormattedReview{review}
	} else {
		// Queue mode: accumulate reviews
		fq.pending = append(fq.pending, review)
	}
	fq.status = "queued"
	fq.pauseRequested = false
	fq.cond.Broadcast()
}

// DiscardPending clears any queued-but-undelivered feedback without delivering
// it, returning how many reviews were discarded. Used to cancel accidental
// submissions before the agent pulls them.
func (fq *FeedbackQueue) DiscardPending() int {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	// Cancel must also drop an unacknowledged in-flight batch, or the reviewer
	// could "cancel" feedback that a lease expiry would later hand back.
	n := len(fq.pending) + len(fq.inFlight)
	fq.pending = nil
	fq.inFlight = nil
	fq.inFlightID = ""
	fq.channelDelivered = false
	fq.status = "none"
	return n
}

// reclaimInFlightLocked returns an unacknowledged in-flight batch to the front
// of the queue. Called before any new delivery: a client asking again is proof
// the previous hand-off didn't stick, so the verdict is redelivered rather than
// stranded. Caller must hold fq.mu.
func (fq *FeedbackQueue) reclaimInFlightLocked() {
	if len(fq.inFlight) == 0 {
		return
	}
	fq.pending = append(fq.inFlight, fq.pending...)
	fq.inFlight = nil
	fq.inFlightID = ""
}

// takeLocked removes the queued batch for delivery. When deliveryID is
// non-empty the batch is held as in-flight (two-phase: recoverable until acked)
// instead of being dropped outright. Caller must hold fq.mu.
func (fq *FeedbackQueue) takeLocked(deliveryID string) *PollResult {
	result := &PollResult{
		Reviews:          fq.pending,
		ChannelDelivered: fq.channelDelivered,
		DeliveryID:       deliveryID,
	}
	if deliveryID != "" {
		fq.inFlight = fq.pending
		fq.inFlightID = deliveryID
	}
	fq.pending = nil
	fq.status = "delivered"
	return result
}

// AckInFlight commits the in-flight delivery identified by id, dropping the
// retained copy. Returns false for an unknown or already-reclaimed id, which is
// a harmless no-op — a late or duplicate ack never loses or double-commits.
func (fq *FeedbackQueue) AckInFlight(id string) bool {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	if id == "" || fq.inFlightID != id {
		return false
	}
	fq.inFlight = nil
	fq.inFlightID = ""
	return true
}

// ReclaimInFlight returns the in-flight batch identified by id to the queue so
// it can be redelivered. Used when the delivery lease expires with no ack.
// Returns false if the id no longer matches (already acked or reclaimed).
func (fq *FeedbackQueue) ReclaimInFlight(id string) bool {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	if id == "" || fq.inFlightID != id {
		return false
	}
	fq.reclaimInFlightLocked()
	fq.status = "queued"
	fq.cond.Broadcast()
	return true
}

// InFlightID returns the id of the current uncommitted delivery, or "".
func (fq *FeedbackQueue) InFlightID() string {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	return fq.inFlightID
}

// Poll returns pending feedback without blocking. Returns nil if none available.
func (fq *FeedbackQueue) Poll() *FormattedReview {
	result := fq.PollWithInfo("")
	if result == nil {
		return nil
	}
	if len(result.Reviews) == 1 {
		return result.Reviews[0]
	}
	// Combine multiple reviews into one
	text, count, action := result.CombinedFeedback()
	return &FormattedReview{Formatted: text, CommentCount: count, Action: action}
}

// PollWithInfo returns all pending feedback with delivery metadata.
// Returns nil if no feedback is available.
//
// A non-empty deliveryID opts into two-phase delivery: the batch is retained as
// in-flight and stays recoverable until AckInFlight(deliveryID). An empty
// deliveryID keeps the historical commit-on-send behaviour.
func (fq *FeedbackQueue) PollWithInfo(deliveryID string) *PollResult {
	fq.mu.Lock()
	defer fq.mu.Unlock()

	// A fresh poll means any previous hand-off went unconfirmed — take it back
	// so it is redelivered here instead of being lost.
	fq.reclaimInFlightLocked()

	if len(fq.pending) == 0 {
		return nil
	}
	return fq.takeLocked(deliveryID)
}

// WaitForFeedback blocks until the user submits feedback. Used for the "pause" flow
// where the agent explicitly waits for review.
func (fq *FeedbackQueue) WaitForFeedback() *FormattedReview {
	result := fq.WaitForFeedbackWithInfo("")
	if len(result.Reviews) == 1 {
		return result.Reviews[0]
	}
	text, count, action := result.CombinedFeedback()
	return &FormattedReview{Formatted: text, CommentCount: count, Action: action}
}

// WaitForFeedbackWithInfo blocks until feedback is available, then returns
// all pending reviews with delivery metadata. Never returns nil.
func (fq *FeedbackQueue) WaitForFeedbackWithInfo(deliveryID string) *PollResult {
	return fq.WaitForFeedbackCancellable(nil, deliveryID)
}

// WaitForFeedbackCancellable blocks until feedback is available OR the cancel
// channel is closed (the waiting client disconnected). On cancel it returns
// nil WITHOUT consuming the queue, so the feedback survives for whoever polls
// next — otherwise an orphaned wait handler whose socket already died would
// drain the queue, mark the submission delivered, and silently lose the
// review. A nil cancel channel makes this a plain uninterruptible wait.
func (fq *FeedbackQueue) WaitForFeedbackCancellable(cancel <-chan struct{}, deliveryID string) *PollResult {
	fq.mu.Lock()
	defer fq.mu.Unlock()

	// An unconfirmed hand-off is effectively still queued: reclaim it so this
	// waiter delivers it rather than blocking behind it.
	fq.reclaimInFlightLocked()

	if cancel != nil {
		// sync.Cond can't select on a channel, so a helper wakes the cond
		// when cancel fires; the wait loop then re-checks cancel and bails.
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			select {
			case <-cancel:
				fq.mu.Lock()
				fq.cond.Broadcast()
				fq.mu.Unlock()
			case <-stop:
			}
		}()
	}

	for {
		// Check cancel before pending so a disconnect that races an arriving
		// submit never consumes the feedback — we'd rather preserve it for
		// the next poll than write it to a dead socket.
		if cancel != nil {
			select {
			case <-cancel:
				return nil
			default:
			}
		}
		if len(fq.pending) > 0 {
			break
		}
		fq.cond.Wait()
	}

	result := fq.takeLocked(deliveryID)
	fq.pauseRequested = false
	return result
}

// SetPauseRequested sets the pause flag. The next review_status call
// from Claude Code will see "pause_requested".
func (fq *FeedbackQueue) SetPauseRequested(paused bool) {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	fq.pauseRequested = paused
}

// IsPauseRequested returns whether the user has requested a pause.
func (fq *FeedbackQueue) IsPauseRequested() bool {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	return fq.pauseRequested
}

// GetStatus returns the current feedback status.
func (fq *FeedbackQueue) GetStatus() string {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	return fq.status
}

// ClearStatus resets the feedback status to "none" and clears any pending
// review. Called after submit when the review has already been delivered
// via push notification, so the queue doesn't hold stale feedback.
func (fq *FeedbackQueue) ClearStatus() {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	fq.status = "none"
	fq.pending = nil
	fq.inFlight = nil
	fq.inFlightID = ""
}

// HasPending returns true if there are reviews waiting for delivery. An
// unacknowledged in-flight batch still counts: delivery isn't committed until
// the client acks, so the verdict is genuinely still outstanding.
func (fq *FeedbackQueue) HasPending() bool {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	return len(fq.pending)+len(fq.inFlight) > 0
}

// QueuedCount returns the number of reviews waiting in the queue, including an
// unacknowledged in-flight batch.
func (fq *FeedbackQueue) QueuedCount() int {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	return len(fq.pending) + len(fq.inFlight)
}
