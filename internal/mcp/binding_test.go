package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/josephschmitt/monocle/internal/adapters"
)

func TestResolveRepoBinding_FindsRepoRootFromSubdir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "internal", "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	repoRoot, socket, err := resolveRepoBinding(sub)
	if err != nil {
		t.Fatalf("resolveRepoBinding: %v", err)
	}
	if repoRoot != dir {
		t.Errorf("repoRoot = %q, want %q", repoRoot, dir)
	}
	if want := adapters.DefaultSocketPath(dir); socket != want {
		t.Errorf("socket = %q, want %q", socket, want)
	}
}

func TestResolveRepoBinding_Errors(t *testing.T) {
	if _, _, err := resolveRepoBinding(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("expected error for missing path")
	}

	f := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveRepoBinding(f); err == nil {
		t.Error("expected error for non-directory path")
	}
}

func TestRequireBoundErr_StrictMode(t *testing.T) {
	prevEnv, hadEnv := os.LookupEnv("MONOCLE_REQUIRE_SET_REPO")
	prevBound := repoBound.Load()
	t.Cleanup(func() {
		if hadEnv {
			_ = os.Setenv("MONOCLE_REQUIRE_SET_REPO", prevEnv)
		} else {
			_ = os.Unsetenv("MONOCLE_REQUIRE_SET_REPO")
		}
		repoBound.Store(prevBound)
	})

	// Default (strict off): never guards, even unbound — preserves the
	// zero-config single-repo flow.
	_ = os.Unsetenv("MONOCLE_REQUIRE_SET_REPO")
	repoBound.Store(false)
	if requireBoundErr() != nil {
		t.Error("strict off + unbound should not error")
	}

	// Strict on + unbound: hard error.
	_ = os.Setenv("MONOCLE_REQUIRE_SET_REPO", "1")
	repoBound.Store(false)
	if requireBoundErr() == nil {
		t.Error("strict on + unbound should error")
	}

	// Strict on + bound: allowed.
	repoBound.Store(true)
	if requireBoundErr() != nil {
		t.Error("strict on + bound should not error")
	}
}

func TestBindSocket_SetsEnvAndSignalsRebind(t *testing.T) {
	prev, had := os.LookupEnv("MONOCLE_SOCKET")
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("MONOCLE_SOCKET", prev)
		} else {
			_ = os.Unsetenv("MONOCLE_SOCKET")
		}
	})

	// Drain any pending signal so the assertion is deterministic.
	select {
	case <-rebindSignal:
	default:
	}

	bindSocket("/tmp/monocle-binding-test.sock")

	if got := os.Getenv("MONOCLE_SOCKET"); got != "/tmp/monocle-binding-test.sock" {
		t.Errorf("MONOCLE_SOCKET = %q, want /tmp/monocle-binding-test.sock", got)
	}
	select {
	case <-rebindSignal:
		// good — the channel listener will pick this up and re-dial.
	default:
		t.Error("expected rebindSignal to be pulsed")
	}
}
