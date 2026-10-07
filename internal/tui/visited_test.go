package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/josephschmitt/monocle/internal/core"
	"github.com/josephschmitt/monocle/internal/types"
)

// noteTitle is the doc pane's title row, as drawn.
func noteTitle(m appModel) string {
	return strings.SplitN(stripANSISeq(m.docPane.View()), "\n", 2)[0]
}

// A stop's title says whether the reviewer has been on it before: new on the
// first arrival, visited on any later one, worked out on arrival and held for
// the whole stay.
func TestTheStopTitleSaysWhetherItIsNew(t *testing.T) {
	m, _ := tourApp(t) // restored on 1.1, which nothing records as visited
	m = pressKey(t, m, ".")
	if !m.tour.fresh || !strings.Contains(noteTitle(m), "Where it lands "+stopNewMark+" ") {
		t.Fatalf("first arrival at 1.2: title %q, want it marked new", noteTitle(m))
	}
	// Staying — the tour switched off and on — keeps it new.
	m = pressKey(t, pressKey(t, m, "W"), "W")
	if !m.tour.fresh {
		t.Error("1.2 turned visited during the stay")
	}
	m = pressKey(t, pressKey(t, m, ","), ".")
	if m.tour.fresh || !strings.Contains(noteTitle(m), "Where it lands "+stopVisitedMark+" ") {
		t.Errorf("back at 1.2: title %q, want it marked visited", noteTitle(m))
	}
	// Back/forward, a call or :stop count the same.
	m = typeCommand(t, m, "stop 2")
	if !m.tour.fresh {
		t.Error(":stop 2, never visited, is not new")
	}
	m = updateApp(t, m, backspaceKey)
	if m.tour.fresh {
		t.Error("back to 1.2 is not visited")
	}
}

// The visited stops come with the review, so a restart knows them; the agent's
// goto counts the stop it shows as new the first time, though the engine has
// already recorded it.
func TestVisitedStopsComeWithTheReview(t *testing.T) {
	_, e := tourAppWith(t, testTour(), &types.Config{})
	e.session.WalkthroughVisited = []string{"1.2", "gone"}
	// A restart: a new TUI over the same engine.
	m := NewApp(e)
	m = updateApp(t, m, tea.WindowSizeMsg{Width: 140, Height: 44})
	m = updateApp(t, m, initialLoadMsg{files: e.changedFiles})
	m = pressKey(t, m, ".")
	if m.tour.fresh {
		t.Error("1.2, visited before the restart, came up new")
	}
	e.session.WalkthroughVisited = append(e.session.WalkthroughVisited, "2")
	e.session.WalkthroughStop = "2"
	m = updateApp(t, m, tourEventMsg{status: core.WalkthroughEventGoto, id: "2"})
	if !m.tour.fresh {
		t.Error("the agent's first goto to 2 did not show it as new")
	}
}

// A tour arriving reports its first stop, so the engine records it visited.
func TestATourArrivingRecordsItsFirstStop(t *testing.T) {
	m, e := noTourApp(t)
	e.session.Walkthrough, e.session.WalkthroughStop = testTour(), "1.1"
	m = updateApp(t, m, tourEventMsg{status: core.WalkthroughEventSet, id: "1.1"})
	if got := e.reports(); len(got) != 1 || got[0] != "1.1" {
		t.Errorf("reported %v, want the first stop", got)
	}
	if !m.tour.fresh {
		t.Error("a new tour's first stop is not new")
	}
}
