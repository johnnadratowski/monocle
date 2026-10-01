package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

// The on-stop command shows a stop's views in windows Monocle cannot see, so
// whether a view is showing is something only that side can say. The
// walkthrough_view_status command is how it says it: run with the stop's
// environment (onStopEnv), it prints one JSON object naming each view's state,
//
//	{"view": "open", "view2": "hidden"}
//
// keyed by stopViewName. "open" and "hidden" mark the view so; a view the
// object leaves out, or gives any other value, is marked "not opened". No
// command, a failure, a timeout, or output that is not a JSON object of
// strings marks nothing at all: Monocle does not know, so it does not say.

// viewStatusTimeout bounds one run of the view-status command. It is asked on
// every stop and after every view opened, so it has to be quick; one that is
// not is treated as having said nothing.
const viewStatusTimeout = 2 * time.Second

// viewStatusMaxOutput caps what Monocle reads from the view-status command.
const viewStatusMaxOutput = 64 << 10

// viewState is what the view-status command said about one view.
type viewState int

const (
	viewUnknown   viewState = iota // nothing said: no marker
	viewNotOpened                  // not on screen
	viewOpen                       // on screen
	viewHidden                     // open, but out of sight
)

// marker is how a view's state shows beside its label, "" for none.
func (s viewState) marker() string {
	var text, color string
	switch s {
	case viewOpen:
		text, color = "open", "2"
	case viewHidden:
		text, color = "hidden", "3"
	case viewNotOpened:
		text, color = "not opened", "8"
	default:
		return ""
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("(" + text + ")")
}

// parseViewStatus reads the view-status command's output for a stop with n
// views into one state per view.
func parseViewStatus(out []byte, n int) ([]viewState, error) {
	var named map[string]string
	if err := json.Unmarshal(bytes.TrimSpace(out), &named); err != nil {
		return nil, fmt.Errorf("view status is not a JSON object of strings: %w", err)
	}
	if named == nil {
		return nil, fmt.Errorf("view status is null")
	}
	states := make([]viewState, n)
	for i := range states {
		switch strings.TrimSpace(named[stopViewName(i+1)]) {
		case "open":
			states[i] = viewOpen
		case "hidden":
			states[i] = viewHidden
		default:
			states[i] = viewNotOpened
		}
	}
	return states, nil
}

// viewStatusMsg is the view-status command's answer for a stop. states is nil
// when it said nothing usable. seq is the ask it answers.
type viewStatusMsg struct {
	seq    int
	stop   string
	states []viewState
}

// viewStatusCommand is the configured view-status command, or "" for none.
func (m appModel) viewStatusCommand() string {
	if m.engine == nil {
		return ""
	}
	if cfg := m.engine.GetConfig(); cfg != nil {
		return strings.TrimSpace(cfg.WalkthroughViewStatus)
	}
	return ""
}

// refreshViewStatus asks the view-status command about the current stop's
// views, in the background. Each ask is numbered so that only the answer to
// the latest lands: one that arrives after the reviewer has moved on, or after
// a newer ask, is stale.
func (m *appModel) refreshViewStatus() tea.Cmd {
	command := m.viewStatusCommand()
	stop, ok := m.currentStop()
	if command == "" || !ok || !m.tour.on || len(stop.Views) == 0 {
		return nil
	}
	m.tour.statusSeq++
	seq, engine, root := m.tour.statusSeq, m.engine, m.repoRoot
	return func() tea.Msg {
		msg := viewStatusMsg{seq: seq, stop: stop.ID}
		env, err := onStopEnv(stop, root, artifactFile(engine))
		if err != nil {
			return msg
		}
		var out bytes.Buffer
		if err := execHook(command, root, env, viewStatusTimeout, &limitedWriter{w: &out, left: viewStatusMaxOutput}); err != nil {
			return msg
		}
		msg.states, _ = parseViewStatus(out.Bytes(), len(stop.Views))
		return msg
	}
}

// stopViewStates is what is known about a stop's views: the last answer, if it
// was about this stop.
func (m appModel) stopViewStates(stop types.WalkthroughStop) []viewState {
	if m.tour.viewsFor != stop.ID {
		return nil
	}
	return m.tour.views
}

// handleViewStatus takes the view-status command's answer, if it is still the
// one wanted, and redraws the stop's view labels with it.
func (m appModel) handleViewStatus(msg viewStatusMsg) appModel {
	stop, ok := m.currentStop()
	if !ok || msg.seq != m.tour.statusSeq || msg.stop != stop.ID {
		return m
	}
	m.tour.views, m.tour.viewsFor = msg.states, stop.ID
	if m.tour.on && m.docPane.active && m.docPane.annotationID == tourNoteKeyPrefix+stop.ID {
		m.docPane.setLinks(stopLinks(stop, msg.states))
		recalcPaneDimensions(&m)
		m.diffView.ensureVisible()
	}
	return m
}
