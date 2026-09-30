package db

import (
	"reflect"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func walkthroughDB(t *testing.T) *DB {
	t.Helper()
	d := testDB(t)
	if err := d.CreateSession(&types.ReviewSession{ID: "s1", Agent: "claude", RepoRoot: "/r", BaseRef: "main"}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return d
}

func sampleTour() *types.Walkthrough {
	return &types.Walkthrough{Title: "Tour", Stops: []types.WalkthroughStop{
		{ID: "1.1", Title: "Entry", File: "a.go", LineStart: 3, LineEnd: 8, Note: "why",
			Related: []types.DocRef{{Kind: types.DocRefFile, Doc: "b.go", StartLine: 12}},
			Views:   []types.StopView{{Kind: types.StopViewVideo, Target: "demo.webm", Label: "demo"}},
			Layout:  "review"},
		{ID: "1.2", Title: "Effect"},
	}}
}

func TestWalkthroughStorage(t *testing.T) {
	t.Run("no tour reads as nil, not an error", func(t *testing.T) {
		d := walkthroughDB(t)
		w, cur, err := d.GetWalkthrough("s1")
		if err != nil || w != nil || cur != "" {
			t.Errorf("got %+v %q %v, want nothing", w, cur, err)
		}
	})

	t.Run("round-trips every field of every stop", func(t *testing.T) {
		d := walkthroughDB(t)
		want := sampleTour()
		if err := d.SaveWalkthrough("s1", want, "1.2"); err != nil {
			t.Fatal(err)
		}
		got, cur, err := d.GetWalkthrough("s1")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) || cur != "1.2" {
			t.Errorf("got %+v at %q, want %+v at 1.2", got, cur, want)
		}
	})

	t.Run("saving again replaces the tour", func(t *testing.T) {
		d := walkthroughDB(t)
		_ = d.SaveWalkthrough("s1", sampleTour(), "1.1")
		next := &types.Walkthrough{Stops: []types.WalkthroughStop{{ID: "only"}}}
		if err := d.SaveWalkthrough("s1", next, "only"); err != nil {
			t.Fatal(err)
		}
		got, cur, _ := d.GetWalkthrough("s1")
		if len(got.Stops) != 1 || got.Stops[0].ID != "only" || cur != "only" || got.Title != "" {
			t.Errorf("got %+v at %q, want only the newer tour", got, cur)
		}
	})

	t.Run("the reviewer's stop moves without touching the stops", func(t *testing.T) {
		d := walkthroughDB(t)
		_ = d.SaveWalkthrough("s1", sampleTour(), "1.1")
		if err := d.SetWalkthroughStop("s1", "1.2"); err != nil {
			t.Fatal(err)
		}
		got, cur, _ := d.GetWalkthrough("s1")
		if cur != "1.2" || len(got.Stops) != 2 {
			t.Errorf("got %d stops at %q, want 2 at 1.2", len(got.Stops), cur)
		}
	})

	t.Run("delete removes it", func(t *testing.T) {
		d := walkthroughDB(t)
		_ = d.SaveWalkthrough("s1", sampleTour(), "1.1")
		if err := d.DeleteWalkthrough("s1"); err != nil {
			t.Fatal(err)
		}
		if w, _, _ := d.GetWalkthrough("s1"); w != nil {
			t.Errorf("got %+v after delete, want nil", w)
		}
	})

	t.Run("tours are per session", func(t *testing.T) {
		d := walkthroughDB(t)
		_ = d.CreateSession(&types.ReviewSession{ID: "s2", Agent: "claude", RepoRoot: "/r", BaseRef: "main"})
		_ = d.SaveWalkthrough("s1", sampleTour(), "1.1")
		if w, _, _ := d.GetWalkthrough("s2"); w != nil {
			t.Errorf("s2 sees %+v, want no tour", w)
		}
	})
}

// An upgrading user's database is on the previous schema, with a review staged.
// The new table has to appear without that review being dropped.
func TestMigrateAddsWalkthroughsWithoutLosingData(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	if err := d.CreateSession(&types.ReviewSession{ID: "keep", Agent: "claude", RepoRoot: "/r", BaseRef: "main", ReviewName: "Staged"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Rewind to the schema before walkthroughs existed.
	if _, err := d.Exec("DROP TABLE walkthroughs"); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if err := setVersion(d.DB, 17); err != nil {
		t.Fatalf("rewind version: %v", err)
	}

	if err := Migrate(d.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if got, err := d.GetSession("keep"); err != nil || got.ReviewName != "Staged" {
		t.Fatalf("staged review lost in the migration: %+v %v", got, err)
	}
	if err := d.SaveWalkthrough("keep", sampleTour(), "1.1"); err != nil {
		t.Errorf("walkthroughs table missing after migration: %v", err)
	}
	var v int
	if err := d.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&v); err != nil || v != schemaVersion {
		t.Errorf("schema version = %d (%v), want %d", v, err, schemaVersion)
	}
}

// Comments gained a stop_id column; a database from before it must get the
// column without losing the comments it already holds.
func TestMigrateAddsCommentStopID(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	_ = d.CreateSession(&types.ReviewSession{ID: "s", Agent: "claude", RepoRoot: "/r", BaseRef: "main"})
	if err := d.CreateComment("s", &types.ReviewComment{ID: "old", TargetType: types.TargetFile, TargetRef: "a.go",
		Type: types.CommentNote, Body: "staged", ReviewRound: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec("ALTER TABLE comments DROP COLUMN stop_id"); err != nil {
		t.Fatalf("rewind: %v", err)
	}
	if err := setVersion(d.DB, 17); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(d.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	got, err := d.GetComments("s")
	if err != nil || len(got) != 1 || got[0].Body != "staged" || got[0].StopID != "" {
		t.Fatalf("comments after migration = %+v (%v)", got, err)
	}
	if err := d.CreateComment("s", &types.ReviewComment{ID: "new", TargetType: types.TargetFile, TargetRef: "a.go",
		Type: types.CommentQuestion, Body: "why?", ReviewRound: 1, StopID: "1.2"}); err != nil {
		t.Fatal(err)
	}
	got, _ = d.GetComments("s")
	if len(got) != 2 || got[1].StopID != "1.2" {
		t.Errorf("stop id did not round-trip: %+v", got)
	}
}
