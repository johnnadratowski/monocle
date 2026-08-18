package tui

import (
	"os"
	"os/exec"

	tea "charm.land/bubbletea/v2"
)

// vim-tmux-navigator semantics: ctrl+h/j/k/l move focus between monocle's own
// panes, and hand off to tmux when there is no pane that way.
//
// The contract that makes this work as one continuous motion is that the key is
// NEVER dropped. If monocle cannot act on a direction — it is at its edge, the
// pane in that direction is hidden, whatever the reason — it must still ask
// tmux to move, or navigation silently stops working in that direction with no
// feedback and no way to tell why.
//
// ctrl+\ is deliberately absent: tmux owns last-pane unconditionally.
//
// The four keys are safe to bind despite arriving as control bytes that other
// input layers fold into Backspace (ctrl+h, 0x08) and Enter (ctrl+j, 0x0A).
// Backspace sends DEL (0x7F) and Enter sends CR (0x0D), so the pairs are
// distinct at the wire, and Bubble Tea v2 keeps them that way — verified with a
// key probe, with and without tmux's extended-keys.

// paneDir is a direction of travel between panes.
type paneDir int

const (
	paneLeft paneDir = iota
	paneDown
	paneUp
	paneRight
)

// selectPaneFlag is the tmux select-pane flag for a direction.
func (d paneDir) selectPaneFlag() string {
	switch d {
	case paneLeft:
		return "-L"
	case paneDown:
		return "-D"
	case paneUp:
		return "-U"
	default:
		return "-R"
	}
}

// inTmux reports whether monocle is running inside tmux, which is the only
// place the hand-off means anything. Outside it the keys keep whatever meaning
// they already had.
func inTmux() bool { return os.Getenv("TMUX") != "" && os.Getenv("TMUX_PANE") != "" }

// handOffToTmux asks tmux to move focus out of monocle's pane. It targets
// TMUX_PANE explicitly rather than the active pane: monocle is not necessarily
// the pane tmux considers active by the time the command runs.
func handOffToTmux(d paneDir) tea.Cmd {
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return nil
	}
	flag := d.selectPaneFlag()
	return func() tea.Msg {
		// Failure is silent on purpose: there is nothing useful to say when the
		// direction has no tmux pane either, which is the common case at the
		// edge of the window.
		_ = exec.Command("tmux", "select-pane", flag, "-t", pane).Run()
		return nil
	}
}

// paneNeighbour returns the pane lying in the given direction, and whether one
// exists. It answers for the CURRENT geometry: the sidebar sits left of the
// diff in the horizontal layout and above it when stacked, and the doc pane —
// when open — always sits below the diff.
func (m appModel) paneNeighbour(d paneDir) (focusTarget, bool) {
	sidebar := !m.sidebarHidden
	doc := m.docPane.active
	stacked := m.layout == layoutStacked

	switch m.focus {
	case focusSidebar:
		// The sidebar is the outermost pane in its axis, so only the direction
		// pointing at the diff leads anywhere.
		if stacked && d == paneDown {
			return focusMain, true
		}
		if !stacked && d == paneRight {
			return focusMain, true
		}

	case focusMain:
		switch d {
		case paneLeft:
			if sidebar && !stacked {
				return focusSidebar, true
			}
		case paneUp:
			if sidebar && stacked {
				return focusSidebar, true
			}
		case paneDown:
			if doc {
				return focusDoc, true
			}
		}

	case focusDoc:
		if d == paneUp {
			return focusMain, true
		}
		// The doc pane spans the diff column, so leaving it sideways means the
		// sidebar in the horizontal layout.
		if d == paneLeft && sidebar && !stacked {
			return focusSidebar, true
		}
	}
	return 0, false
}

// navigatePane moves focus one pane in the given direction, handing off to tmux
// when monocle has nowhere to go. Returns the model and a command; the command
// is non-nil exactly when the key left monocle.
func (m appModel) navigatePane(d paneDir) (appModel, tea.Cmd) {
	if target, ok := m.paneNeighbour(d); ok {
		m.setFocus(target)
		return m, nil
	}
	return m, handOffToTmux(d)
}
