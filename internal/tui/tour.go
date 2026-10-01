package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/josephschmitt/monocle/internal/core"
	"github.com/josephschmitt/monocle/internal/types"
)

// A guided tour is the agent's reading order for the review: stops, each a
// place in the diff plus what the agent wants to say about it. The reviewer
// steps with . and , while the agent answers questions by stop id. Entering a
// stop lands the diff cursor on it, marks its lines, puts its note in the doc
// pane and its position in the status bar.

// tourState is the reviewer's side of the tour. The tour itself and which stop
// is current also live in the engine (so a restart resumes there); this is the
// copy the view answers from.
type tourState struct {
	tour  *types.Walkthrough
	index int  // current stop, or -1
	on    bool // tour mode: W switches it off without losing the tour
	// loading is the file a stop is waiting to show. While set, a diff load for
	// any other file is a stale one racing the tour's jump — a refresh the agent
	// triggered a moment earlier — and must not land on screen, or the note
	// would sit under the wrong file.
	loading string
	// settle numbers stop entries, so the side effects of arriving at a stop —
	// opening windows, running the on-stop command — happen once the reviewer
	// stops moving rather than for every stop they skipped past on the way.
	settle int
	// pane is the tmux pane holding the related files, "" when none is known.
	pane string
}

// tourSettleDelay is how long the reviewer has to rest on a stop before its
// side effects run. Short enough not to feel like lag, long enough that
// holding . does not open and close a split per stop.
const tourSettleDelay = 150 * time.Millisecond

// tourSettledMsg fires tourSettleDelay after a stop is entered.
type tourSettledMsg struct{ seq int }

// tourEventMsg carries a walkthrough_changed engine event into the TUI.
type tourEventMsg struct {
	status string // core.WalkthroughEvent*
	id     string // the stop to show
}

// tourGotoMsg asks for a stop by id — `:stop 1.2`.
type tourGotoMsg struct{ id string }

// tourNoteKeyPrefix marks the doc pane as showing a tour note rather than an
// annotation's refs, so closing the tour closes only what it opened.
const tourNoteKeyPrefix = "tour:"

// hasTour reports whether there is a tour to step through.
func (m appModel) hasTour() bool { return !m.tour.tour.Empty() }

// currentStop returns the stop the reviewer is on.
func (m appModel) currentStop() (types.WalkthroughStop, bool) {
	if !m.hasTour() || m.tour.index < 0 || m.tour.index >= len(m.tour.tour.Stops) {
		return types.WalkthroughStop{}, false
	}
	return m.tour.tour.Stops[m.tour.index], true
}

// tourLabel is the status-bar position: the stop's id, then where it falls in
// the tour — "tour 3.2 · 9 of 9". Both, because the id is what the reviewer
// quotes to the agent and is not a position: "3.2 / 9" read as a fraction, and
// left nobody sure what the 9 counted.
func (m appModel) tourLabel() string {
	stop, ok := m.currentStop()
	if !ok || !m.tour.on {
		return ""
	}
	return fmt.Sprintf("tour %s · %d of %d", stop.ID, m.tour.index+1, len(m.tour.tour.Stops))
}

// syncTour takes the tour the engine holds. It keeps the reviewer on the stop
// they were on when that stop survives, and tears the tour down when the engine
// no longer has one (the review was cleared or approved).
func (m *appModel) syncTour(session *types.ReviewSession) {
	if session == nil || session.Walkthrough.Empty() {
		if m.hasTour() {
			m.leaveTour()
		}
		m.tour = tourState{index: -1}
		return
	}
	prev, _ := m.currentStop()
	m.tour.tour = session.Walkthrough
	m.tour.index = -1
	for _, id := range []string{prev.ID, session.WalkthroughStop} {
		if i := session.Walkthrough.StopIndex(id); id != "" && i >= 0 {
			m.tour.index = i
			break
		}
	}
	if m.tour.index < 0 {
		m.tour.index = 0
	}
	if m.tour.on {
		m.statusBar.tourLabel = m.tourLabel()
	}
}

// stopEntry says what entering a stop should do besides showing it.
type stopEntry struct {
	// report tells the engine the reviewer moved. Off when the engine is the
	// one that moved them (goto_stop, a tour arriving) or nothing moved (a
	// restart restoring the stop the engine already has).
	report bool
	// effects opens the stop's related files (and runs the on-stop command)
	// once the reviewer settles on it. Off only for a restart restoring the
	// stop: resuming is not arriving.
	effects bool
}

// enterStop makes stop i current and shows it.
func (m appModel) enterStop(i int, how stopEntry) (appModel, tea.Cmd) {
	if !m.hasTour() || i < 0 || i >= len(m.tour.tour.Stops) {
		return m, nil
	}
	stop := m.tour.tour.Stops[i]
	m.tour.index = i
	m.tour.on = true
	m.statusBar.tourLabel = m.tourLabel()
	// A notice left from the last key ("end of tour") belongs to the stop being
	// left. A keypress clears it anyway; a move the agent made would not.
	m.statusBar.searchInfo = ""

	m.openStopNote(stop)
	var cmds []tea.Cmd
	if cmd := m.jumpToStop(stop); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if how.report && m.engine != nil {
		engine, id := m.engine, stop.ID
		cmds = append(cmds, func() tea.Msg {
			// Best effort: losing this only costs resuming on a stale stop.
			_ = engine.SetWalkthroughStop(id)
			return nil
		})
	}
	if how.effects {
		m.tour.settle++
		seq := m.tour.settle
		cmds = append(cmds, tea.Tick(tourSettleDelay, func(time.Time) tea.Msg { return tourSettledMsg{seq: seq} }))
	}
	return m, tea.Batch(cmds...)
}

// settleOnStop runs the side effects of arriving at a stop, if the reviewer is
// still on the stop that scheduled them.
func (m appModel) settleOnStop(msg tourSettledMsg) (appModel, tea.Cmd) {
	stop, ok := m.currentStop()
	if !ok || !m.tour.on || msg.seq != m.tour.settle {
		return m, nil
	}
	return m, m.stopEffects(stop)
}

// stopEffects is everything arriving at a stop does outside Monocle's own
// window. Each runs asynchronously and reports back; none blocks the TUI.
func (m appModel) stopEffects(stop types.WalkthroughStop) tea.Cmd {
	return tea.Batch(m.openRelated(stop), m.runOnStop(stop))
}

// stepTour moves one stop forward (+1) or back (-1). The ends clamp rather than
// wrap: a tour is read in order, and going from the last stop back to the first
// on one keypress loses the reader's place in exactly the way a tour exists to
// prevent. With the tour off, a step resumes it on the stop it was left at.
func (m appModel) stepTour(dir int) (appModel, tea.Cmd) {
	if !m.hasTour() {
		m.statusBar.searchInfo = "no tour — the agent has not sent one"
		return m, nil
	}
	if !m.tour.on {
		return m.enterStop(m.tour.index, stopEntry{report: true, effects: true})
	}
	next := m.tour.index + dir
	if next < 0 || next >= len(m.tour.tour.Stops) {
		if dir > 0 {
			m.statusBar.searchInfo = "end of tour"
		} else {
			m.statusBar.searchInfo = "start of tour"
		}
		return m, nil
	}
	return m.enterStop(next, stopEntry{report: true, effects: true})
}

// gotoStop enters a stop by id, from `:stop` or the agent.
func (m appModel) gotoStop(id string, how stopEntry) (appModel, tea.Cmd) {
	if !m.hasTour() {
		m.statusBar.searchInfo = "no tour — the agent has not sent one"
		return m, nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		ids := make([]string, len(m.tour.tour.Stops))
		for i, s := range m.tour.tour.Stops {
			ids[i] = s.ID
		}
		m.statusBar.searchInfo = "stops: " + strings.Join(ids, " ")
		return m, nil
	}
	i := m.tour.tour.StopIndex(id)
	if i < 0 {
		m.statusBar.searchInfo = fmt.Sprintf("no stop %q", id)
		return m, nil
	}
	return m.enterStop(i, how)
}

// toggleTour switches tour mode. Off hides the tour — note, status, marked
// lines — but keeps it and the stop, so switching back on resumes in place.
func (m appModel) toggleTour() (appModel, tea.Cmd) {
	if !m.hasTour() {
		m.statusBar.searchInfo = "no tour — the agent has not sent one"
		return m, nil
	}
	if m.tour.on {
		m.leaveTour()
		m.statusBar.searchInfo = "tour off"
		return m, nil
	}
	return m.enterStop(m.tour.index, stopEntry{report: true, effects: true})
}

// leaveTour takes the tour off screen: its note, its position, its marked
// lines. The stop is kept.
func (m *appModel) leaveTour() {
	m.tour.on = false
	m.tour.loading = ""
	m.statusBar.tourLabel = ""
	m.diffView.stopPath, m.diffView.stopStart, m.diffView.stopEnd = "", 0, 0
	if m.docPane.active && strings.HasPrefix(m.docPane.annotationID, tourNoteKeyPrefix) {
		m.closeDocPane()
	}
}

// openStopNote puts the stop's heading and note in the doc pane, followed by
// what else the stop carries, so the reviewer can see there is a related file
// or a recording without having to know to look. The views are labels a click
// opens.
func (m *appModel) openStopNote(stop types.WalkthroughStop) {
	m.docPane.theme = &m.theme
	m.docPane.openNote(tourNoteKeyPrefix+stop.ID, stop.Heading(), stopNoteBody(stop), stopLinks(stop), m.diffView.mdStyler)
	recalcPaneDimensions(m)
	m.diffView.ensureVisible()
}

// stopNoteBody is the doc pane's text for a stop: the note, then its related
// files. Its views follow as links (stopLinks).
func stopNoteBody(stop types.WalkthroughStop) string {
	var b strings.Builder
	note := strings.TrimSpace(stop.Note)
	if note == "" {
		note = "_(no note)_"
	}
	b.WriteString(note)
	if len(stop.Related) > 0 {
		refs := make([]string, len(stop.Related))
		for i, r := range stop.Related {
			refs[i] = r.Doc
			if r.StartLine > 0 {
				refs[i] += fmt.Sprintf(":%d", r.StartLine)
			}
		}
		b.WriteString("\n\n**Related:** " + strings.Join(refs, " · "))
	}
	return b.String()
}

// stopLinks are a stop's views as the labels under its note, numbered the way
// :view counts them: "[2] url Spec".
func stopLinks(stop types.WalkthroughStop) []noteLink {
	if len(stop.Views) == 0 {
		return nil
	}
	links := make([]noteLink, len(stop.Views))
	for i, v := range stop.Views {
		label := v.Label
		if label == "" {
			label = filepath.Base(v.Target)
		}
		links[i] = noteLink{label: fmt.Sprintf("[%d] %s %s", i+1, v.Kind, label)}
	}
	return links
}

// jumpToStop selects the stop's file and puts the cursor on its first line,
// through the same path ctrl+o uses to return somewhere: record where we were,
// select the file, reload it if it is not the one on screen, land on the line
// once it has loaded. A stop with no file leaves the diff where it is.
func (m *appModel) jumpToStop(stop types.WalkthroughStop) tea.Cmd {
	m.diffView.stopPath, m.diffView.stopStart, m.diffView.stopEnd = "", 0, 0
	if stop.File == "" {
		return nil
	}
	m.diffView.stopStart, m.diffView.stopEnd = stop.LineStart, stop.LineEnd
	if m.diffView.stopEnd < m.diffView.stopStart {
		m.diffView.stopEnd = m.diffView.stopStart
	}
	line := stop.LineStart
	m.recordJump()
	m.setFocus(focusMain)

	if m.reviewHasFile(stop.File) {
		m.diffView.stopPath = stop.File
		m.sidebar.selectPath(stop.File)
		if m.diffView.path == stop.File && !m.diffView.isViewingContentItem() && m.diffView.additionalFilePath == "" {
			if line > 0 {
				m.diffView.GoToLine(line)
			} else {
				m.diffView.LandOnChunkEdge(+1)
			}
			return nil
		}
		m.tour.loading = stop.File
		m.pendingJumpLine = line
		if line <= 0 {
			m.pendingChunkLanding = +1
		}
		path, full := stop.File, m.diffView.fullFile
		return func() tea.Msg { return requestFileDiffMsg{path: path, full: full, anchorLine: line} }
	}

	if af, ok := m.additionalFileFor(stop.File); ok {
		m.diffView.stopPath = af.Path
		m.sidebar.selectAdditionalByPath(af.Path)
		m.tour.loading = af.Path
		m.pendingJumpLine = line
		return m.handleSidebarSelect(sidebarSelectMsg{path: af.Path, isAdditionalFile: true})
	}

	m.statusBar.searchInfo = fmt.Sprintf("%s: %s is not in the review", stop.ID, stop.File)
	return nil
}

// reviewHasFile reports whether a repo-relative path is one of the changed files.
func (m appModel) reviewHasFile(path string) bool {
	for _, f := range m.sidebar.files {
		if f.Path == path {
			return true
		}
	}
	return false
}

// additionalFileFor finds the agent-attached file a stop names, by its display
// name or its path relative to the repo.
func (m appModel) additionalFileFor(path string) (types.AdditionalFile, bool) {
	for _, af := range m.sidebar.additionalFiles {
		if af.Name == path || af.Path == path || af.Path == filepath.Join(m.repoRoot, path) {
			return af, true
		}
	}
	return types.AdditionalFile{}, false
}

// staleForTour reports whether a finished load is for a file other than the
// one a stop is waiting on, and clears the wait when it is the one.
func (m *appModel) staleForTour(path string) bool {
	if m.tour.loading == "" {
		return false
	}
	if path != m.tour.loading {
		return true
	}
	m.tour.loading = ""
	return false
}

// handleTourEvent reacts to the engine: a tour arriving or replaced, the agent
// asking for a stop, the tour withdrawn.
func (m appModel) handleTourEvent(msg tourEventMsg) (appModel, tea.Cmd) {
	if m.engine == nil {
		return m, nil
	}
	m.syncTour(m.engine.GetSession())
	switch msg.status {
	case core.WalkthroughEventCleared:
		return m, nil
	case core.WalkthroughEventSet, core.WalkthroughEventGoto:
		if !m.hasTour() {
			return m, nil
		}
		i := m.tour.tour.StopIndex(msg.id)
		if i < 0 {
			i = m.tour.index
		}
		// The engine already knows where the reviewer is: it put them there.
		return m.enterStop(i, stopEntry{effects: true})
	}
	return m, nil
}
