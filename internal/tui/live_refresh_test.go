package tui

import (
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

// TestFileChangedReloadsAdditionalFiles reproduces the "UI didn't react to
// remove_files" bug: the agent removes added files (which the engine signals via
// EventFileChanged → fileChangedMsg), and the sidebar must drop them rather than
// keep showing stale full-content entries that shadow the real diffs.
func TestFileChangedReloadsAdditionalFiles(t *testing.T) {
	added := []types.AdditionalFile{{Path: "/repo/extra.go", Name: "extra.go"}}
	stub := &stubEngine{
		changedFiles:    []types.ChangedFile{{Path: "a.go"}},
		additionalFiles: added,
		session:         &types.ReviewSession{},
	}
	m := NewApp(stub)
	m.sidebar.files = stub.changedFiles
	m.sidebar.additionalFiles = added
	m.sidebar.rebuildGroups()
	m.sidebar.rebuildTree()

	// Agent removes all added files (remove_files) — engine now reports none.
	stub.additionalFiles = nil

	updated, _ := m.Update(fileChangedMsg{})
	got := updated.(appModel).sidebar.additionalFiles
	if len(got) != 0 {
		t.Fatalf("added files should be cleared after remove_files, still have %v", got)
	}
}

// TestFileChangedReloadsAddedFilesArrival verifies the reverse: added files that
// appear engine-side (add_files) surface in the sidebar on the next change event.
func TestFileChangedReloadsAddedFilesArrival(t *testing.T) {
	stub := &stubEngine{
		changedFiles: []types.ChangedFile{{Path: "a.go"}},
		session:      &types.ReviewSession{},
	}
	m := NewApp(stub)
	m.sidebar.files = stub.changedFiles
	m.sidebar.rebuildGroups()
	m.sidebar.rebuildTree()

	stub.additionalFiles = []types.AdditionalFile{{Path: "/repo/ctx.go", Name: "ctx.go"}}

	updated, _ := m.Update(fileChangedMsg{})
	got := updated.(appModel).sidebar.additionalFiles
	if len(got) != 1 || got[0].Path != "/repo/ctx.go" {
		t.Fatalf("added file should surface after change event, got %v", got)
	}
}
