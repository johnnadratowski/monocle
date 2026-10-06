package main

import (
	"bufio"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/protocol"
)

// shortSock returns a unix-socket path short enough for the OS path limit
// (t.TempDir paths with subtest names blow past macOS's 104-char cap).
func shortSock(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "m")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

// startMockServe listens on a unix socket and handles each connection with fn.
func startMockServe(t *testing.T, sock string, fn func(net.Conn)) {
	t.Helper()
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen %s: %v", sock, err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go fn(conn)
		}
	}()
}

// respondVersion reads the request line and replies with a server-info response.
func respondVersion(v string) func(net.Conn) {
	return func(conn net.Conn) {
		defer conn.Close()
		if _, err := bufio.NewReader(conn).ReadBytes('\n'); err != nil {
			return
		}
		data, _ := protocol.Encode(&protocol.GetServerInfoResponse{
			Type:    protocol.TypeGetServerInfoResponse,
			Version: v,
		})
		_, _ = conn.Write(data)
	}
}

func TestServeIsHealthy(t *testing.T) {
	t.Run("connect failed when nothing is listening", func(t *testing.T) {
		sock := shortSock(t)
		if ok, _ := serveIsHealthy(sock, "v1", true, time.Second); ok {
			t.Error("expected unhealthy when no serve is listening")
		}
	})

	t.Run("unresponsive serve (accepts but never replies)", func(t *testing.T) {
		sock := shortSock(t)
		startMockServe(t, sock, func(conn net.Conn) { _, _ = io.Copy(io.Discard, conn) })
		start := time.Now()
		if ok, _ := serveIsHealthy(sock, "v1", true, 200*time.Millisecond); ok {
			t.Error("expected unhealthy when the serve never responds")
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("health check should give up near the timeout, took %s", elapsed)
		}
	})

	t.Run("healthy when version matches", func(t *testing.T) {
		sock := shortSock(t)
		startMockServe(t, sock, respondVersion("v1"))
		if ok, reason := serveIsHealthy(sock, "v1", true, time.Second); !ok {
			t.Errorf("expected healthy, got %q", reason)
		}
	})

	t.Run("unhealthy on version mismatch (when checked)", func(t *testing.T) {
		sock := shortSock(t)
		startMockServe(t, sock, respondVersion("v2"))
		if ok, _ := serveIsHealthy(sock, "v1", true, time.Second); ok {
			t.Error("expected unhealthy on version mismatch")
		}
		// Same serve is healthy when we don't check the version.
		if ok, reason := serveIsHealthy(sock, "v1", false, time.Second); !ok {
			t.Errorf("expected healthy when version not checked, got %q", reason)
		}
	})
}

// A relaunch keeps the serve only when it provably runs the build being
// relaunched onto; anything less restarts it.
func TestServeRunsBuild(t *testing.T) {
	for _, c := range []struct {
		name, serve, old, new string
		want                  bool
	}{
		{"already on the new build", "v2", "v1", "v2", true},
		{"still on the build being left", "v1", "v1", "v2", false},
		{"on some other build", "v0", "v1", "v2", false},
		{"two builds reporting the same version", "v1", "v1", "v1", false},
		{"a new build that does not say what it is", "v1", "v1", "", false},
		{"neither the new build nor the serve says", "", "v1", "", false},
		{"a serve that does not say what it is", "", "v1", "v2", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			sock := shortSock(t)
			startMockServe(t, sock, respondVersion(c.serve))
			if got := serveRunsBuild(sock, c.old, c.new); got != c.want {
				t.Errorf("serveRunsBuild(serve %q, old %q, new %q) = %v, want %v", c.serve, c.old, c.new, got, c.want)
			}
		})
	}
	t.Run("nothing listening", func(t *testing.T) {
		if serveRunsBuild(shortSock(t), "v1", "v2") {
			t.Error("no serve cannot be running the new build")
		}
	})
}

// goto-line sends its path, line and top as the request.
func TestGotoLineCmdSendsTheRequest(t *testing.T) {
	sock := shortSock(t)
	got := make(chan *protocol.GotoLineMsg, 1)
	startMockServe(t, sock, func(conn net.Conn) {
		defer conn.Close()
		line, err := bufio.NewReader(conn).ReadBytes('\n')
		if err != nil {
			return
		}
		if msg, err := protocol.Decode(line); err == nil {
			if m, ok := msg.(*protocol.GotoLineMsg); ok {
				got <- m
			}
		}
		data, _ := protocol.Encode(&protocol.GotoLineResponse{Type: protocol.TypeGotoLineResponse, Success: true, Message: "ok"})
		_, _ = conn.Write(data)
	})
	cmd := ReviewGotoLineCmd{Socket: sock, Path: "a.go", Line: 12, Top: intPtr(3)}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if m.Path != "a.go" || m.Line != 12 || m.Top == nil || *m.Top != 3 {
			t.Errorf("sent %+v, want a.go:12 with top 3", m)
		}
	default:
		t.Fatal("no goto_line request reached the engine")
	}
}
