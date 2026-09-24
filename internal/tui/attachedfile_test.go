package tui

import (
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

// TestAttachedFileSurvivesAnEmptyChangeset is the reported symptom: an
// agent-attached file appeared and then emptied itself a moment later. The
// refresh treated "the git changeset is empty" as "there is nothing to show",
// but an attached file is not part of the changeset and outlives it.
func TestAttachedFileSurvivesAnEmptyChangeset(t *testing.T) {
	th := DefaultTheme()
	km := DefaultKeyMap()
	m := appModel{keys: km, width: 120, height: 40}
	m.diffView = newDiffViewModel(&th, &km)
	m.diffView.width, m.diffView.height = 80, 30
	m.sidebar = newSidebarModel(&km)

	// Open an attached file, as selecting one in the sidebar does.
	m.diffView, _ = m.diffView.Update(loadAdditionalFileMsg{
		path:    "/outside/notes.md",
		content: "# Context\n\nNotes the agent attached.\n",
	})
	if len(m.diffView.lines) == 0 {
		t.Fatal("precondition: the attached file should have rendered")
	}
	before := len(m.diffView.lines)

	// A refresh tick with no changed files — the working tree is clean.
	updated, _ := m.Update(refreshResultMsg{
		files:           nil,
		additionalFiles: []types.AdditionalFile{{Path: "/outside/notes.md", Name: "notes.md"}},
	})
	m = updated.(appModel)

	if got := len(m.diffView.lines); got != before {
		t.Errorf("the attached file lost its content: %d lines, want %d", got, before)
	}
	if m.diffView.additionalFilePath != "/outside/notes.md" {
		t.Errorf("additionalFilePath = %q, want the attached file", m.diffView.additionalFilePath)
	}
}

// And the second half: once a changed file does appear, the view must stay on
// the attached file rather than jumping to the changeset.
func TestAttachedFileIsNotStolenByANewChangedFile(t *testing.T) {
	th := DefaultTheme()
	km := DefaultKeyMap()
	m := appModel{keys: km, width: 120, height: 40}
	m.diffView = newDiffViewModel(&th, &km)
	m.diffView.width, m.diffView.height = 80, 30
	m.sidebar = newSidebarModel(&km)

	m.diffView, _ = m.diffView.Update(loadAdditionalFileMsg{
		path: "/outside/notes.md", content: "# Context\n",
	})

	updated, _ := m.Update(refreshResultMsg{
		files:           []types.ChangedFile{{Path: "a.go", Status: types.FileModified}},
		additionalFiles: []types.AdditionalFile{{Path: "/outside/notes.md", Name: "notes.md"}},
	})
	m = updated.(appModel)

	if m.diffView.additionalFilePath != "/outside/notes.md" {
		t.Errorf("the view jumped to the changeset; additionalFilePath = %q", m.diffView.additionalFilePath)
	}
}

// A cleared view must not keep claiming to show an attached file, or
// diffViewShowsValidFile answers yes about a blank pane.
func TestClearFileStateDropsTheAttachedFileClaim(t *testing.T) {
	th := DefaultTheme()
	km := DefaultKeyMap()
	dv := newDiffViewModel(&th, &km)
	dv.additionalFilePath = "/outside/notes.md"
	dv.path = "/outside/notes.md"
	dv.clearFileState()
	if dv.additionalFilePath != "" {
		t.Errorf("additionalFilePath = %q, want it cleared with the rest", dv.additionalFilePath)
	}
}
