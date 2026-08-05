package client_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/client"
	"github.com/josephschmitt/monocle/internal/core"
	"github.com/josephschmitt/monocle/internal/db"
	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
)

func setupTestEngine(t *testing.T) (*core.Engine, string) {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	tmpDir := t.TempDir()
	cfg := core.DefaultConfig()
	engine, err := core.NewEngine(cfg, database, tmpDir, true /* nonGitMode */)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	_, err = engine.StartSession(core.SessionOptions{
		Agent:    "test",
		RepoRoot: tmpDir,
	})
	if err != nil {
		t.Fatalf("start session: %v", err)
	}

	hash := sha256.Sum256([]byte(t.Name()))
	socketPath := fmt.Sprintf("/tmp/monocle-test-%s.sock", hex.EncodeToString(hash[:])[:8])
	if err := engine.StartServer(socketPath); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { engine.Shutdown() })

	return engine, socketPath
}

func TestClient_ReviewStatus(t *testing.T) {
	_, socketPath := setupTestEngine(t)

	c, err := client.Connect(socketPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	msg := &protocol.GetReviewStatusMsg{Type: protocol.TypeGetReviewStatus}
	resp, err := c.Request(msg, client.DefaultTimeout)
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	status, ok := resp.(*protocol.GetReviewStatusResponse)
	if !ok {
		t.Fatalf("expected *GetReviewStatusResponse, got %T", resp)
	}
	if status.Status != "no_feedback" {
		t.Errorf("status = %q, want %q", status.Status, "no_feedback")
	}
}

func TestClient_PollFeedback_NoWait(t *testing.T) {
	_, socketPath := setupTestEngine(t)

	c, err := client.Connect(socketPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	msg := &protocol.PollFeedbackMsg{Type: protocol.TypePollFeedback, Wait: false}
	resp, err := c.Request(msg, client.DefaultTimeout)
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	feedback, ok := resp.(*protocol.PollFeedbackResponse)
	if !ok {
		t.Fatalf("expected *PollFeedbackResponse, got %T", resp)
	}
	if feedback.HasFeedback {
		t.Error("expected no feedback")
	}
}

// TestClient_AbortedWaitPreservesVerdict is the regression for the silent
// verdict-loss bug: an agent that starts get_feedback --wait and then aborts the
// call (ctx cancel) must not cause a later reviewer submission to be consumed
// into the abandoned response and marked delivered. RequestWithContext closes
// the connection on cancel, which releases the engine's blocking wait WITHOUT
// consuming — so the verdict stays queued for the next poll.
func TestClient_AbortedWaitPreservesVerdict(t *testing.T) {
	engine, socketPath := setupTestEngine(t)

	cA, err := client.Connect(socketPath)
	if err != nil {
		t.Fatalf("connect A: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	waitDone := make(chan struct{})
	var waitErr error
	go func() {
		defer close(waitDone)
		_, waitErr = cA.RequestWithContext(ctx, &protocol.PollFeedbackMsg{
			Type: protocol.TypePollFeedback,
			Wait: true,
		})
	}()

	// Let the server enter the blocking wait, then abort it (the agent times out).
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case <-waitDone:
		if waitErr == nil {
			t.Error("expected an error from the aborted wait, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("aborted wait did not return promptly on ctx cancel")
	}
	cA.Close()

	// Give the engine time to observe the disconnect and release the wait
	// without consuming (watchConnClose -> cancel).
	time.Sleep(150 * time.Millisecond)

	// The reviewer submits AFTER the wait was abandoned.
	if err := engine.Submit(types.ActionRequestChanges, "please fix"); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// A fresh poll must still find the verdict — it must not have been consumed
	// by the aborted wait.
	cB, err := client.Connect(socketPath)
	if err != nil {
		t.Fatalf("connect B: %v", err)
	}
	defer cB.Close()

	resp, err := cB.Request(&protocol.PollFeedbackMsg{
		Type: protocol.TypePollFeedback,
		Wait: false,
	}, client.DefaultTimeout)
	if err != nil {
		t.Fatalf("poll B: %v", err)
	}
	pfr, ok := resp.(*protocol.PollFeedbackResponse)
	if !ok {
		t.Fatalf("expected *PollFeedbackResponse, got %T", resp)
	}
	if !pfr.HasFeedback {
		t.Fatal("verdict was lost: the aborted wait consumed the feedback")
	}
}

func TestClient_SubmitContent(t *testing.T) {
	_, socketPath := setupTestEngine(t)

	c, err := client.Connect(socketPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	msg := &protocol.SubmitContentMsg{
		Type:        protocol.TypeSubmitContent,
		ID:          "test-plan",
		Title:       "Test Plan",
		Content:     "# My Plan\n\nDo the thing.",
		ContentType: "md",
		IsPlan:      true,
	}
	resp, err := c.Request(msg, client.DefaultTimeout)
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	submit, ok := resp.(*protocol.SubmitContentResponse)
	if !ok {
		t.Fatalf("expected *SubmitContentResponse, got %T", resp)
	}
	if !submit.Success {
		t.Errorf("expected success, got message: %s", submit.Message)
	}
}

func TestClient_AddFiles(t *testing.T) {
	_, socketPath := setupTestEngine(t)

	c, err := client.Connect(socketPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	msg := &protocol.AddAdditionalFilesMsg{
		Type:  protocol.TypeAddAdditionalFiles,
		Paths: []string{t.TempDir()},
	}
	resp, err := c.Request(msg, client.DefaultTimeout)
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	add, ok := resp.(*protocol.AddAdditionalFilesResponse)
	if !ok {
		t.Fatalf("expected *AddAdditionalFilesResponse, got %T", resp)
	}
	if !add.Success {
		t.Errorf("expected success, got message: %s", add.Message)
	}
}

func TestClient_ErrNotRunning(t *testing.T) {
	_, err := client.Connect("/tmp/monocle-does-not-exist.sock")
	if err != client.ErrNotRunning {
		t.Errorf("expected ErrNotRunning, got %v", err)
	}
}
