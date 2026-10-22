package core

import (
	"strings"
	"sync"
	"testing"

	"github.com/josephschmitt/monocle/internal/protocol"
)

// doc_pane asks the reviewer's TUI to hide or show its doc pane: an event, as
// goto_line is, so nothing is typed into the TUI, and a TUI too old to know
// the event never subscribed to it. on, off and toggle go through as given;
// anything else is refused with no event. Delivered counts the attached TUIs
// that said they understand it.
func TestDocPaneIsAnEvent(t *testing.T) {
	e := eventsEngine(t)
	var mu sync.Mutex
	var got []string
	e.On(EventDocPane, func(p EventPayload) { mu.Lock(); got = append(got, p.Status); mu.Unlock() })
	ask := func(state string) *protocol.DocPaneResponse {
		r, ok := e.server.handleMessage(&protocol.DocPaneMsg{Type: protocol.TypeDocPane, State: state}).(*protocol.DocPaneResponse)
		if !ok {
			t.Fatalf("doc_pane %q did not answer a *DocPaneResponse", state)
		}
		return r
	}

	if r := ask("off"); !r.Success || r.Delivered != 0 || !strings.Contains(r.Message, "no Monocle TUI") {
		t.Errorf("off with no TUI attached: %+v", r)
	}
	tui := subscribeOverPipe(t, e, `{"type":"subscribe","events":["doc_pane"],"passive":true,"frontend":{"pid":5,"features":["doc"]}}`)
	defer tui.Close()
	old := subscribeOverPipe(t, e, `{"type":"subscribe","events":["pause_changed"],"passive":true}`)
	defer old.Close()
	for _, s := range []string{"on", "toggle"} {
		if r := ask(s); !r.Success || r.Delivered != 1 {
			t.Errorf("%s with one TUI that knows doc: %+v", s, r)
		}
	}
	if r := ask("sideways"); r.Success || !strings.Contains(r.Message, "on, off or toggle") {
		t.Errorf("an unknown state should be refused, naming the three: %+v", r)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(got, ",") != "off,on,toggle" {
		t.Errorf("events = %v, want off, on, toggle and none for the refused one", got)
	}
}
