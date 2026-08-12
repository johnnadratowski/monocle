package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/db"
	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
)

// TestStagedStateSurvivesRestart exercises what actually happens when the serve
// process dies and comes back: a real on-disk database, the engine torn down
// and rebuilt from scratch, and the same "continue the latest session for this
// repo" resolution that cmd/monocle/serve.go performs at boot.
//
// The existing resume tests all share one process and one session manager, so
// they prove ResumeSession reads the rows — not that a restart reaches it.
func TestStagedStateSurvivesRestart(t *testing.T) {
	repo, _ := setupTestRepo(t)
	dbPath := filepath.Join(t.TempDir(), "monocle.db")

	// Stage some work in the repo so there is a diff to review.
	if err := os.WriteFile(filepath.Join(repo, "hello.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	open := func() (*db.DB, *Engine) {
		t.Helper()
		database, err := db.Open(dbPath)
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		// DefaultConfig, not a zero Config: review tracking defaults on, and a
		// zero value makes MarkReviewed a silent no-op.
		e, err := NewEngine(DefaultConfig(), database, repo, false)
		if err != nil {
			t.Fatalf("new engine: %v", err)
		}
		return database, e
	}

	// --- First run: start a session, stage a comment, mark a file reviewed.
	database, engine := open()
	session, err := engine.StartSession(SessionOptions{Agent: "claude", RepoRoot: repo})
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	files := engine.GetChangedFiles()
	if len(files) == 0 {
		t.Fatal("expected the staged file to show as changed")
	}
	target := files[0].Path

	comment, err := engine.AddComment(
		CommentTarget{TargetType: types.TargetFile, TargetRef: target, LineStart: 1, LineEnd: 1},
		types.CommentIssue, "needs a doc comment")
	if err != nil {
		t.Fatalf("add comment: %v", err)
	}
	if err := engine.MarkReviewed(target); err != nil {
		t.Fatalf("mark reviewed: %v", err)
	}

	// The rest of what a staged review holds, exercised through the same
	// socket handlers the agent's tools reach.
	if r := engine.handleSetReviewName(&protocol.SetReviewNameMsg{Name: "Restart check"}); !r.Success {
		t.Fatalf("set review name: %s", r.Message)
	}
	if r := engine.handleSetFileGroups(&protocol.SetFileGroupsMsg{
		Entries: []protocol.FileGroupEntry{{Path: target, Workstream: "UI", WorkstreamOrder: 1}},
	}); !r.Success {
		t.Fatalf("set file groups: %s", r.Message)
	}
	// Outside the repo: a file already in the diff is deduped out of the
	// additional-files set, which would make this assert nothing.
	extra := filepath.Join(t.TempDir(), "context.md")
	if err := os.WriteFile(extra, []byte("# context\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.AddAdditionalPaths([]string{extra}); err != nil {
		t.Fatalf("add additional paths: %v", err)
	}
	if r := engine.handleAddAnnotations(&protocol.AddAnnotationsMsg{
		Entries: []protocol.AnnotationEntry{{
			File: target, LineStart: 1, LineEnd: 1, Summary: "why this shape",
		}},
	}); !r.Success {
		t.Fatalf("add annotations: %s", r.Message)
	}
	// A submitted-but-uncollected verdict: the safety-critical half, since
	// losing it turns a restart into silent data loss rather than delayed
	// delivery.
	if err := engine.Submit(types.ActionRequestChanges, "please fix"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	firstID := session.ID
	database.Close() // the serve process exits

	// --- Second run: exactly what serve.go does at boot.
	database2, engine2 := open()
	defer database2.Close()

	sessions, err := engine2.ListSessions(ListSessionsOptions{RepoRoot: repo, Limit: 1})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) == 0 {
		t.Fatal("no session found for the repo after restart — nothing would be resumed")
	}
	if sessions[0].ID != firstID {
		t.Fatalf("restart picked session %s, want the one just used (%s)", sessions[0].ID, firstID)
	}
	if _, err := engine2.ResumeSession(sessions[0].ID); err != nil {
		t.Fatalf("resume: %v", err)
	}

	t.Run("the staged comment comes back", func(t *testing.T) {
		got := engine2.GetSession().Comments
		if len(got) != 1 {
			t.Fatalf("expected 1 comment after restart, got %d", len(got))
		}
		if got[0].ID != comment.ID || got[0].Body != "needs a doc comment" {
			t.Errorf("comment did not survive intact: %+v", got[0])
		}
	})

	t.Run("the reviewed mark comes back", func(t *testing.T) {
		for _, f := range engine2.GetChangedFiles() {
			if f.Path == target {
				if !f.Reviewed {
					t.Error("the file lost its reviewed mark across the restart")
				}
				return
			}
		}
		t.Errorf("%s missing from the changed files after restart", target)
	})

	t.Run("the review name comes back", func(t *testing.T) {
		if got := engine2.GetReviewStatusInfo().ReviewName; got != "Restart check" {
			t.Errorf("review name = %q, want %q", got, "Restart check")
		}
	})

	t.Run("the agent's file grouping comes back", func(t *testing.T) {
		for _, f := range engine2.GetChangedFiles() {
			if f.Path == target {
				if f.Workstream != "UI" {
					t.Errorf("workstream = %q, want %q", f.Workstream, "UI")
				}
				return
			}
		}
		t.Errorf("%s missing after restart", target)
	})

	t.Run("added context files come back", func(t *testing.T) {
		if got := engine2.GetAdditionalFiles(); len(got) != 1 {
			t.Errorf("expected 1 additional file after restart, got %d", len(got))
		}
	})

	t.Run("annotations come back", func(t *testing.T) {
		if got := engine2.GetAnnotations(); len(got) != 1 {
			t.Fatalf("expected 1 annotation after restart, got %d", len(got))
		}
	})

	// Without this the reviewer's verdict is gone the moment serve exits, and
	// the agent's next get_feedback reports "nothing pending" — a false all-clear
	// rather than a delayed delivery.
	t.Run("an uncollected verdict is still deliverable", func(t *testing.T) {
		engine2.ReloadPendingFeedback()
		got := engine2.PollFeedback()
		if got == nil {
			t.Fatal("the submitted review did not survive the restart")
		}
		if !strings.Contains(got.Formatted, "please fix") {
			t.Errorf("the verdict came back without its body: %q", got.Formatted)
		}
	})

	t.Run("the base ref comes back", func(t *testing.T) {
		if got, want := engine2.GetSession().BaseRef, session.BaseRef; got != want {
			t.Errorf("base ref = %q, want %q", got, want)
		}
	})
}
