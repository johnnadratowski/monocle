package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func newTestHelp() helpModel {
	theme := DefaultTheme()
	km := DefaultKeyMap()
	h := newHelpModel(theme, &km)
	h.active = true
	h.width = 100
	h.height = 24 // small viewport so content is taller than the modal
	h.reviewTracking = true
	return h
}

func press(h helpModel, key string) helpModel {
	out, _ := h.Update(tea.KeyPressMsg{Code: keyCodeFor(key)})
	return out
}

// keyCodeFor maps the strings used in these tests to the rune codes Bubble Tea
// uses; KeyPressMsg.String() then yields the expected name ("esc", "enter", ...).
func keyCodeFor(key string) rune {
	switch key {
	case "esc":
		return tea.KeyEscape
	case "enter":
		return tea.KeyEnter
	case "backspace":
		return tea.KeyBackspace
	case "space":
		return tea.KeySpace
	default:
		return rune(key[0])
	}
}

func TestHelpScrollClampsAtBottom(t *testing.T) {
	h := newTestHelp()
	// Hold j well past the bottom.
	for i := 0; i < 500; i++ {
		h = press(h, "j")
	}
	max := h.maxScroll()
	if max <= 0 {
		t.Fatalf("expected content taller than viewport, maxScroll=%d", max)
	}
	if h.scrollOffset != max {
		t.Fatalf("scrollOffset = %d after holding down, want clamped to maxScroll %d", h.scrollOffset, max)
	}
	// A single up press must immediately move the viewport (the bug was that
	// offset grew unbounded so up did nothing visible for a long time).
	h = press(h, "k")
	if h.scrollOffset != max-1 {
		t.Errorf("after one up: scrollOffset = %d, want %d", h.scrollOffset, max-1)
	}
}

func TestHelpScrollGtopGbottom(t *testing.T) {
	h := newTestHelp()
	h = press(h, "G")
	if h.scrollOffset != h.maxScroll() {
		t.Errorf("G: scrollOffset = %d, want maxScroll %d", h.scrollOffset, h.maxScroll())
	}
	h = press(h, "g")
	if h.scrollOffset != 0 {
		t.Errorf("g: scrollOffset = %d, want 0", h.scrollOffset)
	}
}

func TestHelpSearchFindsAndScrolls(t *testing.T) {
	h := newTestHelp()
	// Enter search mode and type "wrap" (present in nav keys: "Toggle line wrapping").
	h = press(h, "/")
	if !h.searchMode {
		t.Fatal("expected searchMode after pressing /")
	}
	for _, r := range "wrap" {
		h = press(h, string(r))
	}
	if len(h.searchMatches) == 0 {
		t.Fatalf("expected matches for 'wrap', got none")
	}
	// Confirm with enter: stays highlighted, exits typing mode.
	h = press(h, "enter")
	if h.searchMode {
		t.Error("expected searchMode false after enter")
	}
	if h.searchQuery != "wrap" {
		t.Errorf("query = %q, want wrap", h.searchQuery)
	}
	// View should render without panicking and contain the search bar.
	out := h.View()
	if !strings.Contains(out, "/wrap") {
		t.Errorf("expected search bar with /wrap in view")
	}
	// n cycles matches.
	before := h.searchIndex
	h = press(h, "n")
	if len(h.searchMatches) > 1 && h.searchIndex == before {
		t.Error("expected n to advance the match index")
	}
	// esc clears the search (first esc), keeping help open.
	h = press(h, "esc")
	if h.searchQuery != "" || len(h.searchMatches) != 0 {
		t.Errorf("expected esc to clear search; query=%q matches=%d", h.searchQuery, len(h.searchMatches))
	}
	if !h.active {
		t.Error("first esc should clear search, not close help")
	}
}

func TestHelpSearchNoMatch(t *testing.T) {
	h := newTestHelp()
	h = press(h, "/")
	for _, r := range "zzzznotfound" {
		h = press(h, string(r))
	}
	if len(h.searchMatches) != 0 {
		t.Errorf("expected no matches, got %d", len(h.searchMatches))
	}
	out := h.View()
	if !strings.Contains(out, "no matches") {
		t.Error("expected 'no matches' in search bar")
	}
}

func TestHelpKeyClosesHelp(t *testing.T) {
	h := newTestHelp()
	out, cmd := h.Update(tea.KeyPressMsg{Code: 'H'})
	h = out
	if h.active {
		t.Error("expected H to close the help overlay")
	}
	if cmd == nil {
		t.Error("expected a closeHelpMsg command")
	}
}

// TestHelpColumnAlignment verifies a long command key does not push its
// description out of alignment with short-key rows (the column is sized to the
// widest key).
func TestHelpColumnAlignment(t *testing.T) {
	h := newTestHelp()
	h.width = 120
	lines := strings.Split(h.buildContent(), "\n")

	descCol := func(descText string) int {
		for _, ln := range lines {
			plain := stripANSIHelp(ln)
			if i := strings.Index(plain, descText); i >= 0 {
				return i
			}
		}
		return -1
	}

	short := descCol("Move up/down")
	long := descCol("Base artifact version to diff against")
	if short < 0 || long < 0 {
		t.Fatalf("rows not found: short=%d long=%d", short, long)
	}
	if short != long {
		t.Errorf("description columns misaligned: short row starts at %d, long-key row at %d", short, long)
	}
}

func stripANSIHelp(s string) string {
	return ansiStripForTest(s)
}

func ansiStripForTest(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
