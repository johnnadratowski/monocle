package tui

import (
	"testing"
)

func navModel(focus focusTarget, layout layoutMode, sidebar, doc bool) appModel {
	m := appModel{focus: focus, layout: layout, sidebarHidden: !sidebar}
	m.docPane.active = doc
	return m
}

func TestPaneNeighbour(t *testing.T) {
	const (
		side   = true
		noSide = false
		docOn  = true
		docOff = false
	)

	tests := []struct {
		name   string
		m      appModel
		dir    paneDir
		want   focusTarget
		wantOK bool
	}{
		// Horizontal: sidebar sits left of the diff.
		{"sidebar right reaches the diff", navModel(focusSidebar, layoutHorizontal, side, docOff), paneRight, focusMain, true},
		{"sidebar left is the window edge", navModel(focusSidebar, layoutHorizontal, side, docOff), paneLeft, 0, false},
		{"sidebar up is the window edge", navModel(focusSidebar, layoutHorizontal, side, docOff), paneUp, 0, false},
		{"diff left reaches the sidebar", navModel(focusMain, layoutHorizontal, side, docOff), paneLeft, focusSidebar, true},
		{"diff right is the window edge", navModel(focusMain, layoutHorizontal, side, docOff), paneRight, 0, false},

		// A hidden sidebar is not a pane, so the diff's left edge is the window's.
		{"diff left with the sidebar hidden", navModel(focusMain, layoutHorizontal, noSide, docOff), paneLeft, 0, false},

		// Stacked: the sidebar sits above the diff, so the axis rotates.
		{"stacked sidebar goes down to the diff", navModel(focusSidebar, layoutStacked, side, docOff), paneDown, focusMain, true},
		{"stacked sidebar right is the edge", navModel(focusSidebar, layoutStacked, side, docOff), paneRight, 0, false},
		{"stacked diff goes up to the sidebar", navModel(focusMain, layoutStacked, side, docOff), paneUp, focusSidebar, true},
		{"stacked diff left is the edge", navModel(focusMain, layoutStacked, side, docOff), paneLeft, 0, false},

		// The doc pane sits under the diff in either layout.
		{"diff down reaches an open doc pane", navModel(focusMain, layoutHorizontal, side, docOn), paneDown, focusDoc, true},
		{"diff down is the edge with the doc closed", navModel(focusMain, layoutHorizontal, side, docOff), paneDown, 0, false},
		{"doc up returns to the diff", navModel(focusDoc, layoutHorizontal, side, docOn), paneUp, focusMain, true},
		{"doc down is the window edge", navModel(focusDoc, layoutHorizontal, side, docOn), paneDown, 0, false},
		{"doc left reaches the sidebar", navModel(focusDoc, layoutHorizontal, side, docOn), paneLeft, focusSidebar, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.m.paneNeighbour(tt.dir)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("target = %v, want %v", got, tt.want)
			}
		})
	}
}

// The contract that makes this feel like one continuous motion: a direction
// monocle cannot act on must still reach tmux. Dropping it makes navigation
// stop working one way with no feedback.
func TestEdgeAlwaysHandsOffToTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-501/default,123,0")
	t.Setenv("TMUX_PANE", "%42")

	for _, tc := range []struct {
		name string
		m    appModel
		dir  paneDir
	}{
		{"sidebar at the left edge", navModel(focusSidebar, layoutHorizontal, true, false), paneLeft},
		{"diff at the right edge", navModel(focusMain, layoutHorizontal, true, false), paneRight},
		{"diff below with no doc pane", navModel(focusMain, layoutHorizontal, true, false), paneDown},
		{"stacked sidebar above", navModel(focusSidebar, layoutStacked, true, false), paneUp},
		{"diff left with the sidebar hidden", navModel(focusMain, layoutHorizontal, false, false), paneLeft},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.m.focus
			m, cmd := tc.m.navigatePane(tc.dir)
			if cmd == nil {
				t.Error("an edge must hand the key to tmux, not swallow it")
			}
			if m.focus != before {
				t.Errorf("focus moved to %v at an edge", m.focus)
			}
		})
	}
}

func TestInternalMoveDoesNotReachTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-501/default,123,0")
	t.Setenv("TMUX_PANE", "%42")

	m, cmd := navModel(focusSidebar, layoutHorizontal, true, false).navigatePane(paneRight)
	if cmd != nil {
		t.Error("a move monocle can make itself must not also move tmux")
	}
	if m.focus != focusMain {
		t.Errorf("focus = %v, want the diff pane", m.focus)
	}
	if !m.diffView.focused || m.sidebar.focused {
		t.Error("the per-pane focused flags did not follow")
	}
}

func TestTmuxDetection(t *testing.T) {
	t.Run("both variables are required", func(t *testing.T) {
		t.Setenv("TMUX", "")
		t.Setenv("TMUX_PANE", "%42")
		if inTmux() {
			t.Error("TMUX unset means we are not under tmux")
		}
	})

	t.Run("a pane id alone is not enough", func(t *testing.T) {
		t.Setenv("TMUX", "/tmp/tmux-501/default,123,0")
		t.Setenv("TMUX_PANE", "")
		if inTmux() {
			t.Error("without a pane id there is nothing to hand off to")
		}
	})

	t.Run("outside tmux the hand-off is a no-op", func(t *testing.T) {
		t.Setenv("TMUX_PANE", "")
		if handOffToTmux(paneLeft) != nil {
			t.Error("no pane id should produce no command")
		}
	})
}

func TestSelectPaneFlags(t *testing.T) {
	for dir, want := range map[paneDir]string{
		paneLeft: "-L", paneDown: "-D", paneUp: "-U", paneRight: "-R",
	} {
		if got := dir.selectPaneFlag(); got != want {
			t.Errorf("dir %v -> %q, want %q", dir, got, want)
		}
	}
}
