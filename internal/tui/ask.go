package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

// Asking the agent about code starts in Monocle, where the code is, and ends in
// the agent's window, where the question is typed (John 2026-10-04): select
// lines with v or a drag, or tag them one by one, then send. Monocle sends a
// reference — the tour, the stop, the files and lines — to one configured
// command, walkthrough_ask, which puts it in the agent's prompt and moves focus
// there. Monocle knows nothing about the agent's window.

// lineTag is one line of a file as the agent will read it: the file Monocle
// shows it under (repo-relative, or absolute for an attached file), the side
// of the diff, and its line number on that side.
type lineTag struct {
	path string
	side string // "new", or "old" for a removed line, numbered in the old file
	line int
}

// tagsAt are the file lines the row at index i shows (tagsOf).
func (m diffViewModel) tagsAt(i int) []lineTag {
	if i < 0 || i >= len(m.lines) {
		return nil
	}
	return m.tagsOf(m.lines[i])
}

// tagsOf are the file lines a row shows: none for a hunk header, a comment, an
// annotation or an artifact's row; its one line in the unified and file views,
// on the old side for a removed line; and in the split view its new line and,
// when the left side is a removed line, that too, since both are on screen.
func (m diffViewModel) tagsOf(l diffViewLine) []lineTag {
	if m.path == "" || m.contentMode || m.mediaMode || m.contentID != "" {
		return nil
	}
	if l.isHunk || l.isComment || l.isAnnotation || l.verbatim {
		return nil
	}
	var out []lineTag
	if !l.isSplit {
		switch {
		case l.kind == types.DiffLineRemoved && l.oldLineNum > 0:
			out = append(out, lineTag{m.path, "old", l.oldLineNum})
		case l.kind != types.DiffLineRemoved && l.newLineNum > 0:
			out = append(out, lineTag{m.path, "new", l.newLineNum})
		}
		return out
	}
	if l.rightLineNum > 0 && !l.rightEmpty {
		out = append(out, lineTag{m.path, "new", l.rightLineNum})
	}
	if l.kind == types.DiffLineRemoved && l.oldLineNum > 0 && !l.leftEmpty {
		out = append(out, lineTag{m.path, "old", l.oldLineNum})
	}
	return out
}

// isTagged reports whether a row shows a line the reviewer tagged.
func (m diffViewModel) isTagged(line diffViewLine) bool {
	if len(m.tags) == 0 {
		return false
	}
	for _, t := range m.tagsOf(line) {
		if m.tags[t] {
			return true
		}
	}
	return false
}

// selectedTags are the lines the visual selection covers, or the cursor's line
// when there is no selection.
func (m diffViewModel) selectedTags() []lineTag {
	start, end := m.cursor, m.cursor
	if m.visualMode {
		start, end = m.orderedVisualIndices()
	}
	var out []lineTag
	for i := start; i <= end; i++ {
		out = append(out, m.tagsAt(i)...)
	}
	return out
}

// toggleTags tags the selected lines, or untags them when every one of them is
// tagged already, and leaves visual mode: the selection has become tags. It
// returns how many lines it changed and whether it tagged them.
func (m *diffViewModel) toggleTags() (int, bool) {
	sel := m.selectedTags()
	m.visualMode = false
	if len(sel) == 0 {
		return 0, false
	}
	all := true
	for _, t := range sel {
		all = all && m.tags[t]
	}
	if m.tags == nil {
		m.tags = map[lineTag]bool{}
	}
	for _, t := range sel {
		if all {
			delete(m.tags, t)
		} else {
			m.tags[t] = true
		}
	}
	return len(sel), !all
}

// sendTags is what a send covers: the visual selection and the tags; else the
// tags; else the cursor's line.
func (m diffViewModel) sendTags() []lineTag {
	var out []lineTag
	if m.visualMode || len(m.tags) == 0 {
		out = m.selectedTags()
	}
	for t := range m.tags {
		out = append(out, t)
	}
	return out
}

// askRef is one range of lines the agent is pointed at, as walkthrough_ask
// reads it.
type askRef struct {
	Path  string `json:"path"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	Side  string `json:"side"`
}

// mergeLineTags turns lines into ranges: one per run of adjacent lines on the
// same side of the same file, a line given twice counted once. Ranges are
// ordered by file, then line, then side.
func mergeLineTags(tags []lineTag) []askRef {
	sorted := append([]lineTag(nil), tags...)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.path != b.path {
			return a.path < b.path
		}
		if a.side != b.side {
			return a.side < b.side
		}
		return a.line < b.line
	})
	var out []askRef
	for _, t := range sorted {
		if n := len(out); n > 0 {
			last := &out[n-1]
			if last.Path == t.path && last.Side == t.side && t.line <= last.End+1 {
				last.End = max(last.End, t.line)
				continue
			}
		}
		out = append(out, askRef{Path: t.path, Start: t.line, End: t.line, Side: t.side})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.Side < b.Side
	})
	return out
}

// askPayload is MONOCLE_ASK_JSON: where the reviewer is, and what they point at.
// tour and stop are empty outside tour mode.
type askPayload struct {
	Tour string   `json:"tour"`
	Stop string   `json:"stop"`
	Repo string   `json:"repo"`
	Refs []askRef `json:"refs"`
}

// repoRelative makes an attached file's absolute path relative to the repo
// when it lies inside it, which is how the agent names files.
func repoRelative(root, path string) string {
	if root == "" || !filepath.IsAbs(path) {
		return path
	}
	if rel, err := filepath.Rel(root, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return rel
	}
	return path
}

// askTimeout bounds one run of walkthrough_ask: it types into a window and
// moves focus, which is quick, and a hung one must not linger.
const askTimeout = 10 * time.Second

// askDoneMsg reports how walkthrough_ask went for what it was sent.
type askDoneMsg struct {
	what string
	err  error
}

// askCommand is the configured walkthrough_ask command, or "" for none.
func (m appModel) askCommand() string {
	if m.engine == nil {
		return ""
	}
	if cfg := m.engine.GetConfig(); cfg != nil {
		return strings.TrimSpace(cfg.WalkthroughAsk)
	}
	return ""
}

// tagLines toggles the tag on the cursor's line, or on the visual selection.
// Tags survive moving the cursor, switching files and changing stops, until
// they are sent.
func (m appModel) tagLines() appModel {
	n, tagged := m.diffView.toggleTags()
	switch {
	case n == 0:
		m.statusBar.searchInfo = "no file line here to tag"
	case len(m.diffView.tags) == 0:
		m.statusBar.searchInfo = "no lines tagged"
	default:
		verb := "untagged"
		if tagged {
			verb = "tagged"
		}
		all := len(m.diffView.tags)
		m.statusBar.searchInfo = fmt.Sprintf("%s %d line%s · %d line%s tagged · %s sends them",
			verb, n, plural(n), all, plural(all), PrimaryLabel(m.keys.SendLines))
	}
	return m
}

// sendLines hands what the reviewer points at to walkthrough_ask, then leaves
// visual mode and clears the tags. With no command configured it only says so,
// and keeps the selection and the tags.
func (m appModel) sendLines() (appModel, tea.Cmd) {
	command := m.askCommand()
	if command == "" {
		m.statusBar.searchInfo = "set walkthrough_ask to send lines to the agent"
		return m, nil
	}
	tags := m.diffView.sendTags()
	if len(tags) == 0 {
		m.statusBar.searchInfo = "no file line here to send"
		return m, nil
	}
	for i := range tags {
		tags[i].path = repoRelative(m.repoRoot, tags[i].path)
	}
	payload := askPayload{Repo: m.repoRoot, Refs: mergeLineTags(tags)}
	if stop, ok := m.currentStop(); ok && m.tour.on {
		payload.Tour, payload.Stop = m.tour.tour.Title, stop.ID
	}
	m.diffView.visualMode = false
	m.diffView.tags = nil

	data, err := json.Marshal(payload)
	if err != nil {
		m.statusBar.searchInfo = "sending to the agent failed: " + err.Error()
		return m, nil
	}
	what := askLabel(payload)
	env := []string{"MONOCLE_ASK_JSON=" + string(data), "MONOCLE_REPO_ROOT=" + m.repoRoot}
	root := m.repoRoot
	return m, func() tea.Msg {
		return askDoneMsg{what: what, err: execOnStop(command, root, env, askTimeout)}
	}
}

// handleAskDone says what was sent, or why it was not.
func (m appModel) handleAskDone(msg askDoneMsg) appModel {
	if msg.err != nil {
		m.statusBar.searchInfo = "sending to the agent failed: " + msg.err.Error()
		return m
	}
	m.statusBar.searchInfo = "sent " + msg.what + " to the agent"
	return m
}

// askLabel names what was sent the way the status bar has room for: the stop,
// then up to three ranges by file name — "2.2 walletWithdrawals.ts:312-316".
func askLabel(p askPayload) string {
	var parts []string
	for i, r := range p.Refs {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("+%d more", len(p.Refs)-3))
			break
		}
		s := fmt.Sprintf("%s:%d", filepath.Base(r.Path), r.Start)
		if r.End > r.Start {
			s += fmt.Sprintf("-%d", r.End)
		}
		if r.Side == "old" {
			s += " (old)"
		}
		parts = append(parts, s)
	}
	label := strings.Join(parts, ", ")
	if p.Stop != "" {
		label = p.Stop + " " + label
	}
	return label
}
