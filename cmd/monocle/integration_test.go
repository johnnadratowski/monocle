package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/adapters"
	"github.com/josephschmitt/monocle/internal/client"
	"github.com/josephschmitt/monocle/internal/core"
	"github.com/josephschmitt/monocle/internal/types"
)

// TestServeToClientE2E builds the monocle binary, spawns `monocle serve` via
// the auto-spawn helper, connects as an EngineClient, exercises a handful of
// read/write methods, then stops the server. This is the closest we can get
// to "user runs monocle" without a TTY.
func TestServeToClientE2E(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets unavailable on windows")
	}

	// Build the binary to a temp path.
	buildDir := t.TempDir()
	binary := filepath.Join(buildDir, "monocle")
	cmd := exec.Command("go", "build", "-o", binary, "./")
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// Point the spawned serve at a throwaway database. Without this it inherits
	// the environment and opens the user's real one — which means a test run can
	// migrate a live database to whatever schema the working tree is on, and a
	// binary built from an older tree then refuses to open it.
	t.Setenv("MONOCLE_DB", filepath.Join(t.TempDir(), "e2e.db"))

	// Fresh "repo" with one file.
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	// Override the socket path so we don't collide with a user's live serve.
	hash := sha256.Sum256([]byte(t.Name() + repoRoot))
	socketPath := fmt.Sprintf("/tmp/monocle-e2e-%s.sock", hex.EncodeToString(hash[:])[:10])
	_ = os.Remove(socketPath)

	got, _, err := adapters.EnsureServe(adapters.AutoSpawnOptions{
		RepoRoot:     repoRoot,
		Socket:       socketPath,
		Binary:       binary,
		ReadyTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("ensure serve: %v", err)
	}
	if got != socketPath {
		t.Errorf("socket = %q, want %q", got, socketPath)
	}
	defer func() {
		// Best-effort stop.
		stop := exec.Command(binary, "stop", "-C", repoRoot, "--socket", socketPath)
		_ = stop.Run()
	}()

	ec, err := client.NewEngineClient(socketPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer ec.Close()

	// Server already has a default session from `serve` startup.
	sess := ec.GetSession()
	if sess == nil {
		t.Fatal("expected a default session from monocle serve")
	}

	// Exercise a write path: add a comment.
	c, err := ec.AddComment(core.CommentTarget{
		TargetType: types.TargetFile,
		TargetRef:  "a.go",
		LineStart:  1,
		LineEnd:    1,
	}, types.CommentNote, "hi")
	if err != nil {
		t.Fatalf("add comment: %v", err)
	}
	if c == nil || c.Body != "hi" {
		t.Errorf("unexpected comment: %+v", c)
	}
}

// ctrl+r execs only the TUI. The serve it spawned is a separate process, and
// left alone it keeps running the old binary, silently dropping what the new
// build added. This spawns a real serve from a build reporting v2 and checks
// what a relaunch does to it.
func TestRelaunchRestartsAServeOnTheOldBuild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets unavailable on windows")
	}
	binary := filepath.Join(t.TempDir(), "monocle")
	build := exec.Command("go", "build", "-ldflags", "-X main.version=v2", "-o", binary, "./")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	t.Setenv("MONOCLE_DB", filepath.Join(t.TempDir(), "relaunch.db"))
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(t.Name() + repoRoot))
	socketPath := fmt.Sprintf("/tmp/monocle-e2e-%s.sock", hex.EncodeToString(hash[:])[:10])
	_ = os.Remove(socketPath)
	t.Cleanup(func() {
		stopServe(socketPath)
		_ = os.Remove(socketPath + ".log")
	})

	spawn := func() int {
		t.Helper()
		if _, _, err := adapters.EnsureServe(adapters.AutoSpawnOptions{
			RepoRoot: repoRoot, Socket: socketPath, Binary: binary, ReadyTimeout: 5 * time.Second,
		}); err != nil {
			t.Fatalf("ensure serve: %v", err)
		}
		pid, err := readPIDFile(pidFilePath(socketPath))
		if err != nil {
			t.Fatalf("the serve wrote no pid file: %v", err)
		}
		return pid
	}
	answers := func() bool { info, _ := serveInfo(socketPath, time.Second); return info != nil }

	pid := spawn()
	// Another TUI already relaunched onto v2: the serve is current and stays.
	restartServeForRelaunch(socketPath, "v1", "v2")
	if held, err := readPIDFile(pidFilePath(socketPath)); err != nil || held != pid || !answers() {
		t.Fatalf("a serve already on the new build was restarted (pid file %d %v, answers %v)", held, err, answers())
	}

	// This TUI is leaving v2 for v3, and the serve runs v2: it goes. The serve
	// is this test's child, so once it exits it is a zombie until the test
	// ends, as it is a relaunched TUI's: the stop must see it finish rather
	// than wait out the two seconds before its SIGKILL fallback.
	start := time.Now()
	restartServeForRelaunch(socketPath, "v2", "v3")
	if took := time.Since(start); took >= 1900*time.Millisecond {
		t.Errorf("stopping the serve took %s: it waited out the SIGKILL fallback", took)
	}
	if _, err := readPIDFile(pidFilePath(socketPath)); err == nil || answers() {
		t.Fatalf("a serve on the build being left survived the relaunch (pid file err %v, answers %v)", err, answers())
	}
	// The relaunched TUI's own EnsureServe then starts a fresh one.
	if next := spawn(); next == pid || !answers() {
		t.Errorf("after the relaunch the serve is pid %d (was %d), answers %v: want a new one", next, pid, answers())
	}
}
