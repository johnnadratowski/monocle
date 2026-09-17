package adapters

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// shortTempDir keeps socket paths under the ~104-byte sun_path limit, which a
// shortTempDir(t) path named after a subtest quietly exceeds.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ms")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// leftoverFile stands in for a socket file whose server is gone. Closing a Go
// unix listener unlinks its file, so a dead engine has to be simulated by
// putting a non-listening file back at the path — which is exactly what a
// crashed (rather than cleanly stopped) engine leaves behind.
func leftoverFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("leftover file: %v", err)
	}
}

// listenOn starts a unix listener that accepts and immediately closes.
func listenOn(t *testing.T, path string) net.Listener {
	t.Helper()
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	time.Sleep(10 * time.Millisecond)
	return l
}

func TestProbeSocket(t *testing.T) {
	t.Run("no file is stale", func(t *testing.T) {
		if got := probeSocket(filepath.Join(shortTempDir(t), "nope.sock")); got != socketStale {
			t.Errorf("state = %v, want stale", got)
		}
	})

	t.Run("a listener is listening", func(t *testing.T) {
		path := filepath.Join(shortTempDir(t), "live.sock")
		listenOn(t, path)
		if got := probeSocket(path); got != socketListening {
			t.Errorf("state = %v, want listening", got)
		}
	})

	// A crashed engine leaves the file behind with no pid file, or a pid file
	// naming a process that is gone. Nothing holds it, so it is safe to replace.
	t.Run("an abandoned socket file is stale", func(t *testing.T) {
		dir := shortTempDir(t)
		path := filepath.Join(dir, "dead.sock")
		leftoverFile(t, path)
		if got := probeSocket(path); got != socketStale {
			t.Errorf("state = %v, want stale", got)
		}
	})

	t.Run("a dead pid does not rescue a stale socket", func(t *testing.T) {
		dir := shortTempDir(t)
		path := filepath.Join(dir, "dead.sock")
		leftoverFile(t, path)
		// A pid that cannot be alive.
		os.WriteFile(PIDFilePath(path), []byte("999999"), 0o644)
		if got := probeSocket(path); got != socketStale {
			t.Errorf("state = %v, want stale", got)
		}
	})

	// The case that caused two engines for one repo: the socket exists, its
	// owner is alive, but nothing answered the probe. Reporting "stale" there
	// hands the path to a second engine while the first is still serving the
	// reviewer's frontend.
	t.Run("an unanswering socket whose owner lives is busy", func(t *testing.T) {
		dir := shortTempDir(t)
		path := filepath.Join(dir, "busy.sock")
		leftoverFile(t, path) // present, not answering
		os.WriteFile(PIDFilePath(path), []byte(strconv.Itoa(os.Getpid())), 0o644)
		if got := probeSocket(path); got != socketBusy {
			t.Errorf("state = %v, want busy", got)
		}
	})
}

// TestEnsureServeLeavesALiveSocketAlone is the regression itself: EnsureServe
// used to unlink the socket before spawning, so a probe that merely missed a
// live engine took the path away from it. The engine kept running on the
// orphaned inode with the reviewer's frontend attached, and every later client
// reached the replacement instead.
func TestEnsureServeLeavesALiveSocketAlone(t *testing.T) {
	dir := shortTempDir(t)
	path := filepath.Join(dir, "engine.sock")
	listenOn(t, path)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	got, spawned, err := EnsureServe(AutoSpawnOptions{Socket: path})
	if err != nil {
		t.Fatalf("EnsureServe: %v", err)
	}
	if spawned {
		t.Error("spawned a second engine for a socket that was answering")
	}
	if got != path {
		t.Errorf("socket = %q, want %q", got, path)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the live socket was unlinked: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Error("the socket was replaced; the original engine has been orphaned")
	}
}

// Same protection when the engine is too loaded to answer: a live owner means
// hands off, not "spawn a rival".
func TestEnsureServeDoesNotReplaceABusyEngine(t *testing.T) {
	dir := shortTempDir(t)
	path := filepath.Join(dir, "busy.sock")
	leftoverFile(t, path)
	os.WriteFile(PIDFilePath(path), []byte(strconv.Itoa(os.Getpid())), 0o644)

	_, spawned, err := EnsureServe(AutoSpawnOptions{Socket: path})
	if err != nil {
		t.Fatalf("EnsureServe: %v", err)
	}
	if spawned {
		t.Error("spawned a rival engine while the recorded owner was still alive")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the busy engine's socket was removed: %v", err)
	}
}

func TestPIDFilePath(t *testing.T) {
	if got := PIDFilePath("/tmp/monocle-abc123.sock"); got != "/tmp/monocle-abc123.pid" {
		t.Errorf("got %q", got)
	}
	// A socket path with no .sock suffix still gets a distinct pid path.
	if got := PIDFilePath("/tmp/custom"); got != "/tmp/custom.pid" {
		t.Errorf("got %q", got)
	}
}
