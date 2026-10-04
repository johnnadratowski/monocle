package tui

import (
	"reflect"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

// nearTour has four stops in a.go and one in b.go, so the stops around the
// current one share its file.
func nearTour() *types.Walkthrough {
	return &types.Walkthrough{Title: "Tour", Stops: []types.WalkthroughStop{
		{ID: "1", Title: "One", File: "a.go", LineStart: 5, LineEnd: 6},
		{ID: "2", Title: "Two", File: "a.go", LineStart: 10, LineEnd: 11},
		{ID: "3", Title: "Three", File: "a.go", LineStart: 20},
		{ID: "4", Title: "Four", File: "a.go", LineStart: 30},
		{ID: "5", Title: "Five", File: "b.go", LineStart: 40},
	}}
}

// gutterMarks names the gutter mark of every row of the shown file that has
// one, by its new-file line number.
func gutterMarks(m appModel) map[int]string {
	names := map[string]string{
		tourGutterColor:      "current",
		prevStopGutterColor:  "previous",
		nextStopGutterColor:  "next",
		laterStopGutterColor: "after next",
	}
	marks := map[int]string{}
	for _, ln := range m.diffView.lines {
		bg := m.diffView.markGutter(ln, lipgloss.NewStyle()).GetBackground()
		for code, name := range names {
			if bg == lipgloss.Color(code) {
				marks[ln.newLineNum] = name
			}
		}
	}
	return marks
}

// The previous stop and the next two are marked in the gutter too, each in its
// own colour, so the reviewer sees where the tour goes from here.
func TestTheStopsAroundTheCurrentOneAreMarked(t *testing.T) {
	m, _ := tourAppWith(t, nearTour(), &types.Config{})
	m = pressKey(t, m, ".") // 2
	want := map[int]string{5: "previous", 6: "previous", 10: "current", 11: "current", 20: "next", 30: "after next"}
	if got := gutterMarks(m); !reflect.DeepEqual(got, want) {
		t.Errorf("on 2: %v, want %v", got, want)
	}

	m = pressKey(t, m, ".") // 3: the stop after next is in b.go
	want = map[int]string{10: "previous", 11: "previous", 20: "current", 30: "next"}
	if got := gutterMarks(m); !reflect.DeepEqual(got, want) {
		t.Errorf("on 3: %v, want %v", got, want)
	}

	m = pressKey(t, m, "W") // the tour off
	if got := gutterMarks(m); len(got) != 0 {
		t.Errorf("with the tour off: %v, want no marks", got)
	}
}
