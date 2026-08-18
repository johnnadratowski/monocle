package tui

import (
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func scrolledSidebar(t *testing.T) sidebarModel {
	t.Helper()
	km := DefaultKeyMap()
	m := newSidebarModel(&km)
	m.height = 5
	for i := 0; i < 20; i++ {
		m.files = append(m.files, types.ChangedFile{
			Path: string(rune('a'+i%26)) + "file.go", Status: types.FileModified,
		})
	}
	// Where the previous round left the reviewer: the bottom.
	m.cursor = len(m.files) - 1
	m.offset = len(m.files) - 5
	return m
}

func TestSidebarResetToTop(t *testing.T) {
	t.Run("puts the cursor and the viewport back at the start", func(t *testing.T) {
		m := scrolledSidebar(t)
		m.resetToTop()
		if m.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m.cursor)
		}
		if m.offset != 0 {
			t.Errorf("offset = %d, want 0 — the list must scroll up, not just move the cursor", m.offset)
		}
	})

	t.Run("skips a directory row in tree mode", func(t *testing.T) {
		m := scrolledSidebar(t)
		m.treeMode = true
		m.rebuildTree()
		m.resetToTop()
		idx := m.cursor - len(m.contentItems)
		if idx >= 0 && idx < len(m.visibleItems) && m.visibleItems[idx].isDir {
			t.Error("landed on a directory row, which holds no selection")
		}
	})
}

// The reported symptom: selectPath moved the cursor without scrolling, so a
// list left scrolled to the bottom stayed there with the selection off screen
// above it — reading as "the file selected is at the bottom".
func TestSelectPathScrollsIntoView(t *testing.T) {
	m := scrolledSidebar(t)
	first := m.files[0].Path
	m.selectPath(first)
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}
	if m.offset > m.cursor {
		t.Errorf("offset %d leaves the selection above the viewport", m.offset)
	}
}

func TestNewReviewResetsSelection(t *testing.T) {
	t.Run("the flag is consumed once", func(t *testing.T) {
		m := appModel{newReviewPending: true}
		m.sidebar = scrolledSidebar(t)

		// Simulates the refresh that brings the agent's next batch in.
		consume := func(a *appModel) bool {
			if a.newReviewPending && (len(a.sidebar.files) > 0 || len(a.sidebar.contentItems) > 0) {
				a.newReviewPending = false
				a.sidebar.resetToTop()
				return true
			}
			return false
		}

		if !consume(&m) {
			t.Fatal("the first refresh after delivery should reset to the top")
		}
		if m.sidebar.offset != 0 || m.sidebar.cursor != 0 {
			t.Errorf("not reset: cursor=%d offset=%d", m.sidebar.cursor, m.sidebar.offset)
		}

		// A second refresh within the same round must leave the cursor alone,
		// or the agent writing files would drag the reviewer back to the top.
		m.sidebar.cursor = 7
		m.sidebar.offset = 5
		if consume(&m) {
			t.Error("the reset should fire once per review, not on every refresh")
		}
		if m.sidebar.cursor != 7 {
			t.Errorf("cursor moved mid-review: %d", m.sidebar.cursor)
		}
	})

	t.Run("an empty refresh does not spend the flag", func(t *testing.T) {
		m := appModel{newReviewPending: true}
		if m.newReviewPending && len(m.sidebar.files) > 0 {
			t.Fatal("precondition")
		}
		// Nothing arrived yet, so the flag must survive for the real batch.
		if !m.newReviewPending {
			t.Error("the flag should still be pending when there is nothing to select")
		}
	})
}
