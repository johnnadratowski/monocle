package tui

import (
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func newTestDiffViewModel() diffViewModel {
	theme := DefaultTheme()
	keys := DefaultKeyMap()
	m := newDiffViewModel(&theme, &keys)
	m.width = 80
	m.height = 24
	m.focused = true
	return m
}

func TestIsViewingContentItem(t *testing.T) {
	t.Run("true when contentID set in content mode", func(t *testing.T) {
		m := newTestDiffViewModel()
		m.contentMode = true
		m.contentID = "plan-1"
		if !m.isViewingContentItem() {
			t.Error("expected true for content mode with contentID")
		}
	})
	t.Run("true when contentID set in diff mode", func(t *testing.T) {
		m := newTestDiffViewModel()
		m.contentMode = false
		m.contentID = "plan-1"
		if !m.isViewingContentItem() {
			t.Error("expected true for diff mode with contentID")
		}
	})
	t.Run("false when contentID empty", func(t *testing.T) {
		m := newTestDiffViewModel()
		m.contentMode = false
		m.contentID = ""
		if m.isViewingContentItem() {
			t.Error("expected false for empty contentID")
		}
	})
}

func TestCycleDiffStyle_ContentWithDiff(t *testing.T) {
	m := newTestDiffViewModel()
	m.contentMode = true
	m.contentHasDiff = true
	m.contentID = "plan-1"
	m.contentDiffContent = "# Plan\nSome content"
	m.preferredContentDiffStyle = diffStyleSplit
	m.buildContentLines(m.contentDiffContent)

	cmd := m.CycleDiffStyle()
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd()
	reqMsg, ok := msg.(requestContentDiffMsg)
	if !ok {
		t.Fatalf("expected requestContentDiffMsg, got %T", msg)
	}
	if reqMsg.contentID != "plan-1" {
		t.Errorf("contentID = %q, want %q", reqMsg.contentID, "plan-1")
	}
	if reqMsg.preferredStyle != diffStyleSplit {
		t.Errorf("preferredStyle = %d, want %d (split)", reqMsg.preferredStyle, diffStyleSplit)
	}
}

func TestCycleDiffStyle_ContentWithoutDiff(t *testing.T) {
	m := newTestDiffViewModel()
	m.contentMode = true
	m.contentHasDiff = false
	m.contentID = "plan-1"
	m.contentDiffContent = "# Plan"
	m.buildContentLines(m.contentDiffContent)
	origLineCount := len(m.lines)
	origStyle := m.style

	cmd := m.CycleDiffStyle()
	// No previous version exists, so toggle should be a no-op.
	if cmd != nil {
		t.Error("expected nil cmd for content without diff")
	}
	if !m.contentMode {
		t.Error("expected contentMode to remain true")
	}
	if m.style != origStyle {
		t.Errorf("style changed from %d to %d, expected no change", origStyle, m.style)
	}
	if len(m.lines) != origLineCount {
		t.Errorf("lines changed from %d to %d, expected no change", origLineCount, len(m.lines))
	}
}

func TestCycleDiffStyle_ContentDiffUnifiedToSplit(t *testing.T) {
	m := newTestDiffViewModel()
	m.contentMode = false
	m.contentID = "plan-1"
	m.style = diffStyleUnified
	m.hunks = []types.DiffHunk{{
		OldStart: 1, OldCount: 1, NewStart: 1, NewCount: 1,
		Lines: []types.DiffLine{
			{Kind: types.DiffLineRemoved, Content: "old", OldLineNum: 1},
			{Kind: types.DiffLineAdded, Content: "new", NewLineNum: 1},
		},
	}}
	m.buildLines()

	cmd := m.CycleDiffStyle()
	if cmd != nil {
		t.Error("expected nil cmd (synchronous operation)")
	}
	if m.style != diffStyleSplit {
		t.Errorf("style = %d, want %d (split)", m.style, diffStyleSplit)
	}
	if len(m.lines) == 0 {
		t.Error("expected non-empty lines after switch to split")
	}
}

func TestCycleDiffStyle_ContentDiffSplitBackToContent(t *testing.T) {
	m := newTestDiffViewModel()
	m.contentMode = false
	m.contentID = "plan-1"
	m.contentDiffContent = "# Plan\nSome content"
	m.style = diffStyleSplit
	m.hunks = []types.DiffHunk{{
		OldStart: 1, OldCount: 1, NewStart: 1, NewCount: 1,
		Lines: []types.DiffLine{
			{Kind: types.DiffLineRemoved, Content: "old", OldLineNum: 1},
			{Kind: types.DiffLineAdded, Content: "new", NewLineNum: 1},
		},
	}}
	m.buildLines()

	cmd := m.CycleDiffStyle()
	if cmd != nil {
		t.Error("expected nil cmd (synchronous operation)")
	}
	if !m.contentMode {
		t.Error("expected contentMode = true")
	}
	if m.style != diffStyleUnified {
		t.Errorf("style = %d, want %d (unified)", m.style, diffStyleUnified)
	}
	if m.hunks != nil {
		t.Error("expected hunks to be nil")
	}
	if len(m.lines) == 0 {
		t.Error("expected non-empty content lines")
	}
}

func TestCycleDiffStyle_AdditionalFileNoOp(t *testing.T) {
	m := newTestDiffViewModel()
	m.additionalFilePath = "/tmp/foo.txt"

	cmd := m.CycleDiffStyle()
	if cmd != nil {
		t.Error("expected nil cmd for additional file")
	}
}

func TestLoadContentDiffMsg_NilHunks_StaysInContentMode(t *testing.T) {
	m := newTestDiffViewModel()
	m.contentMode = true
	m.contentID = "plan-1"
	m.contentDiffContent = "# Plan"
	m.buildContentLines(m.contentDiffContent)
	origLineCount := len(m.lines)

	// Result with nil hunks (identical content)
	updated, _ := m.Update(loadContentDiffMsg{
		contentID: "plan-1",
		result:    &types.DiffResult{Path: "plan-1", Hunks: nil},
	})
	if !updated.contentMode {
		t.Error("expected to stay in content mode when hunks are nil")
	}
	if len(updated.lines) != origLineCount {
		t.Errorf("lines changed: got %d, want %d", len(updated.lines), origLineCount)
	}
}

func TestLoadContentDiffMsg_EmptyHunks_StaysInContentMode(t *testing.T) {
	m := newTestDiffViewModel()
	m.contentMode = true
	m.contentID = "plan-1"
	m.contentDiffContent = "# Plan"
	m.buildContentLines(m.contentDiffContent)

	// Result with empty hunks slice
	updated, _ := m.Update(loadContentDiffMsg{
		contentID: "plan-1",
		result:    &types.DiffResult{Path: "plan-1", Hunks: []types.DiffHunk{}},
	})
	if !updated.contentMode {
		t.Error("expected to stay in content mode when hunks are empty")
	}
}

func TestLoadContentDiffMsg_WithHunks_SwitchesToDiffMode(t *testing.T) {
	m := newTestDiffViewModel()
	m.contentMode = true
	m.contentID = "plan-1"
	m.contentDiffContent = "# Plan"
	m.buildContentLines(m.contentDiffContent)

	updated, _ := m.Update(loadContentDiffMsg{
		contentID:      "plan-1",
		preferredStyle: diffStyleSplit,
		result: &types.DiffResult{
			Path: "plan-1",
			Hunks: []types.DiffHunk{{
				OldStart: 1, OldCount: 1, NewStart: 1, NewCount: 1,
				Lines: []types.DiffLine{
					{Kind: types.DiffLineRemoved, Content: "old", OldLineNum: 1},
					{Kind: types.DiffLineAdded, Content: "new", NewLineNum: 1},
				},
			}},
		},
	})
	if updated.contentMode {
		t.Error("expected contentMode = false after loading diff")
	}
	if updated.style != diffStyleSplit {
		t.Errorf("style = %d, want %d (split)", updated.style, diffStyleSplit)
	}
	if len(updated.lines) == 0 {
		t.Error("expected non-empty diff lines")
	}
}

// TestDiffViewShowsValidFile_ContentDiff tests that the validity check recognizes
// content items in diff mode (contentMode=false, contentID set).
func TestDiffViewShowsValidFile_ContentDiff(t *testing.T) {
	theme := DefaultTheme()
	keys := DefaultKeyMap()

	m := appModel{
		diffView: diffViewModel{
			contentMode: false,
			contentID:   "plan-1",
			path:        "content.md",
			theme:       &theme,
			keys:        &keys,
		},
		sidebar: sidebarModel{
			contentItems: []types.ContentItem{
				{ID: "plan-1", Title: "Plan", Content: "stuff"},
			},
		},
	}

	if !m.diffViewShowsValidFile() {
		t.Error("expected diffViewShowsValidFile() = true for content item in diff mode")
	}
}