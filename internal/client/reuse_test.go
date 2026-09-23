package client

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/protocol"
)

// oneShotServer mimics the engine: it answers one request per connection and
// then closes, which is exactly the behaviour that broke a second request on the
// same Client.
func oneShotServer(t *testing.T) (string, *int) {
	t.Helper()
	dir, err := os.MkdirTemp("", "cr")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "e.sock")

	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })

	served := 0
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			served++
			buf := make([]byte, 64*1024)
			n, err := conn.Read(buf)
			if err != nil || n == 0 {
				conn.Close()
				continue
			}
			data, _ := protocol.Encode(&protocol.SubmitDiffResponse{
				Type: protocol.TypeSubmitDiffResponse, Success: true, Message: "ok",
			})
			conn.Write(data)
			conn.Close() // one-shot: the engine does this too
		}
	}()
	time.Sleep(20 * time.Millisecond)
	return path, &served
}

// TestSecondRequestOnAClientSucceeds is the reported bug: send_diff sends one
// request per pair down a single Client, and the second wrote into a socket the
// engine had already closed — "broken pipe", with the first pair having quietly
// landed.
func TestSecondRequestOnAClientSucceeds(t *testing.T) {
	path, served := oneShotServer(t)
	c, err := Connect(path)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	for i := 0; i < 3; i++ {
		resp, err := c.Request(&protocol.SubmitDiffMsg{
			Type: protocol.TypeSubmitDiff, Title: "pair", Before: "a", After: "b",
		}, 2*time.Second)
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		if r, ok := resp.(*protocol.SubmitDiffResponse); !ok || !r.Success {
			t.Fatalf("request %d: unexpected response %+v", i+1, resp)
		}
	}
	// One connection per request, which is what the engine's contract requires.
	if *served != 3 {
		t.Errorf("server saw %d connections, want 3", *served)
	}
}

// A dead engine has to surface as an error rather than a panic on a nil conn.
func TestRedialAfterTheServerGoesAway(t *testing.T) {
	path, _ := oneShotServer(t)
	c, err := Connect(path)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	if _, err := c.Request(&protocol.GetReviewStatusMsg{Type: protocol.TypeGetReviewStatus}, time.Second); err != nil {
		t.Fatalf("first request: %v", err)
	}
	os.Remove(path)
	if _, err := c.Request(&protocol.GetReviewStatusMsg{Type: protocol.TypeGetReviewStatus}, time.Second); err == nil {
		t.Error("a request against a vanished engine should fail, not succeed")
	}
}
