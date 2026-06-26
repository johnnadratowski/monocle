package tui

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestStatusBarReviewMetrics(t *testing.T) {
	m := newStatusBarModel(DefaultTheme())
	m.width = 240
	m.subscriberCount = 1 // connected
	m.fileCount = 5
	m.reviewedCount = 2
	m.setCommentStats([]types.ReviewComment{
		{Type: types.CommentIssue, Resolved: true},
		{Type: types.CommentIssue},
		{Type: types.CommentSuggestion},
		{Type: types.CommentNote},
	})
	m.agentActive = true
	m.feedbackStatus = "request_changes"
	m.baseRef = "main"

	out := m.View()

	for _, want := range []string{
		"2/5 reviewed",
		"2 issues",
		"1 suggestion",
		"1 note",
		"(1/4 resolved)",
		"✎ agent working",
		"⌛ feedback pending",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status bar should contain %q\n got: %q", want, out)
		}
	}
	// ref/files moved to the top bar.
	if strings.Contains(out, "ref:") || strings.Contains(out, "5 files") {
		t.Error("status bar should no longer show ref or raw file count")
	}
}

func TestStatusBarNoCommentsAndDelivered(t *testing.T) {
	m := newStatusBarModel(DefaultTheme())
	m.width = 240
	m.subscriberCount = 1
	m.fileCount = 3
	m.setCommentStats(nil)
	m.feedbackStatus = "delivered" // terminal, not pending

	out := m.View()
	if !strings.Contains(out, "0 comments") {
		t.Errorf("expected '0 comments', got: %q", out)
	}
	if strings.Contains(out, "feedback pending") {
		t.Error("'delivered' is terminal and should not show a pending indicator")
	}
}
