package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

// diffTop is the first content row of the diff's box on screen.
func diffTop(m appModel) int { return computePaneLayout(&m).diff.y }

// A stop's note sits above the code, in every layout: its title row is above
// the diff's box, the click regions say the same, and the diff takes the
// rest of the column.
func TestTheNoteSitsAboveTheCode(t *testing.T) {
	for _, l := range layouts {
		t.Run(l.name, func(t *testing.T) {
			m := viewsAppIn(t, &types.Config{}, l.width, l.sidebarHidden)
			_, ty := onScreen(t, m, "1.2 · Where it lands")
			lay := computePaneLayout(&m)
			if ty >= lay.diff.y {
				t.Errorf("the note's title is on row %d, the diff starts on row %d: want the note above", ty, lay.diff.y)
			}
			if lay.doc.y+lay.doc.h+borderH != lay.diff.y {
				t.Errorf("doc region %+v, diff region %+v: want the diff's box straight under the note's", lay.doc, lay.diff)
			}
			lines := strings.Split(screenText(m), "\n")
			if bottom := lines[lay.diff.y+lay.diff.h]; !strings.Contains(bottom, "─") {
				t.Errorf("the row under the diff's last row is %q, want its box's bottom border", bottom)
			}
		})
	}
}

// From the diff, up reaches a note above it; from the note, down returns to
// the diff and up goes on to a stacked sidebar. Down from the diff leaves
// Monocle, as there is nothing under it.
func TestPaneNeighbourWithTheNoteAbove(t *testing.T) {
	note := func(focus focusTarget, layout layoutMode) appModel {
		m := navModel(focus, layout, true, true)
		m.docPane.note = true
		return m
	}
	for _, c := range []struct {
		name   string
		m      appModel
		dir    paneDir
		want   focusTarget
		wantOK bool
	}{
		{"diff up reaches the note", note(focusMain, layoutHorizontal), paneUp, focusDoc, true},
		{"diff down is the window edge", note(focusMain, layoutHorizontal), paneDown, 0, false},
		{"note down returns to the diff", note(focusDoc, layoutHorizontal), paneDown, focusMain, true},
		{"note up is the window edge", note(focusDoc, layoutHorizontal), paneUp, 0, false},
		{"note left reaches the sidebar", note(focusDoc, layoutHorizontal), paneLeft, focusSidebar, true},
		{"stacked diff up reaches the note", note(focusMain, layoutStacked), paneUp, focusDoc, true},
		{"stacked note up reaches the sidebar", note(focusDoc, layoutStacked), paneUp, focusSidebar, true},
		{"stacked sidebar down reaches the note", note(focusSidebar, layoutStacked), paneDown, focusDoc, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, ok := c.m.paneNeighbour(c.dir)
			if ok != c.wantOK || (ok && got != c.want) {
				t.Errorf("= %v, %v; want %v, %v", got, ok, c.want, c.wantOK)
			}
		})
	}
}

// shortAndLongTour is viewsTour with 1.2's note sixty lines long; 1.1's is one.
func shortAndLongTour() *types.Walkthrough {
	tour := viewsTour()
	var note strings.Builder
	for i := 1; i <= 60; i++ {
		note.WriteString("Note line " + strconv.Itoa(i) + ".\n")
	}
	tour.Stops[1].Note = note.String()
	return tour
}

// The note's pane is the same height on every stop, a short note's and a long
// one's, so the code under it stays put from stop to stop: a third of the
// window by default, or tour_note_percent of it.
func TestTheNotePaneHasAFixedHeight(t *testing.T) {
	for _, c := range []struct {
		percent, height, want int
	}{
		{0, 44, 44 * 33 / 100},
		{0, 60, 60 * 33 / 100},
		{50, 44, 44 * 50 / 100},
	} {
		m, _ := tourAppWith(t, shortAndLongTour(), inertViewers(&types.Config{TourNotePercent: c.percent}))
		m = updateApp(t, m, tea.WindowSizeMsg{Width: 140, Height: c.height})
		m = typeCommand(t, m, "stop 1.1")
		short, diff := m.docPane.height, m.diffView.height
		m = pressKey(t, m, ".")
		if m.docPane.height != short || m.diffView.height != diff {
			t.Errorf("%d%% of %d: note %d → %d rows, diff %d → %d: want both the same on every stop", c.percent, c.height, short, m.docPane.height, diff, m.diffView.height)
		}
		if outer := m.docPane.height + 2; outer != c.want {
			t.Errorf("%d%% of %d: the note's box is %d rows, want %d", c.percent, c.height, outer, c.want)
		}
	}
}
