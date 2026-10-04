package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The windows around a tour — where its views sit, the console beside it — can
// be rearranged by the reviewer, and the side that owns them remembers it: the
// view status says "layout": "saved". The tour note then offers to put them
// back, with "Layout: saved · reset", which runs walkthrough_layout_reset.

// layoutResetTimeout bounds one run of the layout-reset command.
const layoutResetTimeout = 10 * time.Second

// tourLayoutMsg asks about or resets the saved layout — `:layout`,
// `:layout reset`, or a click on the reset label.
type tourLayoutMsg struct{ arg string }

// layoutResetDoneMsg reports how the layout-reset command went.
type layoutResetDoneMsg struct{ err error }

// layoutResetCommand is the configured layout-reset command, or "" for none.
func (m appModel) layoutResetCommand() string {
	if m.engine == nil {
		return ""
	}
	if cfg := m.engine.GetConfig(); cfg != nil {
		return strings.TrimSpace(cfg.WalkthroughLayoutReset)
	}
	return ""
}

// handleLayout answers `:layout` with what the view status last said, and runs
// the layout-reset command for `:layout reset`.
func (m appModel) handleLayout(msg tourLayoutMsg) (appModel, tea.Cmd) {
	switch strings.TrimSpace(msg.arg) {
	case "":
		stop, _ := m.currentStop()
		switch st := m.stopStatus(stop); {
		case st == nil:
			m.statusBar.searchInfo = "layout: unknown (no view status)"
		case st.layoutSaved:
			m.statusBar.searchInfo = "layout: saved — :layout reset puts it back"
		default:
			m.statusBar.searchInfo = "layout: default"
		}
		return m, nil
	case "reset":
		return m.resetLayout()
	default:
		m.statusBar.searchInfo = "usage: :layout [reset]"
		return m, nil
	}
}

// resetLayout puts the tour's windows back as a stop starts them. First the
// related-files pane is respawned with the stop's own related files, fresh —
// the way to get back files closed in its editor — then the layout-reset
// command runs in the background, with the same environment as the
// view-status command: the on-stop command's.
func (m appModel) resetLayout() (appModel, tea.Cmd) {
	stop, onStop := m.currentStop()
	var respawn tea.Cmd
	// Outside tmux there is no pane to respawn, and arriving at the stop has
	// already said so.
	if onStop && m.tour.on && inTmux() {
		if files := relatedFilesFor(stop); len(files) > 0 {
			respawn = m.showRelatedFiles(files, 1, false, false)
		}
	}
	command := m.layoutResetCommand()
	if command == "" {
		m.statusBar.searchInfo = "no walkthrough_layout_reset command configured"
		return m, respawn
	}
	engine, root := m.engine, m.repoRoot
	return m, func() tea.Msg {
		var pane tea.Msg
		if respawn != nil {
			pane = respawn()
		}
		done := layoutResetDoneMsg{}
		if env, err := onStopEnv(stop, root, artifactFile(engine)); err != nil {
			done.err = err
		} else {
			done.err = execHook(command, root, env, layoutResetTimeout, io.Discard)
		}
		if pane == nil {
			return done
		}
		return tea.BatchMsg{func() tea.Msg { return pane }, func() tea.Msg { return done }}
	}
}

// handleLayoutResetDone says how the reset went and asks the view status again,
// so the label goes once the layout is back to the default.
func (m appModel) handleLayoutResetDone(msg layoutResetDoneMsg) (appModel, tea.Cmd) {
	if msg.err != nil {
		m.statusBar.searchInfo = fmt.Sprintf("layout reset failed: %v", msg.err)
	} else {
		m.statusBar.searchInfo = "layout reset"
	}
	return m, m.refreshViewStatus()
}
