package tui

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	// pane is the tmux pane holding the files ctrl+] and open-editor opened,
	// "" when none is known, and paneFiles the files it was last given, so
	// more can be added to them.
	pane      string
	paneFiles []relatedFile
	// history is the stops entered, for back and forward (stopHistory).
	history stopHistory
	// seen are the stops the reviewer has been on, seeded from the review
	// when the tour loads (seenTitle: the tour it was seeded for) and kept
	// here after: the engine records a visit as the stop is entered, so its
	// list already holds the stop being arrived at. fresh says whether the
	// current stop was new on arrival, worked out once per stay (freshFor).
	seen      map[string]bool
	seenTitle string
	fresh     bool
	freshFor  string
	// hunk is the uncovered stop's change on screen, an index into its
	// Hunks, and filesShown that the stop showed the file list the tour had
	// hidden, so leaving it hides the list again.
	hunk       int
	filesShown bool
}

// A tour is read in order, but following a call to the stop about the function
// it calls is a detour, and a detour wants a way back. The
// stop history is the browser's back and forward over every stop entered — by
// stepping, `:stop`, a call followed, the agent's goto — kept for the session.

// maxStopHistory bounds the history, as maxJumpList bounds the jump list.
const maxStopHistory = 100

// stopHistory is the stops entered, oldest first, and which of them is the
// current one. Entries after it are the ones back came from, reachable again
// with forward until another stop is entered.
type stopHistory struct {
	ids []string
	at  int // the current stop's index in ids, when ids is not empty
}

// visit records entering a stop. Re-entering the current one records nothing;
// any other drops what forward could reach and goes on the end, as a link
// followed after going back does in a browser.
func (h *stopHistory) visit(id string) {
	if len(h.ids) > 0 {
		if h.ids[h.at] == id {
			return
		}
		h.ids = h.ids[:h.at+1]
	}
	h.ids = append(h.ids, id)
	if len(h.ids) > maxStopHistory {
		h.ids = h.ids[len(h.ids)-maxStopHistory:]
	}
	h.at = len(h.ids) - 1
}

// walk moves back (dir -1) or forward (+1) to the nearest entry that is still a
// stop of the tour and not the one the reviewer is on, and returns it; false
// when there is none that way. A re-sent tour can drop stops the history
// holds, and dropping one can leave the same stop on both sides of it.
func (h *stopHistory) walk(dir int, exists func(string) bool) (string, bool) {
	if len(h.ids) == 0 {
		return "", false
	}
	current := h.ids[h.at]
	for i := h.at + dir; i >= 0 && i < len(h.ids); i += dir {
		if id := h.ids[i]; id != current && exists(id) {
			h.at = i
			return id, true
		}
	}
	return "", false
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

// tourWalkMsg asks to go back (dir -1) or forward (+1) through the stops
// entered — `:back`, `:forward`.
type tourWalkMsg struct{ dir int }

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
	label := fmt.Sprintf("tour %s · %d of %d", stop.ID, m.tour.index+1, len(m.tour.tour.Stops))
	if hunks := m.stopHunks(); len(hunks) > 0 {
		label += fmt.Sprintf(" · hunk %d of %d", m.tour.hunk+1, len(hunks))
	}
	return label
}

// prTourTitle is a tour title that starts with its PR's number, as an agent
// titles a PR's tour: "PR #308 · CMP-8 Signup Fixes", "PR #12: Fix the retry".
var prTourTitle = regexp.MustCompile(`(?i)^PR\s*#(\d+)(?:\s*[·:|—–-]\s*|\s+|$)(.*)$`)

// tourBarTitle is what the top bar calls a tour while it is on: "Tour for PR
// #308: CMP-8 Signup Fixes" for one titled "PR #308 · CMP-8 Signup Fixes" —
// the PR said once, as what the tour is for, not as part of its name — and
// "Tour: <title>" for any other.
func tourBarTitle(title string) string {
	title = strings.TrimSpace(title)
	if g := prTourTitle.FindStringSubmatch(title); g != nil {
		if name := strings.TrimSpace(g[2]); name != "" {
			return fmt.Sprintf("Tour for PR #%s: %s", g[1], name)
		}
		return "Tour for PR #" + g[1]
	}
	if title == "" {
		return "Tour"
	}
	return "Tour: " + title
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
	if m.tour.seen == nil || m.tour.seenTitle != session.Walkthrough.Title {
		m.tour.seen = make(map[string]bool, len(session.WalkthroughVisited))
		for _, id := range session.WalkthroughVisited {
			m.tour.seen[id] = true
		}
		m.tour.seenTitle, m.tour.freshFor = session.Walkthrough.Title, ""
	}
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
	m.tour.hunk = min(m.tour.hunk, max(len(m.stopHunks())-1, 0))
	if m.tour.on {
		m.statusBar.tourLabel = m.tourLabel()
	}
	m.syncUncoveredFiles()
}

// stopEntry says what entering a stop should do besides showing it.
type stopEntry struct {
	// report tells the engine the reviewer moved. Off when the engine is the
	// one that moved them (goto_stop, a tour arriving) or nothing moved (a
	// restart restoring the stop the engine already has).
	report bool
	// effects runs the on-stop command once the reviewer settles on it. Off only for a restart restoring the
	// stop: resuming is not arriving.
	effects bool
	// fromEnd lands on an uncovered stop's last change rather than its
	// first: stepping back into it.
	fromEnd bool
}

// enterStop makes stop i current and shows it.
func (m appModel) enterStop(i int, how stopEntry) (appModel, tea.Cmd) {
	if !m.hasTour() || i < 0 || i >= len(m.tour.tour.Stops) {
		return m, nil
	}
	stop := m.tour.tour.Stops[i]
	// Another stop starts at its first change (its last, stepping back into
	// it); the same one again — a re-sent tour, :stop to where he is — keeps
	// the change he is on.
	if i != m.tour.index || !m.tour.on {
		m.tour.hunk = 0
		if how.fromEnd {
			m.tour.hunk = max(len(stop.Hunks)-1, 0)
		}
	}
	m.tour.index = i
	m.tour.on = true
	// A walk back or forward has already moved the history to this stop, so
	// for it this records nothing, as re-entering a stop does not.
	m.tour.history.visit(stop.ID)
	// New or visited is decided on arrival and held for the stay, so the
	// title does not turn to visited while the reviewer reads.
	if stop.ID != m.tour.freshFor {
		m.tour.fresh, m.tour.freshFor = !m.tour.seen[stop.ID], stop.ID
	}
	if m.tour.seen == nil {
		m.tour.seen = map[string]bool{}
	}
	m.tour.seen[stop.ID] = true
	m.statusBar.tourLabel = m.tourLabel()
	// A notice left from the last key ("end of tour") belongs to the stop being
	// left. A keypress clears it anyway; a move the agent made would not.
	m.statusBar.searchInfo = ""

	// A new stop's note comes up shown; the same stop again (an agent's re-sent
	// tour, :stop to where he is) keeps a note he or the driver hid hidden.
	keepHidden := m.docHidden && m.docPane.annotationID == tourNoteKeyPrefix+stop.ID
	m.openStopNote(stop)
	if keepHidden {
		m.setDocHidden(true)
	}
	m.syncUncoveredFiles()
	var cmds []tea.Cmd
	if cmd := m.jumpToStop(m.landing(stop)); cmd != nil {
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
// window: the on-stop command, which shows the stop's views. It runs
// asynchronously and reports back; it never blocks the TUI.
func (m appModel) stopEffects(stop types.WalkthroughStop) tea.Cmd {
	return m.runOnStop(stop)
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
	if hunks := m.stopHunks(); m.tour.hunk+dir >= 0 && m.tour.hunk+dir < len(hunks) {
		return m.enterHunk(m.tour.hunk + dir)
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
	return m.enterStop(next, stopEntry{report: true, effects: true, fromEnd: dir < 0})
}

// gotoStop enters a stop by id, from `:stop` or the agent.
func (m appModel) gotoStop(id string, how stopEntry) (appModel, tea.Cmd) {
	if !m.hasTour() {
		m.statusBar.searchInfo = "no tour — the agent has not sent one"
		return m, nil
	}
	id = strings.TrimSpace(id)
	// No id is the tour's first stop; a chapter's number, its first stop (`:stop 5` is 5.1): an
	// exact id still wins, and a chapter ends at its dot, so 1 is never 10.1.
	i := 0
	if id != "" {
		i = m.tour.tour.StopIndex(id)
		for j, s := range m.tour.tour.Stops {
			if i >= 0 {
				break
			}
			if strings.HasPrefix(s.ID, id+".") {
				i = j
			}
		}
	}
	if i < 0 {
		m.statusBar.searchInfo = fmt.Sprintf("no stop %q", id)
		return m, nil
	}
	return m.enterStop(i, how)
}

// walkStops goes back (dir -1) or forward (+1) through the stops entered,
// saying where it went, or that there was nowhere to go.
func (m appModel) walkStops(dir int) (appModel, tea.Cmd) {
	if !m.hasTour() {
		m.statusBar.searchInfo = "no tour — the agent has not sent one"
		return m, nil
	}
	id, ok := m.tour.history.walk(dir, func(id string) bool { return m.tour.tour.StopIndex(id) >= 0 })
	if !ok {
		if dir < 0 {
			m.statusBar.searchInfo = "no earlier stop"
		} else {
			m.statusBar.searchInfo = "no later stop"
		}
		return m, nil
	}
	m, cmd := m.enterStop(m.tour.tour.StopIndex(id), stopEntry{report: true, effects: true})
	if dir < 0 {
		m.statusBar.searchInfo = "back to " + id
	} else {
		m.statusBar.searchInfo = "forward to " + id
	}
	return m, cmd
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

// hideFileListForTour hides the file list as a tour starts — a tour arriving
// where there was none, or tour mode restored on launch. The tour moves between
// files itself, so the list is the exception, and the toggle shows it again. A
// re-sent tour (a note fixed mid-review) and W leave it as the reviewer set it.
func (m *appModel) hideFileListForTour() {
	if m.sidebarHidden {
		return
	}
	m.sidebarHidden = true
	m.sidebarAutoHidden = false // ours, not the empty-review auto-hide: items arriving must not undo it
	m.sidebarUserShown = false
	if m.focus == focusSidebar {
		m.setFocus(focusMain)
	}
	recalcPaneDimensions(m)
}

// leaveTour takes the tour off screen: its note, its position, its marked
// lines. The stop is kept.
func (m *appModel) leaveTour() {
	m.tour.on = false
	m.tour.loading = ""
	m.statusBar.tourLabel = ""
	m.diffView.stopPath, m.diffView.stopStart, m.diffView.stopEnd = "", 0, 0
	m.diffView.nearStops, m.diffView.stopMarks = nil, nil
	if m.docPane.active && strings.HasPrefix(m.docPane.annotationID, tourNoteKeyPrefix) {
		m.closeDocPane()
	}
	m.syncUncoveredFiles()
}

// openStopNote puts the stop's heading and note in the doc pane. The note is
// all it holds: a stop's views show in the windows around Monocle, and its
// calls are underlined in its code.
func (m *appModel) openStopNote(stop types.WalkthroughStop) {
	m.docHidden = false
	m.docPane.theme = &m.theme
	m.docPane.openNote(tourNoteKeyPrefix+stop.ID, stop.Heading(), stopNoteBody(stop), m.diffView.mdStyler)
	m.docPane.titleMark = stopVisitMark(m.tour.fresh && m.tour.freshFor == stop.ID)
	m.docPane.titleLoc = stopLocation(m.landing(stop), m.repoRoot)
	recalcPaneDimensions(m)
	m.diffView.ensureVisible()
}

// stopLocation is where a stop is, as its note's header shows it: its file as
// the review names it, repo-relative, and its lines — none for a stop with no
// file.
func stopLocation(stop types.WalkthroughStop, root string) noteLocation {
	if stop.File == "" {
		return noteLocation{}
	}
	loc := noteLocation{path: repoRelative(root, stop.File)}
	switch {
	case stop.LineStart > 0 && stop.LineEnd > stop.LineStart:
		loc.lines = fmt.Sprintf("%d–%d", stop.LineStart, stop.LineEnd)
	case stop.LineStart > 0:
		loc.lines = fmt.Sprint(stop.LineStart)
	}
	return loc
}

// The marks after a stop's title: a bright "new" on a stop the reviewer has not
// been on before, a dim check on one they have.
const (
	stopNewMark     = "new"
	stopVisitedMark = "✓"
)

// stopVisitMark is the styled mark for a stop new on arrival, or visited.
func stopVisitMark(fresh bool) string {
	if fresh {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true).Render(stopNewMark)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(stopVisitedMark)
}

// stopNoteBody is the doc pane's text for a stop: the note.
func stopNoteBody(stop types.WalkthroughStop) string {
	if note := strings.TrimSpace(stop.Note); note != "" {
		return note
	}
	return "_(no note)_"
}

// tourCallMsg asks to enter the stop one of the current stop's calls leads to
// — `:call 2`, or a click on its label.
type tourCallMsg struct{ arg string }

// followStopCall enters the stop that call n (1-based; empty means the first)
// of the current stop leads to, as `:stop` would.
func (m appModel) followStopCall(arg string) (appModel, tea.Cmd) {
	stop, ok := m.currentStop()
	if !ok || !m.tour.on {
		m.statusBar.searchInfo = "no tour stop to follow a call from"
		return m, nil
	}
	if len(stop.Calls) == 0 {
		m.statusBar.searchInfo = stop.ID + " calls no other stop"
		return m, nil
	}
	n, ok := stopItemNumber(arg, len(stop.Calls))
	if !ok {
		m.statusBar.searchInfo = fmt.Sprintf("%s has calls 1-%d", stop.ID, len(stop.Calls))
		return m, nil
	}
	return m.gotoStop(stop.Calls[n-1].Stop, stopEntry{report: true, effects: true})
}

// jumpToStop selects the stop's file and puts the cursor on its first line,
// through the same path ctrl+o uses to return somewhere: record where we were,
// select the file, reload it if it is not the one on screen, land on the line
// once it has loaded. A stop with no file leaves the diff where it is.
func (m *appModel) jumpToStop(stop types.WalkthroughStop) tea.Cmd {
	m.diffView.stopPath, m.diffView.stopStart, m.diffView.stopEnd = "", 0, 0
	m.diffView.nearStops = m.nearStopMarks()
	m.diffView.stopMarks = stopMarksFor(stop)
	if stop.File == "" {
		return nil
	}
	m.diffView.stopStart, m.diffView.stopEnd = stop.LineStart, stop.LineEnd
	if m.diffView.stopEnd < m.diffView.stopStart {
		m.diffView.stopEnd = m.diffView.stopStart
	}
	m.recordJump()
	m.setFocus(focusMain)
	shown, ok := m.reviewPathOf(stop.File)
	if !ok {
		m.statusBar.searchInfo = fmt.Sprintf("%s: %s is not in the review", stop.ID, stop.File)
		return nil
	}
	m.diffView.stopPath = shown
	return m.openFileAt(stop.File, stop.LineStart, stop.LineEnd, nil)
}

// reviewPathOf is the path the diff view shows a file of the review under —
// a changed file's repo path, an added file's absolute one — or false when
// the review has no such file.
func (m appModel) reviewPathOf(file string) (string, bool) {
	if m.reviewHasFile(file) {
		return file, true
	}
	if af, ok := m.additionalFileFor(file); ok {
		return af.Path, true
	}
	return "", false
}

// openFileAt shows a file of the review with the diff cursor on its new-file
// line (line <= 0: its first change), loading it first when another is on
// screen; top, when set, places the line that many rows from the top of the
// diff rather than centring it. Lines line to end are revealed (reveal.go),
// and the file reloaded when a compact diff on screen hides them, or when a
// load of it is already on its way and would land elsewhere. It returns the
// command that finishes the load. The caller has checked the review holds
// the file (reviewPathOf).
func (m *appModel) openFileAt(file string, line, end int, top *int) tea.Cmd {
	if line > 0 {
		// Lines of a generated file someone chose to show are shown, not folded.
		if f, ok := m.changedFileFor(file); ok && f.IsGenerated() {
			m.unfoldGenerated(file)
		}
	}
	if m.reviewHasFile(file) {
		m.sidebar.selectPath(file)
		reload := m.reveal(file, line, end) || m.tour.loading == file
		if !reload && m.diffView.path == file && !m.diffView.isViewingContentItem() && m.diffView.additionalFilePath == "" {
			switch {
			case line > 0 && top != nil:
				m.diffView.GoToLineAt(line, *top)
			case line > 0:
				m.diffView.GoToLine(line)
			default:
				m.diffView.LandOnChunkEdge(+1)
			}
			return nil
		}
		m.tour.loading = file
		m.pendingJumpLine, m.pendingJumpTop = line, top
		if line <= 0 {
			m.pendingChunkLanding = +1
		}
		path, full := file, m.diffView.fullFile
		return func() tea.Msg { return requestFileDiffMsg{path: path, full: full, anchorLine: line} }
	}
	if af, ok := m.additionalFileFor(file); ok {
		m.sidebar.selectAdditionalByPath(af.Path)
		m.tour.loading = af.Path
		m.pendingJumpLine, m.pendingJumpTop = line, top
		return m.handleSidebarSelect(sidebarSelectMsg{path: af.Path, isAdditionalFile: true})
	}
	return nil
}

// highlightRangeMsg sets the highlighted range — new-file lines start to end
// of a file of the review — or, with no path, clears it (highlight_range).
type highlightRangeMsg struct {
	path       string
	start, end int
}

// highlightRange marks a range of lines in a file of the review, replacing any
// earlier one, or clears it. It moves nothing: goto_line does the moving. The
// range shows in its file only, and stays through file switches until it is
// replaced or cleared. Its lines are revealed (reveal.go); when the compact
// diff on screen hides them it reloads, the cursor's line kept where it is on
// screen, unless a jump to another file is on its way.
func (m appModel) highlightRange(msg highlightRangeMsg) (appModel, tea.Cmd) {
	if msg.path == "" {
		m.diffView.hlPath = ""
		return m, nil
	}
	shown, ok := m.reviewPathOf(msg.path)
	if !ok {
		m.statusBar.searchInfo = msg.path + " is not in the review"
		return m, nil
	}
	m.diffView.hlPath, m.diffView.hlStart, m.diffView.hlEnd = shown, msg.start, msg.end
	m.diffView.hlColor = ""
	if m.engine != nil {
		if cfg := m.engine.GetConfig(); cfg != nil {
			m.diffView.hlColor = strings.TrimSpace(cfg.HighlightColor)
		}
	}
	if !m.reviewHasFile(msg.path) || !m.reveal(msg.path, msg.start, msg.end) {
		return m, nil
	}
	if m.tour.loading != "" && m.tour.loading != msg.path {
		return m, nil // the file shows them when the reviewer comes back to it
	}
	if m.pendingJumpLine == 0 {
		if line := m.diffView.anchorLineForCursor(); line > 0 {
			rows := 0
			for i := m.diffView.offset; i < m.diffView.cursor; i++ {
				rows += m.diffView.screenLinesFor(i)
			}
			m.pendingJumpLine, m.pendingJumpTop = line, &rows
		}
	}
	m.tour.loading = msg.path
	path, full, anchor := msg.path, m.diffView.fullFile, m.pendingJumpLine
	return m, func() tea.Msg { return requestFileDiffMsg{path: path, full: full, anchorLine: anchor} }
}

// gotoLineMsg asks to show a file at a new-file line — the agent's goto_line —
// placed top rows from the top of the diff when top is set.
type gotoLineMsg struct {
	path string
	line int
	top  *int
}

// gotoLine shows a file of the review at a line, as a jump (ctrl+o returns),
// with the diff focused. The tour stays on its stop.
func (m appModel) gotoLine(msg gotoLineMsg) (appModel, tea.Cmd) {
	if _, ok := m.reviewPathOf(msg.path); !ok {
		m.statusBar.searchInfo = msg.path + " is not in the review"
		return m, nil
	}
	m.recordJump()
	m.setFocus(focusMain)
	m.statusBar.searchInfo = fmt.Sprintf("showing %s:%d", msg.path, msg.line)
	return m, m.openFileAt(msg.path, msg.line, msg.line, msg.top)
}

// nearStopMarks are the gutter marks for the stops around the current one: the
// next, the one after it and the previous, in that order, so the nearer stop
// ahead wins where they overlap. A stop with no lines, or in a file the review
// does not hold, has none.
func (m appModel) nearStopMarks() []nearStop {
	if !m.hasTour() {
		return nil
	}
	stops := m.tour.tour.Stops
	var marks []nearStop
	for _, near := range []struct {
		offset int
		color  string
	}{{1, nextStopGutterColor}, {2, laterStopGutterColor}, {-1, prevStopGutterColor}} {
		i := m.tour.index + near.offset
		if i < 0 || i >= len(stops) || stops[i].File == "" || stops[i].LineStart <= 0 {
			continue
		}
		s := stops[i]
		path := s.File
		if !m.reviewHasFile(s.File) {
			af, ok := m.additionalFileFor(s.File)
			if !ok {
				continue
			}
			path = af.Path
		}
		marks = append(marks, nearStop{path: path, start: s.LineStart, end: max(s.LineEnd, s.LineStart), color: near.color})
	}
	return marks
}

// stopMarksFor lists the symbols a stop's calls are about, numbered as
// :call N numbers them.
func stopMarksFor(stop types.WalkthroughStop) []stopMark {
	var marks []stopMark
	for i, c := range stop.Calls {
		if c.Symbol != "" {
			marks = append(marks, stopMark{symbol: c.Symbol, n: i + 1, line: c.Line})
		}
	}
	return marks
}

// reviewHasFile reports whether a repo-relative path is one of the changed
// files: one the file list holds, or, while the uncovered stop narrows the
// list, any the review has.
func (m appModel) reviewHasFile(path string) bool {
	files := m.sidebar.files
	if m.sidebar.onlyPaths != nil && m.engine != nil {
		files = m.engine.GetChangedFiles()
	}
	for _, f := range files {
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
	starting := !m.hasTour()
	m.syncTour(m.engine.GetSession())
	// The tour's notes are the cards' text (never which files are generated):
	// the card on screen is drawn again with them.
	m.syncGeneratedCard()
	if starting && msg.status == core.WalkthroughEventSet && m.hasTour() {
		m.hideFileListForTour()
	}
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
		// A tour arriving is still reported, so the first stop the reviewer is
		// shown is recorded as visited; the agent's goto recorded its own.
		return m.enterStop(i, stopEntry{report: msg.status == core.WalkthroughEventSet, effects: true})
	}
	return m, nil
}

// The uncovered stop is the PR's changes no other stop covers, which stage
// adds at a tour's end. It is a stop of many places rather than one: . and ,
// step through its changes (its hunks) and < and > through their files, the
// file list holding only those files; past either end, . and , go on through
// the tour.

// stopHunks are the changes of the stop the reviewer is on, when it is the
// uncovered stop and the tour is on.
func (m appModel) stopHunks() []types.StopHunk {
	stop, ok := m.currentStop()
	if !ok || !m.tour.on || !stop.Uncovered() {
		return nil
	}
	return stop.Hunks
}

// landing is the place in the code a stop shows: its own, or for the
// uncovered stop the change on screen.
func (m appModel) landing(stop types.WalkthroughStop) types.WalkthroughStop {
	if !stop.Uncovered() || len(stop.Hunks) == 0 {
		return stop
	}
	h := stop.Hunks[min(max(m.tour.hunk, 0), len(stop.Hunks)-1)]
	stop.File, stop.LineStart, stop.LineEnd = h.File, h.Start, h.End
	return stop
}

// enterHunk shows the uncovered stop's change h: its lines marked, the cursor
// on them, the note's header and the status bar saying where it is. The stop
// is not re-entered, so the on-stop command does not run again.
func (m appModel) enterHunk(h int) (appModel, tea.Cmd) {
	stop, _ := m.currentStop()
	m.tour.hunk = h
	m.statusBar.tourLabel = m.tourLabel()
	m.statusBar.searchInfo = ""
	at := m.landing(stop)
	m.docPane.titleLoc = stopLocation(at, m.repoRoot)
	return m, m.jumpToStop(at)
}

// stepHunkFile moves to the first change of the next file (dir +1) or the one
// before (-1) on the uncovered stop. At either end it stays and says so;
// on any other stop it moves nothing.
func (m appModel) stepHunkFile(dir int) (appModel, tea.Cmd) {
	hunks := m.stopHunks()
	if len(hunks) == 0 {
		if m.tour.on {
			m.statusBar.searchInfo = "< and > step through the files on the Not covered stop"
		}
		return m, nil
	}
	file := hunks[m.tour.hunk].File
	i := m.tour.hunk
	for i >= 0 && i < len(hunks) && hunks[i].File == file {
		i += dir
	}
	if i < 0 || i >= len(hunks) {
		if dir > 0 {
			m.statusBar.searchInfo = "last file not covered"
		} else {
			m.statusBar.searchInfo = "first file not covered"
		}
		return m, nil
	}
	for i > 0 && hunks[i-1].File == hunks[i].File {
		i--
	}
	return m.enterHunk(i)
}

// syncUncoveredFiles makes the file list fit the stop: on the uncovered stop
// it comes up holding only that stop's files, as wide as their paths need;
// off it, it holds the whole review again, hidden if the stop showed it.
func (m *appModel) syncUncoveredFiles() {
	var only map[string]bool
	longest := 0
	for _, h := range m.stopHunks() {
		if only == nil {
			only = map[string]bool{}
		}
		only[h.File] = true
		longest = max(longest, len(h.File))
	}
	if maps.Equal(only, m.sidebar.onlyPaths) && (only == nil) == (m.sidebar.onlyPaths == nil) {
		return
	}
	m.sidebar.onlyPaths, m.sidebarFit = only, longest
	if m.engine != nil {
		m.sidebar.files = m.engine.GetChangedFiles()
		m.sidebar.contentItems = m.engine.GetContentItems()
		m.sidebar.additionalFiles = m.engine.GetAdditionalFiles()
	}
	m.sidebar.applyReviewedFilter()
	m.sidebar.rebuildTree()
	m.sidebar.clampOffset()
	switch {
	case only != nil && m.sidebarHidden:
		m.sidebarHidden, m.tour.filesShown = false, true
	case only == nil && m.tour.filesShown:
		m.tour.filesShown = false
		m.hideFileListForTour()
	}
	recalcPaneDimensions(m)
}

// sidebarRowChrome is a file row's columns besides its path: status, recent
// mark, icon and the review mark.
const sidebarRowChrome = 11

// maxUncoveredListWidth is the widest the uncovered stop's file list gets: a
// path longer than that is cut, so the code keeps its room.
const maxUncoveredListWidth = 72

// uncoveredListWidth is the uncovered stop's file list's width side by side:
// enough for its longest path, within the list's usual floor of 30 and
// maxUncoveredListWidth, and no more than room, what the diff can spare.
func uncoveredListWidth(longest, room int) int {
	w := min(longest+sidebarRowChrome, maxUncoveredListWidth, room)
	return max(w, 30)
}

// stopKinds are the kinds of stop this TUI shows as more than a place in the
// code. Each is in Features, so a driver asks the running TUI before sending
// one.
var stopKinds = []string{types.StopKindUncovered}
