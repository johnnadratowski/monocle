package client

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/josephschmitt/monocle/internal/core"
	"github.com/josephschmitt/monocle/internal/protocol"
)

// A frontend's client says what it is in every subscribe, the first and each
// one after a redial, so a restarted engine learns it again.
func TestFrontendClientSubscribesWithItsInfo(t *testing.T) {
	info := protocol.FrontendInfo{Pid: 9, Version: "v3", Features: []string{"doc"}}
	c := &EngineClient{frontend: &info}
	sub := c.subscribeMsg()
	if !sub.Passive || sub.Frontend == nil || !reflect.DeepEqual(*sub.Frontend, info) {
		t.Errorf("subscribe = %+v, want passive with the frontend", sub)
	}
	if plain := (&EngineClient{}).subscribeMsg(); plain.Frontend != nil {
		t.Errorf("a client given no frontend reported one: %+v", plain.Frontend)
	}
}

// The TUI's client subscribes to doc_pane, so the engine's event reaches it.
func TestSubscribeIncludesDocPane(t *testing.T) {
	if sub := (&EngineClient{}).subscribeMsg(); !slices.Contains(sub.Events, string(core.EventDocPane)) {
		t.Errorf("subscribe events %v lack %s", sub.Events, core.EventDocPane)
	}
}

// doc_pane's event reaches a client with its state.
func TestEngineClient_DocPaneEvent(t *testing.T) {
	_, socketPath := setupEngine(t)
	ec, err := NewEngineClient(socketPath)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer ec.Close()
	events := make(chan core.EventPayload, 1)
	ec.On(core.EventDocPane, func(p core.EventPayload) { events <- p })
	c, err := Connect(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	resp, err := c.Request(&protocol.DocPaneMsg{Type: protocol.TypeDocPane, State: "off"}, time.Second)
	if r, ok := resp.(*protocol.DocPaneResponse); err != nil || !ok || !r.Success {
		t.Fatalf("doc_pane: %+v %v", resp, err)
	}
	select {
	case p := <-events:
		if p.Status != "off" {
			t.Errorf("event %+v, want off", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the doc_pane event never reached the client")
	}
}

// The TUI's client subscribes to comments, so the walkthrough's push reaches it.
func TestSubscribeIncludesComments(t *testing.T) {
	if sub := (&EngineClient{}).subscribeMsg(); !slices.Contains(sub.Events, string(core.EventComments)) {
		t.Errorf("subscribe events %v lack %s", sub.Events, core.EventComments)
	}
}
