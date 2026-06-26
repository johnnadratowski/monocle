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
	m.annotationCount = 4
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
		"4 annotations",
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

func TestStatusBarAnnotationPerFileCount(t *testing.T) {
	m := newStatusBarModel(DefaultTheme())
	m.width = 240
	m.subscriberCount = 1
	m.annotationCount = 7

	// No file selected (default): show the total only.
	if out := m.View(); !strings.Contains(out, "7 annotations") || strings.Contains(out, "/7 annotations") {
		t.Errorf("no-file case should show total only, got: %q", out)
	}

	// A file is selected with 2 of the 7 annotations: show x/n.
	m.annotationFileCount = 2
	if out := m.View(); !strings.Contains(out, "2/7 annotations") {
		t.Errorf("file case should show x/n, got: %q", out)
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
