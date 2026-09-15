package db

import (
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/types"
)

// TestSentAtRoundTrip covers the stamp that tells the reviewer how old the round
// in front of them is. The zero value is the interesting half: it means "nothing
// sent since the agent last collected feedback", and must not come back as year 1.
func TestSentAtRoundTrip(t *testing.T) {
	sent := time.Now().Add(-90 * time.Minute).Truncate(time.Second)

	t.Run("set", func(t *testing.T) {
		d := testDB(t)
		s := &types.ReviewSession{ID: "s1", Agent: "claude", RepoRoot: "/r", BaseRef: "main", SentAt: sent}
		if err := d.CreateSession(s); err != nil {
			t.Fatalf("create: %v", err)
		}
		got, err := d.GetSession("s1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !got.SentAt.Equal(sent) {
			t.Errorf("SentAt = %v, want %v", got.SentAt, sent)
		}
	})

	t.Run("zero stays zero", func(t *testing.T) {
		d := testDB(t)
		s := &types.ReviewSession{ID: "s2", Agent: "claude", RepoRoot: "/r", BaseRef: "main"}
		if err := d.CreateSession(s); err != nil {
			t.Fatalf("create: %v", err)
		}
		got, err := d.GetSession("s2")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !got.SentAt.IsZero() {
			t.Errorf("SentAt = %v, want zero", got.SentAt)
		}
	})

	t.Run("update both ways", func(t *testing.T) {
		d := testDB(t)
		s := &types.ReviewSession{ID: "s3", Agent: "claude", RepoRoot: "/r", BaseRef: "main"}
		if err := d.CreateSession(s); err != nil {
			t.Fatalf("create: %v", err)
		}
		s.SentAt = sent
		if err := d.UpdateSession(s); err != nil {
			t.Fatalf("update: %v", err)
		}
		if got, _ := d.GetSession("s3"); !got.SentAt.Equal(sent) {
			t.Fatalf("after update SentAt = %v, want %v", got.SentAt, sent)
		}
		s.SentAt = time.Time{}
		if err := d.UpdateSession(s); err != nil {
			t.Fatalf("clear: %v", err)
		}
		if got, _ := d.GetSession("s3"); !got.SentAt.IsZero() {
			t.Errorf("after clear SentAt = %v, want zero", got.SentAt)
		}
	})
}

// TestMigrateKeepsDataWhenAppendingColumn is the reason the additive path exists:
// the drop-and-recreate fallback would take a review the user staged and had not
// yet submitted with it.
func TestMigrateKeepsDataWhenAppendingColumn(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()

	s := &types.ReviewSession{ID: "keep-me", Agent: "claude", RepoRoot: "/r", BaseRef: "main", ReviewName: "Staged review"}
	if err := d.CreateSession(s); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Rewind to the last schema that lacked the column, as an upgrading user's
	// database would look.
	if _, err := d.Exec("ALTER TABLE sessions DROP COLUMN review_sent_at"); err != nil {
		t.Fatalf("drop column: %v", err)
	}
	if err := setVersion(d.DB, firstAdditiveVersion); err != nil {
		t.Fatalf("rewind version: %v", err)
	}

	if err := Migrate(d.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	got, err := d.GetSession("keep-me")
	if err != nil {
		t.Fatalf("session did not survive the migration: %v", err)
	}
	if got.ReviewName != "Staged review" {
		t.Errorf("review name = %q, want it preserved", got.ReviewName)
	}
	if !got.SentAt.IsZero() {
		t.Errorf("back-filled SentAt = %v, want zero", got.SentAt)
	}

	var v int
	if err := d.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&v); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if v != schemaVersion {
		t.Errorf("schema version = %d, want %d", v, schemaVersion)
	}

	// And the migration is idempotent — a second run must not double-add.
	if err := Migrate(d.DB); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = 'review_sent_at'").Scan(&n); err != nil {
		t.Fatalf("count column: %v", err)
	}
	if n != 1 {
		t.Errorf("review_sent_at column count = %d, want 1", n)
	}
}
