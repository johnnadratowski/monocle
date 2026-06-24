package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestRecordSearchDedupesAndOrders(t *testing.T) {
	m := appModel{}
	m.recordSearch("foo")
	m.recordSearch("bar")
	m.recordSearch("foo") // repeat moves to front, no duplicate
	want := []string{"foo", "bar"}
	if len(m.searchHistory) != len(want) {
		t.Fatalf("history = %v, want %v", m.searchHistory, want)
	}
	for i := range want {
		if m.searchHistory[i] != want[i] {
			t.Errorf("history[%d] = %q, want %q", i, m.searchHistory[i], want[i])
		}
	}
	// Empty queries are ignored.
	m.recordSearch("")
	if len(m.searchHistory) != 2 {
		t.Errorf("empty query was recorded: %v", m.searchHistory)
	}
}

func TestRecallSearchCycles(t *testing.T) {
	m := appModel{searchHistory: []string{"newest", "middle", "oldest"}, searchHistoryIdx: -1}
	if got := m.recallSearch(1); got != "newest" {
		t.Errorf("first up = %q, want newest", got)
	}
	if got := m.recallSearch(1); got != "middle" {
		t.Errorf("second up = %q, want middle", got)
	}
	if got := m.recallSearch(1); got != "oldest" {
		t.Errorf("third up = %q, want oldest", got)
	}
	if got := m.recallSearch(1); got != "oldest" {
		t.Errorf("past-oldest up = %q, want oldest (clamped)", got)
	}
	if got := m.recallSearch(-1); got != "middle" {
		t.Errorf("down = %q, want middle", got)
	}
}

func diffAppWithLines(text string, n int) appModel {
	m := NewApp(nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app := updated.(appModel)
	app.focus = focusMain
	app.diffView.focused = true
	app.diffView.lines = make([]diffViewLine, n)
	for i := range app.diffView.lines {
		app.diffView.lines[i] = diffViewLine{newLineNum: i + 1, content: "plain"}
	}
	// Put the search target on one line.
	app.diffView.lines[n/2].content = text
	return app
}

func TestSeedSearchFromHistoryActivatesDiff(t *testing.T) {
	app := diffAppWithLines("needle here", 20)
	if app.diffView.SearchActive() {
		t.Fatal("precondition: search should be inactive")
	}
	// No history yet → nothing to seed.
	if app.seedSearchFromHistory() {
		t.Error("seed with empty history should return false")
	}
	app.recordSearch("needle")
	if !app.seedSearchFromHistory() {
		t.Fatal("seed should activate search for a query with matches")
	}
	if !app.diffView.SearchActive() {
		t.Error("diff search should be active after seeding")
	}
}

func TestHelpSeedFromHistory(t *testing.T) {
	h := newTestHelp()
	h.history = []string{"wrap"} // "Toggle line wrapping" appears in help
	// Press n with no active help search → should seed from history and match.
	out, _ := h.Update(tea.KeyPressMsg{Code: 'n'})
	h = out
	if h.searchQuery != "wrap" {
		t.Errorf("query = %q, want seeded 'wrap'", h.searchQuery)
	}
	if len(h.searchMatches) == 0 {
		t.Error("expected matches after seeding help search from history")
	}
}
