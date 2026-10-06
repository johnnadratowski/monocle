package client

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/core"
	"github.com/josephschmitt/monocle/internal/db"
	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
)

// setupEngine spins up a non-git-mode engine against a temp directory with
// one file, starts its socket server, and returns the live engine + socket
// path. Cleanup is registered via t.Cleanup.
func setupEngine(t *testing.T) (*core.Engine, string) {
	t.Helper()

	repoRoot := t.TempDir()
	// Seed one file so GetChangedFiles returns something deterministic.
	if err := os.WriteFile(filepath.Join(repoRoot, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	engine, err := core.NewEngine(core.DefaultConfig(), database, repoRoot, true /* nonGitMode */)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	if _, err := engine.StartSession(core.SessionOptions{
		Agent:    "test",
		RepoRoot: repoRoot,
	}); err != nil {
		t.Fatalf("start session: %v", err)
	}

	hash := sha256.Sum256([]byte(t.Name()))
	socketPath := fmt.Sprintf("/tmp/monocle-client-test-%s.sock", hex.EncodeToString(hash[:])[:10])
	if err := engine.StartServer(socketPath); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { engine.Shutdown() })

	return engine, socketPath
}

func TestEngineClient_RoundTrip(t *testing.T) {
	engine, socketPath := setupEngine(t)

	ec, err := NewEngineClient(socketPath)
	if err != nil {
		t.Fatalf("new engine client: %v", err)
	}
	t.Cleanup(func() { ec.Close() })

	// GetSession — session started in setupEngine.
	if sess := ec.GetSession(); sess == nil {
		t.Error("GetSession returned nil")
	}

	// RefreshChangedFiles — should find the seeded a.go.
	files, err := ec.RefreshChangedFiles()
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(files) == 0 {
		t.Error("expected at least one changed file")
	}

	// Comment round-trip.
	c, err := ec.AddComment(core.CommentTarget{
		TargetType: types.TargetFile,
		TargetRef:  "a.go",
		LineStart:  1,
		LineEnd:    1,
	}, types.CommentIssue, "needs a docstring")
	if err != nil {
		t.Fatalf("add comment: %v", err)
	}
	if c == nil || c.Body != "needs a docstring" {
		t.Errorf("unexpected comment: %+v", c)
	}

	edited, err := ec.EditComment(c.ID, types.CommentSuggestion, "add a docstring")
	if err != nil {
		t.Fatalf("edit comment: %v", err)
	}
	if edited.Body != "add a docstring" {
		t.Errorf("edit did not apply: %q", edited.Body)
	}

	if err := ec.ResolveComment(c.ID); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// Server-side state via engine matches the wire result.
	sess := engine.GetSession()
	if sess == nil || len(sess.Comments) != 1 {
		t.Errorf("expected 1 comment on engine session, got %d", len(sess.Comments))
	}

	// Marking flows.
	if err := ec.MarkReviewed("a.go"); err != nil {
		t.Fatalf("mark reviewed: %v", err)
	}
	if err := ec.UnmarkReviewed("a.go"); err != nil {
		t.Fatalf("unmark: %v", err)
	}

	// Config round-trip. GetConfig returns a pointer cached on the client.
	cfg := ec.GetConfig()
	if cfg == nil {
		t.Fatal("GetConfig returned nil")
	}
	origWrap := cfg.Wrap
	cfg.Wrap = !origWrap
	if err := ec.SaveConfig(); err != nil {
		t.Fatalf("save config: %v", err)
	}
	// Server engine saw the mutated value.
	if engine.GetConfig().Wrap == origWrap {
		t.Errorf("SaveConfig did not propagate mutation: got Wrap=%v, want %v", engine.GetConfig().Wrap, !origWrap)
	}
	// ...and wrote it to the throwaway config dir TestMain set up. This save
	// used to land in the user's real ~/.config/monocle/config.json.
	saved := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "monocle", "config.json")
	if !strings.HasPrefix(saved, os.TempDir()) {
		t.Fatalf("config dir %q is not a temp dir: this test would write the user's real config", saved)
	}
	if _, err := os.Stat(saved); err != nil {
		t.Errorf("SaveConfig did not write the isolated config: %v", err)
	}

	// Status queries.
	if path := ec.GetSocketPath(); path != socketPath {
		t.Errorf("GetSocketPath = %q, want %q", path, socketPath)
	}
	// EngineClient subscribes passively (TUI is a viewer, not an agent),
	// so it must NOT bump the agent-facing subscriber count — otherwise
	// Submit() would flip into push mode and the real agent would lose
	// the feedback.
	if count := ec.GetSubscriberCount(); count != 0 {
		t.Errorf("GetSubscriberCount = %d, want 0 (passive subscribe must not count)", count)
	}
}

func TestEngineClient_EventBus(t *testing.T) {
	engine, socketPath := setupEngine(t)
	_ = engine

	// Seed a second file to add as an additional-path (triggers
	// EventAdditionalFileAdded from the server).
	repoRoot := engine.GetSession().RepoRoot
	extra := filepath.Join(repoRoot, "extra.go")
	if err := os.WriteFile(extra, []byte("package a\n"), 0o644); err != nil {
		t.Fatalf("seed extra: %v", err)
	}

	ec, err := NewEngineClient(socketPath)
	if err != nil {
		t.Fatalf("new engine client: %v", err)
	}
	t.Cleanup(func() { ec.Close() })

	events := make(chan core.EventPayload, 4)
	unsub := ec.On(core.EventAdditionalFileAdded, func(p core.EventPayload) {
		events <- p
	})

	if _, err := ec.AddAdditionalPaths([]string{extra}); err != nil {
		t.Fatalf("add additional paths: %v", err)
	}

	select {
	case p := <-events:
		if p.Kind != core.EventAdditionalFileAdded {
			t.Errorf("wrong event kind: %v", p.Kind)
		}
		if p.Path != extra {
			t.Errorf("event path = %q, want %q", p.Path, extra)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event not dispatched to client")
	}

	// Unsubscribing stops delivery for further events.
	unsub()
	extra2 := filepath.Join(repoRoot, "extra2.go")
	if err := os.WriteFile(extra2, []byte("package a\n"), 0o644); err != nil {
		t.Fatalf("seed extra2: %v", err)
	}
	if _, err := ec.AddAdditionalPaths([]string{extra2}); err != nil {
		t.Fatalf("add additional paths 2: %v", err)
	}
	select {
	case p := <-events:
		t.Errorf("received event after unsub: %+v", p)
	case <-time.After(200 * time.Millisecond):
		// expected
	}
}

// TestEngineClient_SurvivesForcedRedial reproduces the redial race: tear
// down the underlying connection via the daemon (Shutdown + restart of
// the server), force the client to redial on the next request, then
// confirm that any lagging readLoop teardown from the dead connection
// can't kill the freshly-dialed one. Pre-fix this test would
// intermittently fail with "client closed" because the stale readLoop
// closed the new clientConn's closed channel via the shared closedOnce.
func TestEngineClient_SurvivesForcedRedial(t *testing.T) {
	engine, socketPath := setupEngine(t)

	ec, err := NewEngineClient(socketPath)
	if err != nil {
		t.Fatalf("new engine client: %v", err)
	}
	t.Cleanup(func() { ec.Close() })

	// Sanity: first request works.
	if _, err := ec.RefreshChangedFiles(); err != nil {
		t.Fatalf("initial refresh: %v", err)
	}

	// Forcibly kill the active connection from the daemon side. The
	// client's readLoop will exit and signal redial on next request.
	engine.Shutdown()
	// Bring the server back on the same socket so the redial can succeed.
	if err := engine.StartServer(socketPath); err != nil {
		t.Fatalf("restart server: %v", err)
	}

	// Give the kernel a beat to notify the dead conn so readLoop reaches
	// its closeConn() — this is the window in which the racy old code
	// would kill the next fresh connection.
	time.Sleep(50 * time.Millisecond)

	// Run several requests back-to-back. With the bug, at least one would
	// return "client closed" / "timeout waiting for response" because the
	// stale readLoop closed the fresh closed channel and socket.
	for i := 0; i < 10; i++ {
		if _, err := ec.RefreshChangedFiles(); err != nil {
			t.Fatalf("post-redial refresh %d: %v", i, err)
		}
	}
}

// A setting added to config.json while the TUI runs reaches it: the engine
// re-reads the changed file, and the client asks again in the background.
func TestEngineClient_SeesAChangedConfigFile(t *testing.T) {
	_, socketPath := setupEngine(t)
	ec, err := NewEngineClient(socketPath)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer ec.Close()
	if got := ec.GetConfig().WalkthroughResolve; got != "" {
		t.Fatalf("walkthrough_resolve %q before it is set", got)
	}
	path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "monocle", "config.json")
	if !strings.HasPrefix(path, os.TempDir()) {
		t.Fatalf("config dir %q is not a temp dir", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"walkthrough_resolve": "resolve-refs"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	deadline := time.Now().Add(5 * time.Second)
	for ec.GetConfig().WalkthroughResolve != "resolve-refs" {
		if time.Now().After(deadline) {
			t.Fatal("the client never saw walkthrough_resolve set in config.json")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// A config fetched in the background is never swapped in under an edit in
// progress, and one fetched before a save is never handed out after it.
func TestEngineClient_BackgroundFetchKeepsEdits(t *testing.T) {
	engine, socketPath := setupEngine(t)
	ec, err := NewEngineClient(socketPath)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer ec.Close()

	cfg := ec.GetConfig()
	cfg.Theme = "light"
	ec.refreshConfig(ec.cfgGen) // a fetch lands between the edit and the save
	if err := ec.SaveConfig(); err != nil {
		t.Fatal(err)
	}
	if got := engine.GetConfig().Theme; got != "light" {
		t.Errorf("the engine has theme %q: the edit was dropped", got)
	}
	if got := ec.GetConfig(); got != cfg || got.Theme != "light" {
		t.Errorf("after the save the client hands out theme %q: the fetch from before the save replaced it", got.Theme)
	}

	gen := ec.cfgGen
	cfg.Theme = "dark"
	if err := ec.SaveConfig(); err != nil {
		t.Fatal(err)
	}
	ec.refreshConfig(gen) // began before that save
	if got := ec.GetConfig(); got != cfg {
		t.Error("a fetch begun before a save was handed out after it")
	}
}

// goto_line's event reaches a client with its path and line.
func TestEngineClient_GotoLineEvent(t *testing.T) {
	engine, socketPath := setupEngine(t)
	ec, err := NewEngineClient(socketPath)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer ec.Close()
	events := make(chan core.EventPayload, 1)
	ec.On(core.EventGotoLine, func(p core.EventPayload) { events <- p })
	_ = engine
	// The agent's side: a plain request, as the CLI and the MCP tool send it.
	c, err := Connect(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	resp, err := c.Request(&protocol.GotoLineMsg{Type: protocol.TypeGotoLine, Path: "a.go", Line: 7}, time.Second)
	if r, ok := resp.(*protocol.GotoLineResponse); err != nil || !ok || !r.Success {
		t.Fatalf("goto_line: %+v %v", resp, err)
	}
	select {
	case p := <-events:
		if p.Path != "a.go" || p.Line != 7 {
			t.Errorf("event %+v, want a.go:7", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the goto_line event never reached the client")
	}
}
