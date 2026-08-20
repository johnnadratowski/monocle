package core

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func stateEngine(t *testing.T, session *types.ReviewSession) *Engine {
	t.Helper()
	e := &Engine{feedback: NewFeedbackQueue(), subscribers: make(map[EventKind]map[int]EventCallback)}
	e.cfg.Store(DefaultConfig())
	e.current = session
	return e
}

func TestReviewState(t *testing.T) {
	staged := func() *types.ReviewSession {
		return &types.ReviewSession{
			RepoRoot: "/repo", ReviewName: "Auth rework", ReviewRound: 3,
			ChangedFiles: []types.ChangedFile{
				{Path: "a.go", Reviewed: false},
				{Path: "b.go", Reviewed: true},
			},
			ContentItems: []types.ContentItem{{ID: "plan", Reviewed: false}},
		}
	}

	t.Run("staged and unreviewed is waiting", func(t *testing.T) {
		info := stateEngine(t, staged()).GetReviewStatusInfo()
		if info.ReviewState != ReviewStateWaiting {
			t.Errorf("state = %q, want %q", info.ReviewState, ReviewStateWaiting)
		}
		if info.Round != 3 || info.Files != 2 || info.FilesUnreviewed != 1 || info.Artifacts != 1 {
			t.Errorf("counts wrong: %+v", info)
		}
		if info.ReviewName != "Auth rework" {
			t.Errorf("review name = %q", info.ReviewName)
		}
	})

	t.Run("an empty engine is none, not waiting", func(t *testing.T) {
		info := stateEngine(t, &types.ReviewSession{RepoRoot: "/repo"}).GetReviewStatusInfo()
		if info.ReviewState != ReviewStateNone {
			t.Errorf("state = %q, want %q", info.ReviewState, ReviewStateNone)
		}
	})

	// Everything signed off: the human is done and nothing awaits them, even
	// though the diff is still staged.
	t.Run("all reviewed is none", func(t *testing.T) {
		s := staged()
		for i := range s.ChangedFiles {
			s.ChangedFiles[i].Reviewed = true
		}
		s.ContentItems[0].Reviewed = true
		if got := stateEngine(t, s).GetReviewStatusInfo().ReviewState; got != ReviewStateNone {
			t.Errorf("state = %q, want %q", got, ReviewStateNone)
		}
	})

	// Between a submit and the agent collecting, neither side waits on the
	// other — reporting "waiting" would put the human back in a queue they
	// already cleared.
	t.Run("submitted-but-uncollected is not waiting", func(t *testing.T) {
		e := stateEngine(t, staged())
		e.feedback.Submit(&FormattedReview{Formatted: "## Review", Action: "request_changes"}, false)
		info := e.GetReviewStatusInfo()
		if info.ReviewState != ReviewStateNone {
			t.Errorf("state = %q, want %q", info.ReviewState, ReviewStateNone)
		}
		if !info.FeedbackQueued {
			t.Error("FeedbackQueued should say the verdict is waiting on the agent")
		}
	})

	t.Run("added context files alone do not mean waiting", func(t *testing.T) {
		s := &types.ReviewSession{
			RepoRoot:        "/repo",
			AdditionalFiles: []types.AdditionalFile{{Path: "iface.go"}},
		}
		info := stateEngine(t, s).GetReviewStatusInfo()
		if info.ReviewState != ReviewStateNone {
			t.Errorf("reference material is not work to sign off; state = %q", info.ReviewState)
		}
		if info.AddedFiles != 1 {
			t.Errorf("added files should still be counted, got %d", info.AddedFiles)
		}
	})

	// With tracking off nothing is ever marked reviewed, so the precise test
	// would answer "waiting" forever. The coarse answer is flagged as such.
	t.Run("tracking disabled reports the coarse answer and says so", func(t *testing.T) {
		e := stateEngine(t, staged())
		cfg := DefaultConfig()
		cfg.ReviewTracking = false
		e.cfg.Store(cfg)
		info := e.GetReviewStatusInfo()
		if info.ReviewState != ReviewStateWaiting {
			t.Errorf("state = %q, want %q", info.ReviewState, ReviewStateWaiting)
		}
		if info.ReviewTracking {
			t.Error("ReviewTracking should report that the answer is coarse")
		}
	})

	// The hard constraint from the request: asking must never change what a
	// later get_feedback returns.
	t.Run("asking repeatedly never drains the queue", func(t *testing.T) {
		e := stateEngine(t, staged())
		e.feedback.Submit(&FormattedReview{Formatted: "## Review — Changes Requested"}, false)
		for i := 0; i < 25; i++ {
			e.GetReviewStatusInfo()
		}
		got := e.PollFeedback()
		if got == nil {
			t.Fatal("polling status consumed the verdict")
		}
		if !strings.Contains(got.Formatted, "Changes Requested") {
			t.Errorf("verdict came back altered: %q", got.Formatted)
		}
	})
}
