package tui

import (
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestSubmitDefaultsToQuestions(t *testing.T) {
	open := func(s *types.ReviewSummary) reviewSummaryModel {
		var m reviewSummaryModel
		m.open(s, true)
		return m
	}

	t.Run("questions alone default the verdict to questions", func(t *testing.T) {
		m := open(&types.ReviewSummary{QuestionCt: 2})
		if m.action != types.ActionQuestions {
			t.Errorf("action = %v, want questions", m.action)
		}
	})

	// Anything asking for an edit outranks a question: the agent has to change
	// code either way, so the stronger verdict is the honest one.
	t.Run("an issue alongside questions still requests changes", func(t *testing.T) {
		m := open(&types.ReviewSummary{QuestionCt: 2, IssueCt: 1})
		if m.action != types.ActionRequestChanges {
			t.Errorf("action = %v, want request_changes", m.action)
		}
	})

	t.Run("notes and praise alone still approve", func(t *testing.T) {
		m := open(&types.ReviewSummary{NoteCt: 3, PraiseCt: 1})
		if m.action != types.ActionApprove {
			t.Errorf("action = %v, want approve", m.action)
		}
	})

	t.Run("a questions verdict with only questions can submit", func(t *testing.T) {
		m := open(&types.ReviewSummary{QuestionCt: 1})
		if !m.canSubmit() {
			t.Error("questions backed by a question comment should submit")
		}
	})

	t.Run("a questions verdict with nothing to ask cannot submit", func(t *testing.T) {
		m := open(&types.ReviewSummary{})
		m.action = types.ActionQuestions
		if m.canSubmit() {
			t.Error("blocking the agent with no question is never the intent")
		}
	})
}
