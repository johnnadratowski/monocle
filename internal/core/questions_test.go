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

func TestQuestionCommentType(t *testing.T) {
	now := time.Now()
	mixed := []types.ReviewComment{
		{ID: "q", TargetType: types.TargetFile, TargetRef: "a.go", LineStart: 1, LineEnd: 1,
			Type: types.CommentQuestion, Body: "why a mutex here?", CreatedAt: now, UpdatedAt: now},
		{ID: "p", TargetType: types.TargetFile, TargetRef: "a.go", LineStart: 2, LineEnd: 2,
			Type: types.CommentPraise, Body: "nice", CreatedAt: now, UpdatedAt: now},
	}

	t.Run("questions are counted separately", func(t *testing.T) {
		ct := countByType(mixed)
		if ct[types.CommentQuestion] != 1 {
			t.Errorf("question count = %d, want 1", ct[types.CommentQuestion])
		}
		if ct[types.CommentPraise] != 1 {
			t.Errorf("praise count = %d, want 1", ct[types.CommentPraise])
		}
		for _, absent := range []types.CommentType{types.CommentIssue, types.CommentSuggestion, types.CommentNote} {
			if ct[absent] != 0 {
				t.Errorf("%s count = %d, want 0", absent, ct[absent])
			}
		}
	})

	t.Run("the summary asks for answers, not changes", func(t *testing.T) {
		got := NewReviewFormatter(nil, defaultFormatCfg()).
			Format(&types.ReviewSession{}, mixed, types.ActionQuestions, "").Formatted
		if !strings.Contains(got, "question(s) to answer") {
			t.Errorf("expected the question count in the summary:\n%s", got)
		}
		if strings.Contains(got, "address the issues") {
			t.Errorf("a review with no issues must not demand fixes:\n%s", got)
		}
	})

	// Notes and praise ask nothing of the agent; the other three do.
	t.Run("WantsResponse separates the actionable types", func(t *testing.T) {
		for _, ct := range []types.CommentType{types.CommentIssue, types.CommentSuggestion, types.CommentQuestion} {
			if !ct.WantsResponse() {
				t.Errorf("%s should want a response", ct)
			}
		}
		for _, ct := range []types.CommentType{types.CommentNote, types.CommentPraise} {
			if ct.WantsResponse() {
				t.Errorf("%s should not want a response", ct)
			}
		}
	})

	t.Run("the notification counts questions", func(t *testing.T) {
		got := buildFeedbackSummary(string(types.ActionQuestions), mixed)
		if !strings.Contains(got, "1 question") {
			t.Errorf("expected the question count, got %q", got)
		}
	})
}

func TestAnswerCommentType(t *testing.T) {
	now := time.Now()
	answers := []types.ReviewComment{
		{ID: "a1", TargetType: types.TargetFile, TargetRef: "a.go", LineStart: 1, LineEnd: 1,
			Type: types.CommentAnswer, Body: "yes, the mutex guards the map", CreatedAt: now, UpdatedAt: now},
	}

	// The point the reviewer made: a review that is only answers is a reviewer
	// discharging a request, not making one, so it goes back as an approval.
	t.Run("an answer asks nothing of the agent", func(t *testing.T) {
		if types.CommentAnswer.WantsResponse() {
			t.Error("an answer should not oblige the agent to come back")
		}
		if !types.CommentQuestion.WantsResponse() {
			t.Error("a question still should")
		}
	})

	t.Run("answers are counted separately", func(t *testing.T) {
		if got := countByType(answers)[types.CommentAnswer]; got != 1 {
			t.Errorf("answer count = %d, want 1", got)
		}
	})

	t.Run("an approval carrying answers still reads as approved", func(t *testing.T) {
		got := NewReviewFormatter(nil, defaultFormatCfg()).
			Format(&types.ReviewSession{}, answers, types.ActionApprove, "").Formatted
		if !strings.Contains(got, "answer(s)") {
			t.Errorf("expected the answer count in the summary:\n%s", got)
		}
		for _, forbidden := range []string{"Changes Requested", "address the issues", "answer the questions"} {
			if strings.Contains(got, forbidden) {
				t.Errorf("an approval with answers must not read as %q:\n%s", forbidden, got)
			}
		}
	})

	t.Run("the notification counts answers", func(t *testing.T) {
		got := buildFeedbackSummary(string(types.ActionApprove), answers)
		if !strings.Contains(got, "1 answer") {
			t.Errorf("expected the answer count, got %q", got)
		}
	})

	t.Run("praise is still left out of the notification", func(t *testing.T) {
		praise := []types.ReviewComment{{ID: "p", Type: types.CommentPraise, Body: "nice",
			TargetType: types.TargetFile, TargetRef: "a.go", CreatedAt: now, UpdatedAt: now}}
		if got := buildFeedbackSummary(string(types.ActionApprove), praise); strings.Contains(got, "praise") {
			t.Errorf("praise asks nothing and should not inflate the count: %q", got)
		}
	})
}
