package db

import (
	"strings"
	"testing"
	"time"

	"github.com/anthropics/monocle/internal/types"
)

func TestDBPath_EnvOverride(t *testing.T) {
	t.Setenv("MONOCLE_DB", "/custom/path/test.db")
	got := DBPath()
	if got != "/custom/path/test.db" {
		t.Errorf("expected /custom/path/test.db, got %q", got)
	}
}

func TestDBPath_Default(t *testing.T) {
	t.Setenv("MONOCLE_DB", "")
	got := DBPath()
	if got == "" {
		t.Error("expected non-empty default path")
	}
	if !strings.HasSuffix(got, "monocle/monocle.db") {
		t.Errorf("expected path ending in monocle/monocle.db, got %q", got)
	}
}

func testDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestSessionCRUD(t *testing.T) {
	d := testDB(t)
	now := time.Now()

	s := &types.ReviewSession{
		ID:          "sess-1",
		Agent:       "claude",
		RepoRoot:    "/tmp/repo",
		BaseRef:     "abc123",
		ReviewRound: 1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := d.CreateSession(s); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := d.GetSession("sess-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Agent != "claude" {
		t.Errorf("got agent=%q", got.Agent)
	}

	s.BaseRef = "def456"
	if err := d.UpdateSession(s); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ = d.GetSession("sess-1")
	if got.BaseRef != "def456" {
		t.Errorf("expected updated base_ref, got %q", got.BaseRef)
	}

	summaries, err := d.ListSessions("", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(summaries) != 1 {
		t.Errorf("expected 1 session, got %d", len(summaries))
	}
}

func TestChangedFiles(t *testing.T) {
	d := testDB(t)
	now := time.Now()
	d.CreateSession(&types.ReviewSession{ID: "sess-1", Agent: "claude", RepoRoot: "/tmp", BaseRef: "abc", ReviewRound: 1, CreatedAt: now, UpdatedAt: now})

	f := &types.ChangedFile{Path: "main.go", Status: types.FileModified}
	if err := d.UpsertChangedFile("sess-1", f); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	files, err := d.GetChangedFiles("sess-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(files) != 1 || files[0].Path != "main.go" {
		t.Errorf("unexpected files: %+v", files)
	}

	if err := d.MarkFileReviewed("sess-1", "main.go", true); err != nil {
		t.Fatalf("mark: %v", err)
	}

	files, _ = d.GetChangedFiles("sess-1")
	if !files[0].Reviewed {
		t.Error("expected reviewed")
	}
}

func TestComments(t *testing.T) {
	d := testDB(t)
	now := time.Now()
	d.CreateSession(&types.ReviewSession{ID: "sess-1", Agent: "claude", RepoRoot: "/tmp", BaseRef: "abc", ReviewRound: 1, CreatedAt: now, UpdatedAt: now})

	c := &types.ReviewComment{
		ID:          "cmt-1",
		TargetType:  types.TargetFile,
		TargetRef:   "main.go",
		LineStart:   10,
		LineEnd:     15,
		Type:        types.CommentIssue,
		Body:        "Fix this bug",
		ReviewRound: 1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := d.CreateComment("sess-1", c); err != nil {
		t.Fatalf("create: %v", err)
	}

	comments, err := d.GetComments("sess-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(comments) != 1 || comments[0].Body != "Fix this bug" {
		t.Errorf("unexpected: %+v", comments)
	}

	c.Body = "Updated body"
	if err := d.UpdateComment(c); err != nil {
		t.Fatalf("update: %v", err)
	}

	comments, _ = d.GetComments("sess-1")
	if comments[0].Body != "Updated body" {
		t.Errorf("expected updated body, got %q", comments[0].Body)
	}

	if err := d.ClearComments("sess-1"); err != nil {
		t.Fatalf("clear comments: %v", err)
	}
	comments, _ = d.GetComments("sess-1")
	if len(comments) != 0 {
		t.Errorf("expected 0 comments after clear, got %d", len(comments))
	}
}

func TestContentItems(t *testing.T) {
	d := testDB(t)
	now := time.Now()
	d.CreateSession(&types.ReviewSession{ID: "sess-1", Agent: "claude", RepoRoot: "/tmp", BaseRef: "abc", ReviewRound: 1, CreatedAt: now, UpdatedAt: now})

	item := &types.ContentItem{
		ID:          "item-1",
		Title:       "Test Plan",
		Content:     "# Test Plan\nSteps...",
		ContentType: "markdown",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := d.UpsertContentItem("sess-1", item); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	items, err := d.GetContentItems("sess-1")
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Test Plan" {
		t.Errorf("unexpected: %+v", items)
	}

	got, err := d.GetContentItem("item-1")
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if got.Content != "# Test Plan\nSteps..." {
		t.Errorf("unexpected content: %q", got.Content)
	}
}

func TestDeleteComment(t *testing.T) {
	d := testDB(t)
	now := time.Now()
	d.CreateSession(&types.ReviewSession{ID: "sess-1", Agent: "claude", RepoRoot: "/tmp", BaseRef: "abc", ReviewRound: 1, CreatedAt: now, UpdatedAt: now})

	c := &types.ReviewComment{
		ID:          "cmt-1",
		TargetType:  types.TargetFile,
		TargetRef:   "main.go",
		LineStart:   10,
		LineEnd:     15,
		Type:        types.CommentIssue,
		Body:        "Fix this bug",
		ReviewRound: 1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := d.CreateComment("sess-1", c); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	if err := d.DeleteComment("cmt-1"); err != nil {
		t.Fatalf("delete comment: %v", err)
	}

	comments, err := d.GetComments("sess-1")
	if err != nil {
		t.Fatalf("get comments: %v", err)
	}
	if len(comments) != 0 {
		t.Errorf("expected 0 comments after delete, got %d", len(comments))
	}
}

func TestClearComments(t *testing.T) {
	d := testDB(t)
	now := time.Now()
	d.CreateSession(&types.ReviewSession{ID: "sess-1", Agent: "claude", RepoRoot: "/tmp", BaseRef: "abc", ReviewRound: 1, CreatedAt: now, UpdatedAt: now})

	c1 := &types.ReviewComment{ID: "cmt-a1", TargetType: types.TargetFile, TargetRef: "a.go", Type: types.CommentIssue, Body: "first", ReviewRound: 1, CreatedAt: now, UpdatedAt: now}
	c2 := &types.ReviewComment{ID: "cmt-a2", TargetType: types.TargetFile, TargetRef: "b.go", Type: types.CommentIssue, Body: "second", ReviewRound: 1, CreatedAt: now, UpdatedAt: now}
	d.CreateComment("sess-1", c1)
	d.CreateComment("sess-1", c2)

	if err := d.ClearComments("sess-1"); err != nil {
		t.Fatalf("clear comments: %v", err)
	}

	comments, err := d.GetComments("sess-1")
	if err != nil {
		t.Fatalf("get comments: %v", err)
	}
	if len(comments) != 0 {
		t.Errorf("expected 0 comments after clear, got %d", len(comments))
	}
}

func TestDeleteChangedFiles(t *testing.T) {
	d := testDB(t)
	now := time.Now()
	d.CreateSession(&types.ReviewSession{ID: "sess-1", Agent: "claude", RepoRoot: "/tmp", BaseRef: "abc", ReviewRound: 1, CreatedAt: now, UpdatedAt: now})

	f1 := &types.ChangedFile{Path: "main.go", Status: types.FileModified}
	f2 := &types.ChangedFile{Path: "util.go", Status: types.FileAdded}
	d.UpsertChangedFile("sess-1", f1)
	d.UpsertChangedFile("sess-1", f2)

	if err := d.DeleteChangedFiles("sess-1"); err != nil {
		t.Fatalf("delete changed files: %v", err)
	}

	files, err := d.GetChangedFiles("sess-1")
	if err != nil {
		t.Fatalf("get changed files: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 changed files after delete, got %d", len(files))
	}
}

func TestCreateAndGetSubmissions(t *testing.T) {
	d := testDB(t)
	now := time.Now()
	d.CreateSession(&types.ReviewSession{ID: "sess-1", Agent: "claude", RepoRoot: "/tmp", BaseRef: "abc", ReviewRound: 1, CreatedAt: now, UpdatedAt: now})

	sub := &types.ReviewSubmission{
		ID:              "sub-1",
		SessionID:       "sess-1",
		Action:          types.ActionRequestChanges,
		FormattedReview: "Please fix the error handling",
		CommentCount:    3,
		ReviewRound:     1,
		SubmittedAt:     now,
	}
	if err := d.CreateSubmission("sess-1", sub); err != nil {
		t.Fatalf("create submission: %v", err)
	}

	subs, err := d.GetSubmissions("sess-1")
	if err != nil {
		t.Fatalf("get submissions: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 submission, got %d", len(subs))
	}
	got := subs[0]
	if got.ID != "sub-1" {
		t.Errorf("expected ID sub-1, got %q", got.ID)
	}
	if got.Action != types.ActionRequestChanges {
		t.Errorf("expected action request_changes, got %q", got.Action)
	}
	if got.FormattedReview != "Please fix the error handling" {
		t.Errorf("expected formatted review text, got %q", got.FormattedReview)
	}
	if got.CommentCount != 3 {
		t.Errorf("expected comment_count 3, got %d", got.CommentCount)
	}
	if got.ReviewRound != 1 {
		t.Errorf("expected review_round 1, got %d", got.ReviewRound)
	}
}

func TestAdditionalFiles(t *testing.T) {
	d := testDB(t)
	now := time.Now()
	d.CreateSession(&types.ReviewSession{ID: "sess-1", Agent: "claude", RepoRoot: "/tmp", BaseRef: "abc", ReviewRound: 1, CreatedAt: now, UpdatedAt: now})

	af := &types.AdditionalFile{Path: "/tmp/extra.go", Name: "extra.go"}
	if err := d.UpsertAdditionalFile("sess-1", af); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	files, err := d.GetAdditionalFiles("sess-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(files) != 1 || files[0].Path != "/tmp/extra.go" || files[0].Name != "extra.go" {
		t.Errorf("unexpected files: %+v", files)
	}

	// Upsert same path updates name
	af2 := &types.AdditionalFile{Path: "/tmp/extra.go", Name: "renamed.go"}
	if err := d.UpsertAdditionalFile("sess-1", af2); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	files, _ = d.GetAdditionalFiles("sess-1")
	if len(files) != 1 || files[0].Name != "renamed.go" {
		t.Errorf("expected updated name, got %+v", files)
	}

	// Mark reviewed
	if err := d.MarkAdditionalFileReviewed("sess-1", "/tmp/extra.go", true); err != nil {
		t.Fatalf("mark reviewed: %v", err)
	}
	files, _ = d.GetAdditionalFiles("sess-1")
	if !files[0].Reviewed {
		t.Error("expected reviewed")
	}

	// Unmark reviewed
	if err := d.MarkAdditionalFileReviewed("sess-1", "/tmp/extra.go", false); err != nil {
		t.Fatalf("unmark reviewed: %v", err)
	}
	files, _ = d.GetAdditionalFiles("sess-1")
	if files[0].Reviewed {
		t.Error("expected not reviewed")
	}

	// Delete
	if err := d.DeleteAdditionalFiles("sess-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	files, _ = d.GetAdditionalFiles("sess-1")
	if len(files) != 0 {
		t.Errorf("expected 0 files after delete, got %d", len(files))
	}
}

func TestListSessions_WithFilter(t *testing.T) {
	d := testDB(t)
	now := time.Now()
	d.CreateSession(&types.ReviewSession{ID: "sess-1", Agent: "claude", RepoRoot: "/tmp/repo-a", BaseRef: "abc", ReviewRound: 1, CreatedAt: now, UpdatedAt: now})
	d.CreateSession(&types.ReviewSession{ID: "sess-2", Agent: "claude", RepoRoot: "/tmp/repo-b", BaseRef: "def", ReviewRound: 1, CreatedAt: now, UpdatedAt: now})

	summaries, err := d.ListSessions("/tmp/repo-a", 0)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 session with filter, got %d", len(summaries))
	}
	if summaries[0].ID != "sess-1" {
		t.Errorf("expected sess-1, got %q", summaries[0].ID)
	}
	if summaries[0].RepoRoot != "/tmp/repo-a" {
		t.Errorf("expected repo root /tmp/repo-a, got %q", summaries[0].RepoRoot)
	}
}

func TestListSessions_WithLimit(t *testing.T) {
	d := testDB(t)
	now := time.Now()
	d.CreateSession(&types.ReviewSession{ID: "sess-1", Agent: "claude", RepoRoot: "/tmp", BaseRef: "abc", ReviewRound: 1, CreatedAt: now, UpdatedAt: now})
	d.CreateSession(&types.ReviewSession{ID: "sess-2", Agent: "claude", RepoRoot: "/tmp", BaseRef: "def", ReviewRound: 1, CreatedAt: now, UpdatedAt: now})
	d.CreateSession(&types.ReviewSession{ID: "sess-3", Agent: "claude", RepoRoot: "/tmp", BaseRef: "ghi", ReviewRound: 1, CreatedAt: now, UpdatedAt: now})

	summaries, err := d.ListSessions("", 2)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(summaries) != 2 {
		t.Errorf("expected 2 sessions with limit, got %d", len(summaries))
	}
}
