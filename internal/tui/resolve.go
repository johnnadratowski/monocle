package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Code names things that live elsewhere: a SQL file loaded by name, a function
// in another file. Opening what the lines under the cursor reference, in the
// related pane beside Monocle, is a jump to definition across that gap. Which
// file a line means is project knowledge Monocle does not have, so a
// configured command, walkthrough_resolve, works it out; Monocle runs it and
// opens what it says.

// maxRelatedFiles caps the files in the related pane: past this many split
// windows nothing in any of them can be read.
const maxRelatedFiles = 8

// resolveTimeout bounds one run of walkthrough_resolve.
const resolveTimeout = 10 * time.Second

// resolveMaxOutput caps what Monocle reads from walkthrough_resolve.
const resolveMaxOutput = 64 << 10

// resolveDoneMsg carries what walkthrough_resolve said to open.
type resolveDoneMsg struct {
	files   []relatedFile
	err     error
	preview bool // p: preview the first, rather than open them all
}

// resolveCommand is the configured walkthrough_resolve command, or "".
func (m appModel) resolveCommand() string {
	if m.engine == nil {
		return ""
	}
	if cfg := m.engine.GetConfig(); cfg != nil {
		return strings.TrimSpace(cfg.WalkthroughResolve)
	}
	return ""
}

// openReferences asks walkthrough_resolve what the lines the reviewer points
// at — the same lines @ would send — reference, and opens the answer beside
// what the related pane already holds. Like @ it leaves visual mode and clears
// the tags.
func (m appModel) openReferences(preview bool) (appModel, tea.Cmd) {
	command := m.resolveCommand()
	if command == "" {
		m.statusBar.searchInfo = "set walkthrough_resolve to open what these lines reference"
		return m, nil
	}
	_, data, ok := m.takeRefs("open references from")
	if !ok {
		return m, nil
	}
	env := []string{"MONOCLE_RESOLVE_JSON=" + string(data), "MONOCLE_REPO_ROOT=" + m.repoRoot}
	root := m.repoRoot
	return m, func() tea.Msg {
		var out bytes.Buffer
		if err := execHook(command, root, env, resolveTimeout, &limitedWriter{w: &out, left: resolveMaxOutput}); err != nil {
			return resolveDoneMsg{err: err, preview: preview}
		}
		files, err := parseResolved(out.Bytes(), root)
		return resolveDoneMsg{files: files, err: err, preview: preview}
	}
}

// parseResolved reads walkthrough_resolve's output, a JSON array of
// {"path", "line"}: paths repo-relative or absolute, line 0 or absent for the
// top. Printing nothing means nothing to open.
func parseResolved(out []byte, root string) ([]relatedFile, error) {
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 {
		return nil, nil
	}
	var raw []struct {
		Path string `json:"path"`
		Line int    `json:"line"`
	}
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil, fmt.Errorf("output is not a JSON array of {path, line}: %w", err)
	}
	var files []relatedFile
	for _, r := range raw {
		if p := strings.TrimSpace(r.Path); p != "" {
			files = append(files, relatedFile{path: p, line: max(r.Line, 0)})
		}
	}
	return absRelated(root, files), nil
}

// mergeRelated adds extra to the files the pane holds: a file it already
// holds is not added again, and the total stays within limit, the files
// already there kept first. It returns the files, which of extra were added,
// and how many did not fit.
func mergeRelated(have, extra []relatedFile, limit int) (files, added []relatedFile, left int) {
	seen := make(map[string]bool, len(have)+len(extra))
	files = append(files, have...)
	for _, f := range have {
		seen[f.path] = true
	}
	for _, f := range extra {
		if seen[f.path] {
			continue
		}
		seen[f.path] = true
		if len(files) >= limit {
			left++
			continue
		}
		files = append(files, f)
		added = append(added, f)
	}
	return files, added, left
}

// handleResolveDone opens what walkthrough_resolve found beside what the
// related pane holds — or, before it holds anything, the current stop's
// related files — and moves focus there, the editor on the first file found.
func (m appModel) handleResolveDone(msg resolveDoneMsg) (appModel, tea.Cmd) {
	if msg.err != nil {
		m.statusBar.searchInfo = "walkthrough_resolve failed: " + msg.err.Error()
		return m, nil
	}
	if len(msg.files) == 0 {
		m.statusBar.searchInfo = "nothing to open on these lines"
		return m, nil
	}
	if msg.preview && m.relatedPreviewCommand() != "" && inTmux() {
		return m, m.previewResolved(msg.files)
	}
	if m.relatedAddCommand() != "" {
		return m.addResolved(msg.files)
	}
	have := m.tour.paneFiles
	if stop, ok := m.currentStop(); len(have) == 0 && ok && m.tour.on {
		have = absRelated(m.repoRoot, relatedFilesFor(stop))
	}
	files, added, left := mergeRelated(have, msg.files, maxRelatedFiles)
	active := 1
	for i, f := range files {
		if f.path == msg.files[0].path {
			active = i + 1
			break
		}
	}
	names := make([]string, 0, len(added))
	for _, f := range added {
		names = append(names, filepath.Base(f.path))
	}
	switch {
	case len(added) == 0 && left > 0:
		m.statusBar.searchInfo = fmt.Sprintf("no room: the related pane holds at most %d files", maxRelatedFiles)
	case len(added) == 0:
		m.statusBar.searchInfo = "already open: " + filepath.Base(msg.files[0].path)
	default:
		m.statusBar.searchInfo = fmt.Sprintf("opened %d: %s", len(added), strings.Join(names, ", "))
		if left > 0 {
			m.statusBar.searchInfo += fmt.Sprintf(" · %d left out (at most %d files)", left, maxRelatedFiles)
		}
	}
	return m, m.showRelatedFiles(files, active, true, true)
}

// addResolved opens what walkthrough_resolve found through related_editor_add:
// every file, in order, added to the editor in the live pane, which knows what
// is open in it. Should the pane not be alive, it is spawned with the current
// stop's related files and the files found — not with what it last held,
// which the reviewer may have closed — at most maxRelatedFiles of them.
func (m appModel) addResolved(found []relatedFile) (appModel, tea.Cmd) {
	var base []relatedFile
	if stop, ok := m.currentStop(); ok && m.tour.on {
		base = absRelated(m.repoRoot, relatedFilesFor(stop))
	}
	spawn, _, _ := mergeRelated(base, found, maxRelatedFiles)
	active := 1
	for i, f := range spawn {
		if f.path == found[0].path {
			active = i + 1
			break
		}
	}
	names := make([]string, len(found))
	for i, f := range found {
		names[i] = filepath.Base(f.path)
	}
	m.statusBar.searchInfo = fmt.Sprintf("opened %d: %s", len(found), strings.Join(names, ", "))
	return m, m.addOrShowRelated(found, spawn, active, true, true)
}

// relatedPreviewCommand is the configured related_editor_preview, or "".
func (m appModel) relatedPreviewCommand() string {
	if m.engine == nil {
		return ""
	}
	if cfg := m.engine.GetConfig(); cfg != nil {
		return strings.TrimSpace(cfg.RelatedEditorPreview)
	}
	return ""
}

// relatedPreviewMsg reports a preview: the file shown and how many were left
// out, or, with fallback, that there was no live editor to preview in.
type relatedPreviewMsg struct {
	shown    relatedFile
	more     int
	fallback []relatedFile
	err      error
}

// previewResolved shows the first file found in a passing preview in the
// editor beside Monocle (related_editor_preview), and gives that editor the
// keyboard, so the preview can be read and scrolled; the editor closes it once
// the reviewer moves on. Nothing is added to the pane. With no live pane there
// is no editor to preview in, and the files open as ctrl+] opens them.
func (m appModel) previewResolved(found []relatedFile) tea.Cmd {
	template := m.relatedPreviewCommand()
	owner, tracked, root := os.Getenv("TMUX_PANE"), m.tour.pane, m.repoRoot
	return func() tea.Msg {
		relatedPaneMu.Lock()
		defer relatedPaneMu.Unlock()
		pane := findPane(tracked, owner)
		if pane == "" {
			return relatedPreviewMsg{fallback: found}
		}
		revealOwner(owner)
		if err := execHook(expandAddCommand(template, found[0], owner), root, nil, relatedAddTimeout, io.Discard); err != nil {
			return relatedPreviewMsg{err: fmt.Errorf("related_editor_preview: %w", err)}
		}
		_, _ = tmux("select-pane", "-t", pane)
		return relatedPreviewMsg{shown: found[0], more: len(found) - 1}
	}
}

// handleRelatedPreview says what the preview showed, or opens the files when
// there was nothing to preview in.
func (m appModel) handleRelatedPreview(msg relatedPreviewMsg) (appModel, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.statusBar.searchInfo = "related files: " + msg.err.Error()
		return m, nil
	case msg.fallback != nil:
		return m.addResolved(msg.fallback)
	}
	m.statusBar.searchInfo = "preview: " + filepath.Base(msg.shown.path)
	if msg.more > 0 {
		m.statusBar.searchInfo += fmt.Sprintf(" · %d more, ctrl+] opens them all", msg.more)
	}
	return m, nil
}
