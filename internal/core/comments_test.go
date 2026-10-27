package core

import (
	"strings"
	"sync"
	"testing"

	"github.com/josephschmitt/monocle/internal/protocol"
)

// comments asks the reviewer's TUI to show or hide source-code comment lines (a
// walkthrough's tour-wide comment state): an event, as doc_pane is. on, off and
// toggle go through as given; anything else is refused with no event. Delivered
// counts the attached TUIs that said they understand it.
func TestCommentsIsAnEvent(t *testing.T) {
	e := eventsEngine(t)
	var mu sync.Mutex
	var got []string
	e.On(EventComments, func(p EventPayload) { mu.Lock(); got = append(got, p.Status); mu.Unlock() })
	ask := func(state string) *protocol.CommentsResponse {
		r, ok := e.server.handleMessage(&protocol.CommentsMsg{Type: protocol.TypeComments, State: state}).(*protocol.CommentsResponse)
		if !ok {
			t.Fatalf("comments %q did not answer a *CommentsResponse", state)
		}
		return r
	}
	if r := ask("off"); !r.Success || r.Delivered != 0 || !strings.Contains(r.Message, "no Monocle TUI") {
		t.Errorf("off with no TUI attached: %+v", r)
	}
	tui := subscribeOverPipe(t, e, `{"type":"subscribe","events":["comments"],"passive":true,"frontend":{"pid":5,"features":["doc","comments"]}}`)
	defer tui.Close()
	old := subscribeOverPipe(t, e, `{"type":"subscribe","events":["doc_pane"],"passive":true,"frontend":{"pid":6,"features":["doc"]}}`)
	defer old.Close()
	for _, s := range []string{"on", "toggle"} {
		if r := ask(s); !r.Success || r.Delivered != 1 {
			t.Errorf("%s with one TUI that knows comments (and one that knows only doc): %+v", s, r)
		}
	}
	if r := ask("dim"); r.Success || !strings.Contains(r.Message, "on, off or toggle") {
		t.Errorf("an unknown state should be refused, naming the three: %+v", r)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(got, ",") != "off,on,toggle" {
		t.Errorf("events = %v, want off, on, toggle and none for the refused one", got)
	}
}
