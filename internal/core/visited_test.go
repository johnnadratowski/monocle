package core

import (
	"reflect"
	"testing"

	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
)

// A stop is visited once it has been the current stop: the reviewer's moves and
// the agent's goto record it, with the review, so a restart keeps it. A tour
// arriving does not count its first stop until the reviewer is shown it.
func TestVisitedStops(t *testing.T) {
	tour := func(title string, ids ...string) *protocol.SetWalkthroughMsg {
		var stops []types.WalkthroughStop
		for _, id := range ids {
			stops = append(stops, types.WalkthroughStop{ID: id})
		}
		return &protocol.SetWalkthroughMsg{Type: protocol.TypeSetWalkthrough, Walkthrough: types.Walkthrough{Title: title, Stops: stops}}
	}
	e, _ := summaryEngine(t)
	e.handleSetWalkthrough(tour("T", "1.1", "1.2", "2"))
	if got := e.current.WalkthroughVisited; len(got) != 0 {
		t.Fatalf("visited %v before the reviewer was shown anything", got)
	}
	if err := e.SetWalkthroughStop("1.1"); err != nil {
		t.Fatal(err)
	}
	e.handleGotoStop(&protocol.GotoStopMsg{Type: protocol.TypeGotoStop, ID: "2"})
	_ = e.SetWalkthroughStop("1.1")
	if got := e.current.WalkthroughVisited; !reflect.DeepEqual(got, []string{"1.1", "2"}) {
		t.Errorf("visited %v, want 1.1 and 2", got)
	}

	// A restart resumes them, with the stop the reviewer is on.
	resumed, err := e.sessions.ResumeSession(e.current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := resumed.WalkthroughVisited; !reflect.DeepEqual(got, []string{"1.1", "2"}) {
		t.Errorf("after a resume: %v", got)
	}

	// Re-sending the tour keeps them, less ids it no longer has; another tour
	// starts afresh; withdrawing the tour forgets them.
	e.handleSetWalkthrough(tour("T", "1.1", "3"))
	if got := e.current.WalkthroughVisited; !reflect.DeepEqual(got, []string{"1.1"}) {
		t.Errorf("after a re-send: %v, want 1.1 (2 is gone)", got)
	}
	e.handleSetWalkthrough(tour("Round two", "1.1"))
	if got := e.current.WalkthroughVisited; len(got) != 0 {
		t.Errorf("a new tour starts with %v visited", got)
	}
	_ = e.SetWalkthroughStop("1.1")
	e.handleSetWalkthrough(&protocol.SetWalkthroughMsg{Type: protocol.TypeSetWalkthrough})
	e.handleSetWalkthrough(tour("Round two", "1.1"))
	if got := e.current.WalkthroughVisited; len(got) != 0 {
		t.Errorf("withdrawing and re-sending kept %v visited", got)
	}
}
