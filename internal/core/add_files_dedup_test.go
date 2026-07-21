package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/db"
	"github.com/josephschmitt/monocle/internal/types"
)

// TestAddAdditionalPaths_SkipsBaseRefChangeset reproduces the "add_files renders
// changed files as full content" bug: a file already in the base-ref changeset
// (shown to the reviewer as a diff) must not also be added as a full-content
// additional file — that would shadow the diff.
func TestAddAdditionalPaths_SkipsBaseRefChangeset(t *testing.T) {
	repo := t.TempDir()
	changed := filepath.Join(repo, "changed.go")
	extra := filepath.Join(repo, "extra.go")
	for _, p := range []string{changed, extra} {
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// The stub's Diff (the base-ref changeset) contains only changed.go.
	stub := &gitStub{
		repoRoot:   repo,
		currentRef: "head1head1head1head1head1head1head1head01",
		files:      []types.ChangedFile{{Path: "changed.go", Status: types.FileModified}},
	}

	now := time.Now()
	session := &types.ReviewSession{
		ID: "sess-1", Agent: "claude",
		RepoRoot: repo, BaseRef: "base-sha", ReviewRound: 1,
		FileStatuses: make(map[string]bool), CreatedAt: now, UpdatedAt: now,
	}
	if err := database.CreateSession(session); err != nil {
		t.Fatal(err)
	}

	e := &Engine{
		feedback:    NewFeedbackQueue(),
		database:    database,
		git:         stub,
		subscribers: make(map[EventKind]map[int]EventCallback),
	}
	e.current = session

	added, skipped, err := e.addAdditionalPaths([]string{changed, extra})
	if err != nil {
		t.Fatalf("addAdditionalPaths: %v", err)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1 (changed.go is in the base-ref changeset)", skipped)
	}
	if len(added) != 1 || added[0].Path != extra {
		t.Fatalf("added = %+v, want only %s", added, extra)
	}
	for _, af := range e.current.AdditionalFiles {
		if af.Path == changed {
			t.Errorf("changed.go was added as a full-content file; it must show as a diff")
		}
	}
}

// TestAddAdditionalPaths_NoBaseRefAddsAll documents the edge where no base ref is
// set: with nothing yet rendered as a diff, add_files adds every path.
func TestAddAdditionalPaths_NoBaseRefAddsAll(t *testing.T) {
	repo := t.TempDir()
	a := filepath.Join(repo, "a.go")
	if err := os.WriteFile(a, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	stub := &gitStub{repoRoot: repo, files: []types.ChangedFile{{Path: "a.go"}}}
	now := time.Now()
	session := &types.ReviewSession{
		ID: "s", Agent: "c", RepoRoot: repo, BaseRef: "",
		FileStatuses: make(map[string]bool), CreatedAt: now, UpdatedAt: now,
	}
	if err := database.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	e := &Engine{
		feedback:    NewFeedbackQueue(),
		database:    database,
		git:         stub,
		subscribers: make(map[EventKind]map[int]EventCallback),
	}
	e.current = session

	added, skipped, err := e.addAdditionalPaths([]string{a})
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 0 || len(added) != 1 {
		t.Errorf("empty base ref: added=%d skipped=%d, want added=1 skipped=0", len(added), skipped)
	}
}
