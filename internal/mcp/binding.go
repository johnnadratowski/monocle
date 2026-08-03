package mcp

import (
	"fmt"
	"os"
	"sync/atomic"

	"github.com/josephschmitt/monocle/internal/adapters"
	"github.com/josephschmitt/monocle/internal/client"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// repoBound records whether set_repo has successfully bound this process to a
// repo. It gates strict mode (see requireBoundErr). A serve-mcp process serves
// exactly one agent (stdio, single session), so this per-process flag is
// per-agent — there is no cross-agent sharing to race on.
var repoBound atomic.Bool

// rebindSignal is pulsed whenever set_repo re-points the MCP server at a
// different engine socket. The channel listener (engine.go) selects on it to
// drop its current connection and re-dial the new socket, so push
// notifications follow the rebind instead of streaming from the launch-time
// engine. Buffered depth 1 with a non-blocking send: a signal delivered while
// nothing is listening is harmless because the listener re-resolves the socket
// on its next reconnect anyway.
var rebindSignal = make(chan struct{}, 1)

// resolveRepoBinding turns an arbitrary path (typically an agent's worktree)
// into the repo root that owns it and the engine socket for that repo. An empty
// path means "use the current working directory". It neither spawns nor connects
// — callers do that — so it stays pure and unit-testable.
func resolveRepoBinding(path string) (repoRoot, socket string, err error) {
	if path == "" {
		cwd, cerr := os.Getwd()
		if cerr != nil {
			return "", "", fmt.Errorf("no path given and cwd unavailable: %w", cerr)
		}
		path = cwd
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", "", fmt.Errorf("repo path: %w", err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("repo path %q is not a directory", path)
	}
	repoRoot = adapters.FindRepoRoot(path)
	return repoRoot, adapters.DefaultSocketPath(repoRoot), nil
}

// bindSocket points every subsequent tool call — and, via rebindSignal, the
// channel listener — at socket by setting MONOCLE_SOCKET, the same variable the
// startup roots resolver (resolveSocketFromRoots) uses. Tool handlers re-read it
// through client.ConnectDefault on every call, so no handler needs to change.
func bindSocket(socket string) {
	_ = os.Setenv("MONOCLE_SOCKET", socket)
	repoBound.Store(true)
	select {
	case rebindSignal <- struct{}{}:
	default:
	}
}

// strictBinding reports whether the operator requires an explicit set_repo
// before any review tool will act (MONOCLE_REQUIRE_SET_REPO set to a truthy
// value). Fleet staffing enables it so a teammate that forgets to bind gets a
// hard error instead of silently reading/writing the launch directory's engine
// — the exact silent-wrong-answer that made a cross-lane misbinding expensive
// to diagnose. It stays OFF by default so the single-repo, zero-config flow
// (where the launch-directory binding is correct) is unaffected.
func strictBinding() bool {
	switch os.Getenv("MONOCLE_REQUIRE_SET_REPO") {
	case "", "0", "false", "no":
		return false
	default:
		return true
	}
}

// requireBoundErr returns a non-nil error result when strict mode is on and no
// repo has been bound yet, and nil otherwise. Handlers call it (via boundClient)
// before touching the engine.
func requireBoundErr() *sdkmcp.CallToolResult {
	if repoBound.Load() || !strictBinding() {
		return nil
	}
	return errResult("no repo bound: call set_repo with your worktree path before any other review tool (strict binding is enabled via MONOCLE_REQUIRE_SET_REPO)")
}

// boundClient enforces the strict-mode bind guard, then connects to the bound
// engine. It centralizes the connect-and-error pattern every review tool shares:
// a non-nil result means "return this to the caller now" (either the guard error
// or a connect failure); otherwise the client is ready and the caller must Close it.
func boundClient() (*client.Client, *sdkmcp.CallToolResult) {
	if r := requireBoundErr(); r != nil {
		return nil, r
	}
	c, err := client.ConnectDefault()
	if err != nil {
		return nil, errResult("connect: %v", err)
	}
	return c, nil
}
