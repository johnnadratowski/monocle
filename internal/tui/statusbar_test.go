package tui

import (
	"strings"
	"testing"
)

func TestStatusBarReviewProgressAndAnnotations(t *testing.T) {
	m := newStatusBarModel(DefaultTheme())
	m.width = 200
	m.subscriberCount = 1 // connected
	m.fileCount = 5
	m.reviewedCount = 2
	m.commentCount = 3
	m.annotationCount = 4
	m.baseRef = "main"

	out := m.View()

	for _, want := range []string{"2/5 reviewed", "3 comments", "4 annotations"} {
		if !strings.Contains(out, want) {
			t.Errorf("status bar should contain %q, got: %q", want, out)
		}
	}
	// The base ref and raw file count moved to the top bar — they must not appear here.
	if strings.Contains(out, "ref:") {
		t.Error("status bar should no longer show the base ref")
	}
	if strings.Contains(out, "5 files") {
		t.Error("status bar should no longer show the raw file count")
	}
}
