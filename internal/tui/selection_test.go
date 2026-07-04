package tui

import (
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

// TestSyncSidebarSelectionToShown: the highlight follows the shown file even when
// the cursor was left on a different index (e.g. after new files / a regroup).
func TestSyncSidebarSelectionToShown(t *testing.T) {
	m := appModel{}
	m.sidebar.files = []types.ChangedFile{{Path: "a.go"}, {Path: "b.go"}, {Path: "c.go"}}
	m.sidebar.rebuildGroups()
	m.sidebar.cursor = 0 // highlight on a.go...
	m.diffView.path = "c.go" // ...but the diff shows c.go

	m.syncSidebarSelectionToShown()

	if got := m.sidebar.selectedFile(); got == nil || got.Path != "c.go" {
		t.Errorf("cursor should follow the shown file c.go, got %v", got)
	}
}

// TestEditorTargetFileSkipsArtifacts: Ctrl+g on an artifact must not try to open
// a file (the synthetic content.<ext> path), while real files still open.
func TestEditorTargetFileSkipsArtifacts(t *testing.T) {
	// Artifact in diff mode: contentID set, contentMode false, synthetic path.
	art := appModel{repoRoot: "/repo", focus: focusMain}
	art.diffView.contentID = "plan-1"
	art.diffView.path = "content.md"
	if _, _, ok := art.editorTargetFile(); ok {
		t.Error("Ctrl+g on an artifact must not open a file")
	}

	// A real changed file still resolves to a path under the repo root.
	f := appModel{repoRoot: "/repo", focus: focusMain}
	f.diffView.path = "pkg/util.go"
	if p, _, ok := f.editorTargetFile(); !ok || p != "/repo/pkg/util.go" {
		t.Errorf("expected /repo/pkg/util.go, got %q ok=%v", p, ok)
	}
}
