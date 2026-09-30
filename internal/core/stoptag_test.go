package core

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/db"
	"github.com/josephschmitt/monocle/internal/types"
)

// A question asked during a tour arrives as "[1.2] …", so the agent reads it
// against what it said at stop 1.2.
func TestFeedbackLeadsWithTheStop(t *testing.T) {
	f := NewReviewFormatter(nil, types.ReviewFormatConfig{})
	comments := []types.ReviewComment{
		{ID: "a", TargetType: types.TargetFile, TargetRef: "a.go", LineStart: 3, Type: types.CommentQuestion,
			Body: "Does x get hoisted here?", StopID: "1.2"},
		{ID: "b", TargetType: types.TargetFile, TargetRef: "b.go", LineStart: 9, Type: types.CommentSuggestion,
			Body: "```suggestion\nlet y = 2\n```", StopID: "2"},
		{ID: "c", TargetType: types.TargetContent, TargetRef: "plan", LineStart: 1, Type: types.CommentNote,
			Body: "Untagged, written outside the tour."},
	}
	out := f.Format(&types.ReviewSession{}, comments, types.ActionQuestions, "").Formatted
	for _, want := range []string{
		"[1.2] Does x get hoisted here?",
		// Text before ``` would break the suggestion's fence, so the tag
		// gets its own line.
		"[2]\n```suggestion\nlet y = 2\n```",
		"Untagged, written outside the tour.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("feedback is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[] Untagged") {
		t.Error("a comment with no stop must not get an empty tag")
	}
}

func TestCommentKeepsItsStopAcrossARestart(t *testing.T) {
	repo, _ := setupTestRepo(t)
	dbPath := filepath.Join(t.TempDir(), "monocle.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	e, err := NewEngine(DefaultConfig(), database, repo, false)
	if err != nil {
		t.Fatal(err)
	}
	session, err := e.StartSession(SessionOptions{Agent: "claude", RepoRoot: repo})
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.AddComment(CommentTarget{TargetType: types.TargetFile, TargetRef: "hello.go", LineStart: 3, StopID: "1.2"},
		types.CommentQuestion, "why?")
	if err != nil || c.StopID != "1.2" {
		t.Fatalf("comment %+v, err %v", c, err)
	}

	database2, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database2.Close()
	e2, err := NewEngine(DefaultConfig(), database2, repo, false)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := e2.ResumeSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(resumed.Comments) != 1 || resumed.Comments[0].StopID != "1.2" {
		t.Errorf("comments after restart = %+v, want the stop kept", resumed.Comments)
	}
}
