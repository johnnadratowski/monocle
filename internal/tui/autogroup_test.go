package tui

import (
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func groupedSidebar(t *testing.T, grouped bool) sidebarModel {
	t.Helper()
	files := []types.ChangedFile{{Path: "a.go"}, {Path: "b.go"}}
	if grouped {
		files[0].Workstream = "UI"
		files[1].Workstream = "Backend"
	}
	return sidebarModel{files: files}
}

func TestAutoEnableGrouping(t *testing.T) {
	t.Run("ungrouped files leave the view alone", func(t *testing.T) {
		m := groupedSidebar(t, false)
		m.rebuildGroups()
		if m.groupMode {
			t.Error("grouped view switched on without any agent grouping")
		}
	})

	t.Run("arriving groups switch the sidebar to grouped", func(t *testing.T) {
		m := groupedSidebar(t, false)
		m.rebuildGroups()
		// The agent now sends groupings and the files refresh.
		m.files[0].Workstream = "UI"
		m.rebuildGroups()
		if !m.groupMode {
			t.Error("grouped view should switch on when the agent sends groupings")
		}
	})

	t.Run("it leaves tree mode rather than nesting the two", func(t *testing.T) {
		m := groupedSidebar(t, true)
		m.treeMode = true
		m.rebuildGroups()
		if !m.groupMode || m.treeMode {
			t.Errorf("want grouped on and tree off, got grouped=%v tree=%v", m.groupMode, m.treeMode)
		}
	})

	t.Run("a group label alone is enough", func(t *testing.T) {
		m := sidebarModel{files: []types.ChangedFile{{Path: "a.go", GroupLabel: "Schema"}}}
		m.rebuildGroups()
		if !m.groupMode {
			t.Error("GroupLabel should count as agent grouping")
		}
	})

	t.Run("an inferred category is not agent grouping", func(t *testing.T) {
		// Category is derived for every file by monocle itself, so it must not
		// be read as the agent having arranged the review.
		m := sidebarModel{files: []types.ChangedFile{{Path: "a.go", Category: "source"}}}
		m.rebuildGroups()
		if m.groupMode {
			t.Error("an inferred category should not switch the view")
		}
	})

	t.Run("additional files can carry the grouping", func(t *testing.T) {
		m := sidebarModel{
			files:           []types.ChangedFile{{Path: "a.go"}},
			additionalFiles: []types.AdditionalFile{{Name: "iface.go", Workstream: "Contracts"}},
		}
		m.rebuildGroups()
		if !m.groupMode {
			t.Error("grouping on an additional file should still switch the view")
		}
	})

	// The important half: revealing the agent's ordering must not become a
	// fight over the view every time files refresh.
	t.Run("leaving grouped view sticks across refreshes", func(t *testing.T) {
		m := groupedSidebar(t, true)
		m.rebuildGroups()
		if !m.groupMode {
			t.Fatal("expected the initial switch to grouped")
		}
		m.groupMode = false // the reader pressed f
		for i := 0; i < 3; i++ {
			m.rebuildGroups()
		}
		if m.groupMode {
			t.Error("a refresh dragged the reader back to grouped view")
		}
	})
}
