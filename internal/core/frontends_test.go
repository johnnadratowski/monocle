package core

import (
	"bufio"
	"encoding/json"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/protocol"
)

// subscribeOverPipe opens a persistent connection the way a frontend does and
// waits for the ack. Closing the returned conn is the frontend going away.
func subscribeOverPipe(t *testing.T, e *Engine, line string) net.Conn {
	t.Helper()
	conn, served := net.Pipe()
	go e.server.handleConnection(served)
	if _, err := conn.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("write subscribe: %v", err)
	}
	sc := bufio.NewScanner(conn)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if !sc.Scan() {
		t.Fatalf("no subscribe ack: %v", sc.Err())
	}
	var ack map[string]any
	_ = json.Unmarshal(sc.Bytes(), &ack)
	if ack["success"] != true {
		t.Fatalf("subscribe ack = %v", ack)
	}
	conn.SetReadDeadline(time.Time{})
	// Keep draining so event writes never block the server.
	go func() {
		for sc.Scan() {
		}
	}()
	return conn
}

func frontends(t *testing.T, e *Engine) *protocol.GetFrontendsResponse {
	t.Helper()
	r, ok := e.server.handleMessage(&protocol.GetFrontendsMsg{Type: protocol.TypeGetFrontends}).(*protocol.GetFrontendsResponse)
	if !ok {
		t.Fatal("get_frontends did not answer a *GetFrontendsResponse")
	}
	return r
}

// get_frontends lists the frontends attached now, as each said on
// subscribing, and the features every one of them has: what a driver may
// type into any of them.
func TestGetFrontendsListsTheLiveFrontends(t *testing.T) {
	e := eventsEngine(t)
	if r := frontends(t, e); len(r.Frontends) != 0 || r.Features == nil || len(r.Features) != 0 {
		t.Fatalf("with nothing attached: %+v, want no frontends and features []", r)
	}

	agent := subscribeOverPipe(t, e, `{"type":"subscribe","events":["pause_changed"]}`)
	defer agent.Close()
	newTUI := subscribeOverPipe(t, e, `{"type":"subscribe","events":["pause_changed"],"passive":true,"frontend":{"pid":11,"version":"v2","features":["doc","zoom"]}}`)
	r := frontends(t, e)
	if len(r.Frontends) != 1 || r.Frontends[0].Pid != 11 || !reflect.DeepEqual(r.Features, []string{"doc", "zoom"}) {
		t.Fatalf("one new TUI (and an agent, not a frontend): %+v", r)
	}

	other := subscribeOverPipe(t, e, `{"type":"subscribe","events":["pause_changed"],"passive":true,"frontend":{"pid":12,"version":"v2","features":["doc"]}}`)
	if r := frontends(t, e); len(r.Frontends) != 2 || !reflect.DeepEqual(r.Features, []string{"doc"}) {
		t.Errorf("two TUIs: features should be what both have: %+v", r)
	}
	other.Close()
	waitFor(t, func() bool { return len(frontends(t, e).Frontends) == 1 }, "a closed frontend to leave the list")

	// A TUI from before frontends were reported subscribes with none: it is
	// a frontend with no features, so nothing may be typed into it.
	oldTUI := subscribeOverPipe(t, e, `{"type":"subscribe","events":["pause_changed"],"passive":true}`)
	defer oldTUI.Close()
	_ = oldTUI
	newTUI.Close()
	waitFor(t, func() bool { return len(frontends(t, e).Frontends) == 0 }, "the last reporting frontend to leave")
}

func waitFor(t *testing.T, ok func() bool, what string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
