package core

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/db"
	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
)

// TestHandleAddAnnotations_Validation covers the pre-storage validation and the
// per-entry echo response: valid entries are stored, invalid ones are rejected
// with reasons, and the single-line shorthand (line_end == 0) is honored.
func TestHandleAddAnnotations_Validation(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	e := &Engine{
		feedback:    NewFeedbackQueue(),
		database:    database,
		subscribers: make(map[EventKind]map[int]EventCallback),
	}
	e.current = &types.ReviewSession{
		ID:           "sess-1",
		ReviewRound:  2,
		FileStatuses: make(map[string]bool),
		ChangedFiles: []types.ChangedFile{{Path: "main.go", Status: types.FileModified}},
	}
	if err := database.CreateSession(e.current); err != nil {
		t.Fatalf("create session: %v", err)
	}

	msg := &protocol.AddAnnotationsMsg{
		Type: protocol.TypeAddAnnotations,
		Entries: []protocol.AnnotationEntry{
			{File: "main.go", LineStart: 3, LineEnd: 5, Summary: "valid range",
				Refs: []types.DocRef{{Kind: types.DocRefArtifact, Doc: "no-such-artifact", Label: "missing"}}},
			{File: "main.go", LineStart: 7, LineEnd: 0, Summary: "single line shorthand"},
			{File: "main.go", LineStart: 10, LineEnd: 5, Summary: "bad range"},
			{File: "other.go", LineStart: 1, LineEnd: 1, Summary: "not a changed file"},
			{File: "main.go", LineStart: 1, LineEnd: 1, Summary: "   "},
		},
	}

	resp := e.handleAddAnnotations(msg)
	if !resp.Success {
		t.Fatalf("expected success, got message %q", resp.Message)
	}
	if resp.Count != 2 {
		t.Fatalf("expected 2 accepted, got %d", resp.Count)
	}
	if len(resp.Rejected) != 3 {
		t.Fatalf("expected 3 rejected, got %d (%+v)", len(resp.Rejected), resp.Rejected)
	}

	reasons := strings.Join([]string{resp.Rejected[0].Reason, resp.Rejected[1].Reason, resp.Rejected[2].Reason}, "\n")
	for _, want := range []string{"line_end (5) must be >= line_start (10)", "changed files", "summary is required"} {
		if !strings.Contains(reasons, want) {
			t.Errorf("rejection reasons missing %q; got:\n%s", want, reasons)
		}
	}

	// The unresolvable artifact ref is a warning (entry still stored), not a rejection.
	if len(resp.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d (%v)", len(resp.Warnings), resp.Warnings)
	}
	if !strings.Contains(resp.Warnings[0], "could not be resolved") {
		t.Errorf("warning missing resolution hint: %q", resp.Warnings[0])
	}

	// Stored annotations carry the current round and the resolved (shorthand) range.
	stored, err := database.GetAnnotations("sess-1")
	if err != nil {
		t.Fatalf("GetAnnotations: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("expected 2 stored annotations, got %d", len(stored))
	}
	for _, a := range stored {
		if a.ReviewRound != 2 {
			t.Errorf("annotation round = %d, want 2", a.ReviewRound)
		}
		if a.LineStart == 7 && a.LineEnd != 7 {
			t.Errorf("single-line shorthand: line_end = %d, want 7", a.LineEnd)
		}
	}
}

// TestCompleteQueuedDelivery_ClearsAnnotations verifies annotations are wiped
// when the reviewer's feedback is consumed and the round advances, so each round
// starts clean and the agent re-annotates against the revised code.
func TestCompleteQueuedDelivery_ClearsAnnotations(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	e := &Engine{
		feedback:    NewFeedbackQueue(),
		database:    database,
		sessions:    NewSessionManager(database, nil),
		subscribers: make(map[EventKind]map[int]EventCallback),
	}
	e.current = &types.ReviewSession{
		ID:           "sess-1",
		ReviewRound:  1,
		FileStatuses: make(map[string]bool),
		ChangedFiles: []types.ChangedFile{{Path: "main.go", Status: types.FileModified}},
	}
	if err := database.CreateSession(e.current); err != nil {
		t.Fatalf("create session: %v", err)
	}

	anns := []types.Annotation{{TargetRef: "main.go", LineStart: 1, LineEnd: 2, Summary: "note", ReviewRound: 1}}
	if err := database.SetAnnotations("sess-1", anns, false); err != nil {
		t.Fatalf("SetAnnotations: %v", err)
	}
	reloaded, _ := database.GetAnnotations("sess-1")
	e.current.Annotations = reloaded

	e.completeQueuedDelivery()

	if e.current.ReviewRound != 2 {
		t.Errorf("round = %d, want 2 (advanced)", e.current.ReviewRound)
	}
	if len(e.current.Annotations) != 0 {
		t.Errorf("expected in-memory annotations cleared, got %d", len(e.current.Annotations))
	}
	left, err := database.GetAnnotations("sess-1")
	if err != nil {
		t.Fatalf("GetAnnotations: %v", err)
	}
	if len(left) != 0 {
		t.Errorf("expected DB annotations cleared, got %d", len(left))
	}
}
