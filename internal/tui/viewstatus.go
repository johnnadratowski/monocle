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
// walkthrough_view_status command is how it says it. Run with MONOCLE_STOP_ID
// and MONOCLE_REPO_ROOT, it prints one line of JSON:
//
//	{"views": {"view": "open", "view2": "hidden"}, "layout": "saved"}
//
// "views" is keyed by stopViewName, each "open", "hidden" or "closed"; a view
// it leaves out, or gives any other value, is closed. "layout" is "saved" when
// the windows are in a layout the reviewer arranged rather than the default.
// No command, a failure, a timeout, or output without a "views" object says
// nothing at all: no markers, no layout. Monocle does not know, so it does not
// say.

// viewStatusTimeout bounds one run of the view-status command. It is asked on
// every stop and after every view opened, so it has to be quick; one that is
// not is treated as having said nothing.
const viewStatusTimeout = 500 * time.Millisecond

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

// tourStatus is what the view-status command said: one state per view of the
// stop, and whether the windows are in a saved layout.
type tourStatus struct {
	views       []viewState
	layoutSaved bool
}

// parseViewStatus reads the view-status command's output for a stop with n
// views.
func parseViewStatus(out []byte, n int) (*tourStatus, error) {
	var raw struct {
		Views  map[string]string `json:"views"`
		Layout string            `json:"layout"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &raw); err != nil {
		return nil, fmt.Errorf("view status is not the expected JSON: %w", err)
	}
	if raw.Views == nil {
		return nil, fmt.Errorf(`view status has no "views" object`)
	}
	st := &tourStatus{views: make([]viewState, n), layoutSaved: raw.Layout == "saved"}
	for i := range st.views {
		switch raw.Views[stopViewName(i+1)] {
		case "open":
			st.views[i] = viewOpen
		case "hidden":
			st.views[i] = viewHidden
		default:
			st.views[i] = viewNotOpened
		}
	}
	return st, nil
}

// tourCommandEnv is the environment of the commands that ask about or change
// the windows around a stop rather than open it: which stop, which repo.
func tourCommandEnv(stopID, repoRoot string) []string {
	return []string{"MONOCLE_STOP_ID=" + stopID, "MONOCLE_REPO_ROOT=" + repoRoot}
}

// viewStatusMsg is the view-status command's answer for a stop: nil status
// when it said nothing usable. seq is the ask it answers.
type viewStatusMsg struct {
	seq    int
	stop   string
	status *tourStatus
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

// refreshViewStatus asks the view-status command about the current stop, in
// the background. Each ask is numbered so that only the answer to the latest
// lands: one that arrives after the reviewer has moved on, or after a newer
// ask, is stale.
func (m *appModel) refreshViewStatus() tea.Cmd {
	command := m.viewStatusCommand()
	stop, ok := m.currentStop()
	if command == "" || !ok || !m.tour.on {
		return nil
	}
	m.tour.statusSeq++
	seq, root := m.tour.statusSeq, m.repoRoot
	return func() tea.Msg {
		msg := viewStatusMsg{seq: seq, stop: stop.ID}
		var out bytes.Buffer
		env := tourCommandEnv(stop.ID, root)
		if err := execHook(command, root, env, viewStatusTimeout, &limitedWriter{w: &out, left: viewStatusMaxOutput}); err != nil {
			return msg
		}
		msg.status, _ = parseViewStatus(out.Bytes(), len(stop.Views))
		return msg
	}
}

// stopStatus is what is known about a stop's windows: the last answer, if it
// was about this stop.
func (m appModel) stopStatus(stop types.WalkthroughStop) *tourStatus {
	if m.tour.statusFor != stop.ID {
		return nil
	}
	return m.tour.status
}

// handleViewStatus takes the view-status command's answer, if it is still the
// one wanted, and redraws the stop's labels with it.
func (m appModel) handleViewStatus(msg viewStatusMsg) appModel {
	stop, ok := m.currentStop()
	if !ok || msg.seq != m.tour.statusSeq || msg.stop != stop.ID {
		return m
	}
	m.tour.status, m.tour.statusFor = msg.status, stop.ID
	if m.tour.on && m.docPane.active && m.docPane.annotationID == tourNoteKeyPrefix+stop.ID {
		m.docPane.setLinks(stopLinks(stop, msg.status))
		recalcPaneDimensions(&m)
		m.diffView.ensureVisible()
	}
	return m
}
