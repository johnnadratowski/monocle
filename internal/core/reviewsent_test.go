package core

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/db"
	"github.com/josephschmitt/monocle/internal/protocol"
)

func sentEngine(t *testing.T) (*Engine, *SocketServer) {
	t.Helper()
	repo, _ := setupTestRepo(t)
	database, err := db.Open(filepath.Join(t.TempDir(), "monocle.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	e, err := NewEngine(DefaultConfig(), database, repo, false)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	if _, err := e.StartSession(SessionOptions{Agent: "claude", RepoRoot: repo}); err != nil {
		t.Fatalf("start session: %v", err)
	}
	return e, &SocketServer{engine: e}
}

// TestReviewSentStamp pins the lifecycle of the "sent" timestamp shown beside the
// review title: one stamp per handover, surviving the follow-up calls an agent
// makes to dress the same review, and cleared once the agent has the feedback.
func TestReviewSentStamp(t *testing.T) {
	t.Run("a handover stamps once, not per call", func(t *testing.T) {
		e, srv := sentEngine(t)
		if !e.current.SentAt.IsZero() {
			t.Fatal("a fresh session has not been sent to anyone")
		}

		srv.handleMessage(&protocol.SetReviewNameMsg{Type: protocol.TypeSetReviewName, Name: "Auth rework"})
		first := e.current.SentAt
		if first.IsZero() {
			t.Fatal("naming a review is handing it over; it should be stamped")
		}

		// The rest of the same handover must not move the stamp forward, or a
		// chatty agent makes an hour-old review read as brand new.
		time.Sleep(2 * time.Millisecond)
		srv.handleMessage(&protocol.SubmitContentMsg{
			Type: protocol.TypeSubmitContent, ID: "plan", Title: "Plan", Content: "# plan",
		})
		srv.handleMessage(&protocol.SetFileGroupsMsg{Type: protocol.TypeSetFileGroups})
		if !e.current.SentAt.Equal(first) {
			t.Errorf("SentAt moved to %v, want it pinned at %v", e.current.SentAt, first)
		}
	})

	t.Run("working is not sending", func(t *testing.T) {
		e, srv := sentEngine(t)
		srv.handleMessage(&protocol.MarkActivityMsg{Type: protocol.TypeMarkActivity})
		if !e.current.SentAt.IsZero() {
			t.Error("mark-activity is a per-turn pulse, not a handover")
		}
		srv.handleMessage(&protocol.GetReviewStatusMsg{Type: protocol.TypeGetReviewStatus})
		if !e.current.SentAt.IsZero() {
			t.Error("polling status must not stamp a send")
		}
	})

	t.Run("collecting feedback arms the next round", func(t *testing.T) {
		e, srv := sentEngine(t)
		srv.handleMessage(&protocol.SetReviewNameMsg{Type: protocol.TypeSetReviewName, Name: "Auth rework"})
		if e.current.SentAt.IsZero() {
			t.Fatal("expected a stamp to clear")
		}

		e.completeQueuedDelivery()
		if !e.current.SentAt.IsZero() {
			t.Fatal("the agent has the feedback; nothing is sitting in front of the reviewer")
		}

		srv.handleMessage(&protocol.SubmitContentMsg{
			Type: protocol.TypeSubmitContent, ID: "plan2", Title: "Round 2", Content: "# v2",
		})
		if e.current.SentAt.IsZero() {
			t.Error("round two's send should stamp again")
		}
	})

	t.Run("a stamp survives a restart", func(t *testing.T) {
		e, srv := sentEngine(t)
		srv.handleMessage(&protocol.SetReviewNameMsg{Type: protocol.TypeSetReviewName, Name: "Auth rework"})
		want := e.current.SentAt

		resumed, err := e.ResumeSession(e.current.ID)
		if err != nil {
			t.Fatalf("resume: %v", err)
		}
		if !resumed.SentAt.Equal(want.Truncate(time.Second)) && !resumed.SentAt.Equal(want) {
			t.Errorf("resumed SentAt = %v, want %v", resumed.SentAt, want)
		}
	})

	t.Run("clearing the review clears the stamp", func(t *testing.T) {
		e, srv := sentEngine(t)
		srv.handleMessage(&protocol.SetReviewNameMsg{Type: protocol.TypeSetReviewName, Name: "Auth rework"})
		if err := e.ClearReview(); err != nil {
			t.Fatalf("clear: %v", err)
		}
		if !e.current.SentAt.IsZero() {
			t.Errorf("SentAt = %v, want zero after a full clear", e.current.SentAt)
		}
	})

	t.Run("approval closes the review and the stamp with it", func(t *testing.T) {
		e, srv := sentEngine(t)
		srv.handleMessage(&protocol.SetReviewNameMsg{Type: protocol.TypeSetReviewName, Name: "Auth rework"})
		e.closeReviewOnApprove()
		if e.current.ReviewName != "" || !e.current.SentAt.IsZero() {
			t.Errorf("after approve: name=%q sentAt=%v, want both empty", e.current.ReviewName, e.current.SentAt)
		}
	})

	// The stamp is what the top bar ages, so it must never be in the future or
	// the display would read "sent -3m ago".
	t.Run("the stamp is not in the future", func(t *testing.T) {
		e, srv := sentEngine(t)
		before := time.Now()
		srv.handleMessage(&protocol.SetReviewNameMsg{Type: protocol.TypeSetReviewName, Name: "Auth rework"})
		after := time.Now()
		if e.current.SentAt.Before(before) || e.current.SentAt.After(after) {
			t.Errorf("SentAt = %v, want between %v and %v", e.current.SentAt, before, after)
		}
	})

}
