package tui

import (
	"strings"
	"testing"
)

// :doc off hides the doc pane and gives the diff its whole column; :doc on
// brings it back as it was. The note is kept while hidden, not closed.
func TestDocOffGivesTheDiffTheRoom(t *testing.T) {
	m := longNoteApp(t)
	diff, doc := m.diffView.height, m.docPane.height
	if !strings.Contains(screenText(m), "Note line 1.") {
		t.Fatal("the stop's note should be showing before :doc off")
	}
	m = typeCommand(t, m, "doc off")
	if strings.Contains(screenText(m), "Note line 1.") {
		t.Errorf(":doc off left the note on screen:\n%s", screenText(m))
	}
	if want := diff + doc + 2; m.diffView.height != want {
		t.Errorf(":doc off: the diff is %d rows, want %d (its own %d, the pane's %d and its border)", m.diffView.height, want, diff, doc)
	}
	if !m.docPane.active {
		t.Error(":doc off closed the note; it should only hide it")
	}
	m = typeCommand(t, m, "doc on")
	if !strings.Contains(screenText(m), "Note line 1.") || m.diffView.height != diff || m.docPane.height != doc {
		t.Errorf(":doc on: diff %d doc %d, want %d and %d with the note showing", m.diffView.height, m.docPane.height, diff, doc)
	}
}

// :doc toggle, :doc alone and the - key each flip it; on and off say what they
// mean however often they are sent (stage sends them without knowing the state).
func TestDocToggleAndKey(t *testing.T) {
	m := longNoteApp(t)
	for _, step := range []struct {
		do    func(appModel) appModel
		shown bool
		what  string
	}{
		{func(m appModel) appModel { return typeCommand(t, m, "doc toggle") }, false, ":doc toggle"},
		{func(m appModel) appModel { return typeCommand(t, m, "doc toggle") }, true, ":doc toggle again"},
		{func(m appModel) appModel { return pressKey(t, m, "-") }, false, "-"},
		{func(m appModel) appModel { return pressKey(t, m, "-") }, true, "- again"},
		{func(m appModel) appModel { return typeCommand(t, m, "doc") }, false, ":doc"},
		{func(m appModel) appModel { return typeCommand(t, m, "doc off") }, false, ":doc off when hidden"},
		{func(m appModel) appModel { return typeCommand(t, m, "doc on") }, true, ":doc on"},
		{func(m appModel) appModel { return typeCommand(t, m, "doc on") }, true, ":doc on when shown"},
	} {
		m = step.do(m)
		if m.docShown() != step.shown {
			t.Fatalf("after %s the doc pane shown is %v, want %v", step.what, m.docShown(), step.shown)
		}
	}
	m = typeCommand(t, m, "doc sideways")
	if !m.docShown() || !strings.Contains(m.statusBar.searchInfo, "usage: :doc") {
		t.Errorf("an unknown :doc argument should change nothing and say the usage: %q", m.statusBar.searchInfo)
	}
}

// Hidden, the pane is out of the focus cycle, and focus in it goes to the diff.
func TestHiddenDocPaneTakesNoFocus(t *testing.T) {
	m := longNoteApp(t)
	m.setFocus(focusDoc)
	m = typeCommand(t, m, "doc off")
	if m.focus != focusMain || m.docPane.focused {
		t.Errorf("hiding the focused doc pane should hand focus to the diff, focus is %v", m.focus)
	}
	for i := 0; i < 4; i++ {
		if m = m.cycleFocus(1); m.focus == focusDoc {
			t.Fatal("tab reached the hidden doc pane")
		}
	}
	if _, ok := m.paneNeighbour(paneDown); ok {
		t.Error("ctrl+j from the diff should not lead into the hidden doc pane")
	}
	m = pressKey(t, m, "=")
	if m.docSize != docSizeDefault {
		t.Error("= sized a hidden doc pane")
	}
}

// Another stop's note comes up shown (stage hides it at a stop's regions step
// and every new stop starts with its note); going to the same stop again, as an
// agent's re-sent tour does, leaves it hidden.
func TestANewStopShowsTheDocAgain(t *testing.T) {
	m := longNoteApp(t)
	m = typeCommand(t, m, "doc off")
	m = typeCommand(t, m, "stop 1.2")
	if m.docShown() {
		t.Error("going to the stop already showing brought the hidden note back")
	}
	m = pressKey(t, m, ",")
	if !m.docShown() || !strings.Contains(screenText(m), "The entry.") {
		t.Errorf("a new stop's note should show:\n%s", screenText(m))
	}
}
