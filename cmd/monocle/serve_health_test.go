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
