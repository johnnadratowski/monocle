package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/types"
)

// TestReviewTitleShowsWhenItWasSent covers the age beside the review name: coming
// back to the terminal, it is what separates the round you already read from one
// that just landed.
func TestReviewTitleShowsWhenItWasSent(t *testing.T) {
	bar := func(m appModel) string { return stripANSISeq(m.renderTitleBar()) }

	t.Run("an hours-old review says so", func(t *testing.T) {
		m := appModel{width: 160, reviewName: "Auth rework", reviewSentAt: time.Now().Add(-3 * time.Hour)}
		out := bar(m)
		if !strings.Contains(out, "Auth rework") {
			t.Fatalf("missing the review name: %q", out)
		}
		if !strings.Contains(out, "sent 3h ago") {
			t.Errorf("want a sent age in the bar, got: %q", out)
		}
	})

	t.Run("a fresh review reads as just now", func(t *testing.T) {
		m := appModel{width: 160, reviewName: "Auth rework", reviewSentAt: time.Now().Add(-2 * time.Second)}
		if out := bar(m); !strings.Contains(out, "sent just now") {
			t.Errorf("want 'sent just now', got: %q", out)
		}
	})

	// Zero means the agent has collected the feedback and sent nothing since.
	// Showing the previous round's age there would be a lie.
	t.Run("nothing sent shows no age", func(t *testing.T) {
		m := appModel{width: 160, reviewName: "Auth rework"}
		out := bar(m)
		if !strings.Contains(out, "Auth rework") {
			t.Fatalf("the name should still show: %q", out)
		}
		if strings.Contains(out, "sent") {
			t.Errorf("want no age with a zero stamp, got: %q", out)
		}
	})

	t.Run("an untitled review ages by its artifact", func(t *testing.T) {
		m := appModel{width: 160}
		m.sidebar.contentItems = []types.ContentItem{
			{ID: "p1", Title: "Refactor plan", UpdatedAt: time.Now().Add(-45 * time.Minute)},
		}
		if out := bar(m); !strings.Contains(out, "sent 45m ago") {
			t.Errorf("want the artifact's own age, got: %q", out)
		}
	})

	// The name is the part you cannot re-derive from anywhere else on screen, so
	// it outranks the age when the bar runs out of room.
	t.Run("a narrow bar sheds the age, not the name", func(t *testing.T) {
		const name = "Auth rework"
		wide := appModel{width: 160, reviewName: name, reviewSentAt: time.Now().Add(-3 * time.Hour)}
		if !strings.Contains(bar(wide), "sent 3h ago") {
			t.Fatal("precondition: the wide bar should carry the age")
		}
		narrow := wide
		narrow.width = len(" o_(◉) monocle dev") + len(name) + 6
		out := bar(narrow)
		if !strings.Contains(out, name) {
			t.Errorf("the name should survive a narrow bar, got: %q", out)
		}
		if strings.Contains(out, "sent") {
			t.Errorf("the age should be shed first, got: %q", out)
		}
	})
}
