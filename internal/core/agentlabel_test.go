package core

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/db"
	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
)

func setLabel(e *Engine, label string) *protocol.SetAgentLabelResponse {
	return e.handleSetAgentLabel(&protocol.SetAgentLabelMsg{
		Type: protocol.TypeSetAgentLabel, Label: label,
	})
}

func TestAgentLabel(t *testing.T) {
	t.Run("is stored and reported", func(t *testing.T) {
		e, _ := summaryEngine(t)
		r := setLabel(e, "ott (2)")
		if !r.Success || r.Label != "ott (2)" {
			t.Fatalf("response = %+v", r)
		}
		if e.current.AgentLabel != "ott (2)" {
			t.Errorf("label = %q", e.current.AgentLabel)
		}
	})

	t.Run("is capped", func(t *testing.T) {
		e, _ := summaryEngine(t)
		r := setLabel(e, strings.Repeat("long ", 40))
		if len([]rune(r.Label)) != agentLabelLimit {
			t.Errorf("label is %d runes, want %d", len([]rune(r.Label)), agentLabelLimit)
		}
	})

	t.Run("an empty label clears it", func(t *testing.T) {
		e, _ := summaryEngine(t)
		setLabel(e, "ott (2)")
		r := setLabel(e, "")
		if !r.Success || r.Label != "" {
			t.Fatalf("response = %+v", r)
		}
		if e.current.AgentLabel != "" {
			t.Errorf("label = %q, want cleared", e.current.AgentLabel)
		}
	})

	// The label names the PANE. A review ending does not rename the window, so
	// approving — which clears the review name, the base and the summary — must
	// leave it standing.
	t.Run("survives an approval", func(t *testing.T) {
		e, _ := summaryEngine(t)
		setLabel(e, "ott (2)")
		e.mu.Lock()
		e.current.ChangedFiles = []types.ChangedFile{{Path: "hello.go"}}
		e.current.ReviewName = "some review"
		e.mu.Unlock()

		if err := e.Submit(types.ActionApprove, ""); err != nil {
			t.Fatalf("submit: %v", err)
		}
		if e.current.ReviewName != "" {
			t.Fatal("precondition: approve should have cleared the review name")
		}
		if e.current.AgentLabel != "ott (2)" {
			t.Errorf("label = %q after approve, want it kept", e.current.AgentLabel)
		}
	})

	t.Run("survives a restart", func(t *testing.T) {
		e, dbPath := summaryEngine(t)
		repo := e.current.RepoRoot
		sessionID := e.current.ID
		setLabel(e, "ott (2)")

		database, err := db.Open(dbPath)
		if err != nil {
			t.Fatalf("reopen db: %v", err)
		}
		defer database.Close()
		e2, err := NewEngine(DefaultConfig(), database, repo, false)
		if err != nil {
			t.Fatalf("new engine: %v", err)
		}
		resumed, err := e2.ResumeSession(sessionID)
		if err != nil {
			t.Fatalf("resume: %v", err)
		}
		if resumed.AgentLabel != "ott (2)" {
			t.Errorf("label after restart = %q", resumed.AgentLabel)
		}
	})

	// Labelling is not putting work in front of the reviewer, so it must not
	// restart the "sent N ago" clock the top bar shows.
	t.Run("does not count as a review send", func(t *testing.T) {
		if isReviewSend(&protocol.SetAgentLabelMsg{}) {
			t.Error("a label is not a handover")
		}
	})
}
