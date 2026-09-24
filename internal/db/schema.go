package db

import (
	"database/sql"
	"fmt"
)

const schemaVersion = 16

const dropSQL = `
DROP TABLE IF EXISTS review_snapshot_files;
DROP TABLE IF EXISTS review_snapshots;
DROP TABLE IF EXISTS review_submissions;
DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS content_versions;
DROP TABLE IF EXISTS content_items;
DROP TABLE IF EXISTS additional_files;
DROP TABLE IF EXISTS file_metadata;
DROP TABLE IF EXISTS annotations;
DROP TABLE IF EXISTS changed_files;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS schema_version;
`

const schemaSQL = `
CREATE TABLE IF NOT EXISTS schema_version (
	version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	agent TEXT NOT NULL,
	repo_root TEXT NOT NULL,
	base_ref TEXT NOT NULL,
	review_name TEXT NOT NULL DEFAULT '',
	summary_overview TEXT NOT NULL DEFAULT '',
	review_sent_at DATETIME,
	auto_advance_ref INTEGER NOT NULL DEFAULT 1,
	selected_ref TEXT NOT NULL DEFAULT '',
	ignore_patterns TEXT NOT NULL DEFAULT '[]',
	review_round INTEGER NOT NULL DEFAULT 1,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS changed_files (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT NOT NULL REFERENCES sessions(id),
	path TEXT NOT NULL,
	status TEXT NOT NULL,
	reviewed INTEGER NOT NULL DEFAULT 0,
	additions INTEGER NOT NULL DEFAULT 0,
	deletions INTEGER NOT NULL DEFAULT 0,
	UNIQUE(session_id, path)
);

-- Agent-authored code annotations (rationale + doc links). A separate channel
-- from reviewer comments; never sent back to the agent as feedback. refs holds
-- a JSON array of DocRef.
CREATE TABLE IF NOT EXISTS annotations (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL REFERENCES sessions(id),
	target_ref TEXT NOT NULL,
	line_start INTEGER NOT NULL DEFAULT 0,
	line_end INTEGER NOT NULL DEFAULT 0,
	summary TEXT NOT NULL DEFAULT '',
	refs TEXT NOT NULL DEFAULT '[]',
	review_round INTEGER NOT NULL DEFAULT 1,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- The agent's account of what a review round fixed: one short line per item,
-- with the files/line-ranges it accounts for carried as a JSON array of
-- SummaryTarget. Replaced wholesale when the agent sends a new set.
CREATE TABLE IF NOT EXISTS summary_items (
	session_id TEXT NOT NULL REFERENCES sessions(id),
	id TEXT NOT NULL,
	text TEXT NOT NULL DEFAULT '',
	sort_order INTEGER NOT NULL DEFAULT 0,
	targets TEXT NOT NULL DEFAULT '[]',
	UNIQUE(session_id, id)
);

-- Agent-supplied per-file grouping metadata. Kept in a separate table so it
-- survives changed_files being replaced on refresh; joined back in on read.
CREATE TABLE IF NOT EXISTS file_metadata (
	session_id TEXT NOT NULL REFERENCES sessions(id),
	path TEXT NOT NULL,
	workstream TEXT NOT NULL DEFAULT '',
	workstream_order INTEGER NOT NULL DEFAULT 0,
	category TEXT NOT NULL DEFAULT '',
	group_label TEXT NOT NULL DEFAULT '',
	group_order INTEGER NOT NULL DEFAULT 0,
	sort_index INTEGER NOT NULL DEFAULT 0,
	criticality INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY(session_id, path)
);

CREATE TABLE IF NOT EXISTS content_items (
	id TEXT NOT NULL,
	session_id TEXT NOT NULL REFERENCES sessions(id),
	title TEXT NOT NULL,
	content TEXT NOT NULL,
	content_type TEXT NOT NULL DEFAULT 'text',
	is_plan INTEGER NOT NULL DEFAULT 0,
	reviewed INTEGER NOT NULL DEFAULT 0,
	media_path TEXT NOT NULL DEFAULT '',
	media_type TEXT NOT NULL DEFAULT '',
	mime_type TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY(id, session_id)
);

CREATE TABLE IF NOT EXISTS content_versions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	content_item_id TEXT NOT NULL,
	session_id TEXT NOT NULL REFERENCES sessions(id),
	version INTEGER NOT NULL,
	title TEXT NOT NULL,
	content TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(content_item_id, session_id, version)
);

CREATE TABLE IF NOT EXISTS comments (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL REFERENCES sessions(id),
	target_type TEXT NOT NULL,
	target_ref TEXT NOT NULL,
	line_start INTEGER,
	line_end INTEGER,
	type TEXT NOT NULL,
	body TEXT NOT NULL,
	code_snippet TEXT NOT NULL DEFAULT '',
	resolved INTEGER NOT NULL DEFAULT 0,
	outdated INTEGER NOT NULL DEFAULT 0,
	review_round INTEGER NOT NULL DEFAULT 1,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS review_submissions (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL REFERENCES sessions(id),
	action TEXT NOT NULL,
	formatted_review TEXT NOT NULL,
	comment_count INTEGER NOT NULL DEFAULT 0,
	review_round INTEGER NOT NULL DEFAULT 1,
	submitted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	delivered_at DATETIME
);

CREATE TABLE IF NOT EXISTS additional_files (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT NOT NULL REFERENCES sessions(id),
	path TEXT NOT NULL,
	name TEXT NOT NULL,
	reviewed INTEGER NOT NULL DEFAULT 0,
	UNIQUE(session_id, path)
);

CREATE TABLE IF NOT EXISTS review_snapshots (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT NOT NULL REFERENCES sessions(id),
	submission_id TEXT NOT NULL REFERENCES review_submissions(id),
	review_round INTEGER NOT NULL,
	head_ref TEXT NOT NULL,
	base_ref TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS review_snapshot_files (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	snapshot_id INTEGER NOT NULL REFERENCES review_snapshots(id),
	path TEXT NOT NULL,
	status TEXT NOT NULL,
	reviewed INTEGER NOT NULL DEFAULT 0,
	blob_sha TEXT NOT NULL DEFAULT '',
	content TEXT NOT NULL DEFAULT '',
	UNIQUE(snapshot_id, path)
);

CREATE INDEX IF NOT EXISTS idx_changed_files_session ON changed_files(session_id);
CREATE INDEX IF NOT EXISTS idx_annotations_session ON annotations(session_id);
CREATE INDEX IF NOT EXISTS idx_content_items_session ON content_items(session_id);
CREATE INDEX IF NOT EXISTS idx_comments_session ON comments(session_id);
CREATE INDEX IF NOT EXISTS idx_comments_target ON comments(target_type, target_ref);
CREATE INDEX IF NOT EXISTS idx_review_submissions_session ON review_submissions(session_id);
CREATE INDEX IF NOT EXISTS idx_additional_files_session ON additional_files(session_id);
CREATE INDEX IF NOT EXISTS idx_review_snapshots_session ON review_snapshots(session_id);
CREATE INDEX IF NOT EXISTS idx_review_snapshot_files_snapshot ON review_snapshot_files(snapshot_id);
`

// Migrate checks the schema version and applies migrations as needed.
func Migrate(db *sql.DB) error {
	// Check if schema_version table exists
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_version'").Scan(&count)
	if err != nil {
		return fmt.Errorf("check schema_version: %w", err)
	}

	if count == 0 {
		// Fresh database — apply full schema
		if _, err := db.Exec(schemaSQL); err != nil {
			return fmt.Errorf("apply schema: %w", err)
		}
		if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion); err != nil {
			return fmt.Errorf("set schema version: %w", err)
		}
		return nil
	}

	// Check current version
	var currentVersion int
	err = db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	if currentVersion > schemaVersion {
		return fmt.Errorf("database schema version %d is newer than supported version %d", currentVersion, schemaVersion)
	}

	// A database that is intact and recent enough is migrated in place. The
	// drop-and-recreate below is fine for a change that restructures data, but it
	// would also throw away a review the reviewer has staged and not yet
	// submitted — so a change that only appends nullable columns says so in
	// addedColumns and keeps the data.
	if currentVersion >= firstAdditiveVersion && schemaIntact(db) {
		// Every statement in schemaSQL is CREATE TABLE IF NOT EXISTS, so replaying
		// it adds tables introduced since without touching a row of the ones
		// already there. Columns appended to existing tables still need addColumns.
		if _, err := db.Exec(schemaSQL); err != nil {
			return fmt.Errorf("apply new tables: %w", err)
		}
		if err := addColumns(db); err != nil {
			return err
		}
		if currentVersion == schemaVersion {
			return nil
		}
		return setVersion(db, schemaVersion)
	}

	// Drop and recreate (safe during pre-release development).
	if _, err := db.Exec(dropSQL); err != nil {
		return fmt.Errorf("drop old schema: %w", err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	return nil
}

// firstAdditiveVersion is the oldest schema a column-append can upgrade in place.
// Anything older predates a restructuring migration and still takes the
// drop-and-recreate path.
const firstAdditiveVersion = 13

// addedColumns lists nullable columns appended since firstAdditiveVersion, as
// table -> column -> DDL type. Adding one here (and to schemaSQL) is the whole
// migration: existing rows get NULL, which every reader already tolerates.
var addedColumns = map[string]map[string]string{
	"sessions": {
		"review_sent_at":   "DATETIME",
		"summary_overview": "TEXT NOT NULL DEFAULT ''",
	},
}

// schemaIntact reports whether the tables look like the schema they claim to be.
// A version row can outlive the tables it described (a half-applied migration, a
// hand-edited database), so the version alone is not proof.
func schemaIntact(db *sql.DB) bool {
	var n int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = 'repo_root'",
	).Scan(&n)
	return err == nil && n == 1
}

// addColumns appends any columns in addedColumns that the database is missing.
func addColumns(db *sql.DB) error {
	for table, cols := range addedColumns {
		for col, typ := range cols {
			var n int
			if err := db.QueryRow(
				"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, col,
			).Scan(&n); err != nil {
				return fmt.Errorf("check %s.%s: %w", table, col, err)
			}
			if n > 0 {
				continue
			}
			if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, col, typ)); err != nil {
				return fmt.Errorf("add %s.%s: %w", table, col, err)
			}
		}
	}
	return nil
}

// setVersion rewrites the single schema_version row.
func setVersion(db *sql.DB, v int) error {
	if _, err := db.Exec("DELETE FROM schema_version"); err != nil {
		return fmt.Errorf("clear schema version: %w", err)
	}
	if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (?)", v); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	return nil
}
