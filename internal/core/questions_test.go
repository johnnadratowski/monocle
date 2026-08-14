package core

import (
	"strings"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/types"
)

func questionComments() []types.ReviewComment {
	now := time.Now()
	return []types.ReviewComment{{
		ID: "c1", TargetType: types.TargetFile, TargetRef: "a.go",
		LineStart: 3, LineEnd: 3, Type: types.CommentNote,
		Body: "why a mutex here rather than a channel?", CreatedAt: now, UpdatedAt: now,
	}}
}

func TestQuestionsAction(t *testing.T) {
	t.Run("the header names questions, not changes", func(t *testing.T) {
		got := NewReviewFormatter(nil, defaultFormatCfg()).Format(&types.ReviewSession{}, questionComments(), types.ActionQuestions, "").Formatted
		if !strings.Contains(got, "## Review — Questions") {
			t.Errorf("expected a questions header, got:\n%s", got)
		}
		if strings.Contains(got, "Changes Requested") {
			t.Errorf("a questions review must not read as a change request:\n%s", got)
		}
	})

	// The whole point of the action: the agent is told to answer, not to edit.
	t.Run("the review tells the agent to answer first", func(t *testing.T) {
		got := NewReviewFormatter(nil, defaultFormatCfg()).Format(&types.ReviewSession{}, questionComments(), types.ActionQuestions, "").Formatted
		if !strings.Contains(strings.ToLower(got), "answer them before continuing") {
			t.Errorf("expected explicit answer-first guidance, got:\n%s", got)
		}
	})

	t.Run("an empty questions review still says questions", func(t *testing.T) {
		got := NewReviewFormatter(nil, defaultFormatCfg()).Format(&types.ReviewSession{}, nil, types.ActionQuestions, "").Formatted
		if !strings.Contains(got, "Questions") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("questions keeps the review open", func(t *testing.T) {
		if types.ActionQuestions.ClosesReview() {
			t.Error("questions must not close the review — the agent still has to come back")
		}
		if types.ActionRequestChanges.ClosesReview() {
			t.Error("request_changes must not close the review")
		}
		if !types.ActionApprove.ClosesReview() {
			t.Error("approve should close the review")
		}
	})

	t.Run("the agent notification distinguishes it from a change request", func(t *testing.T) {
		got := strings.ToLower(buildFeedbackSummary(string(types.ActionQuestions), questionComments()))
		if !strings.Contains(got, "questions") {
			t.Errorf("expected the notification to say questions, got %q", got)
		}
		if strings.Contains(got, "requested changes") {
			t.Errorf("a questions verdict must not announce itself as a change request: %q", got)
		}
	})
}
