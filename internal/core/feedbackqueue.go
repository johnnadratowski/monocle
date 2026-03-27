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

// Poll returns pending feedback without blocking. Returns nil if none available.
func (fq *FeedbackQueue) Poll() *FormattedReview {
	result := fq.PollWithInfo()
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
func (fq *FeedbackQueue) PollWithInfo() *PollResult {
	fq.mu.Lock()
	defer fq.mu.Unlock()

	if len(fq.pending) == 0 {
		return nil
	}

	result := &PollResult{
		Reviews:          fq.pending,
		ChannelDelivered: fq.channelDelivered,
	}
	fq.pending = nil
	fq.status = "delivered"
	return result
}

// WaitForFeedback blocks until the user submits feedback. Used for the "pause" flow
// where the agent explicitly waits for review.
func (fq *FeedbackQueue) WaitForFeedback() *FormattedReview {
	result := fq.WaitForFeedbackWithInfo()
	if len(result.Reviews) == 1 {
		return result.Reviews[0]
	}
	text, count, action := result.CombinedFeedback()
	return &FormattedReview{Formatted: text, CommentCount: count, Action: action}
}

// WaitForFeedbackWithInfo blocks until feedback is available, then returns
// all pending reviews with delivery metadata.
func (fq *FeedbackQueue) WaitForFeedbackWithInfo() *PollResult {
	fq.mu.Lock()
	defer fq.mu.Unlock()

	// If there's already pending feedback, return it immediately
	if len(fq.pending) > 0 {
		result := &PollResult{
			Reviews:          fq.pending,
			ChannelDelivered: fq.channelDelivered,
		}
		fq.pending = nil
		fq.status = "delivered"
		fq.pauseRequested = false
		return result
	}

	// Block until feedback is submitted
	for len(fq.pending) == 0 {
		fq.cond.Wait()
	}

	result := &PollResult{
		Reviews:          fq.pending,
		ChannelDelivered: fq.channelDelivered,
	}
	fq.pending = nil
	fq.status = "delivered"
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
}

// HasPending returns true if there are queued reviews waiting for delivery.
func (fq *FeedbackQueue) HasPending() bool {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	return len(fq.pending) > 0
}

// QueuedCount returns the number of reviews waiting in the queue.
func (fq *FeedbackQueue) QueuedCount() int {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	return len(fq.pending)
}
