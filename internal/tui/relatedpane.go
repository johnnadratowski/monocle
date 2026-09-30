package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/josephschmitt/monocle/internal/types"
)

// A tour stop's related files — typically where a change starts, when the stop
// shows where it lands — open beside Monocle in ONE tmux pane running the
// editor, each at its line. The next stop with related files reuses that pane
// (respawn-pane) rather than splitting another, so a tour of twenty stops does
// not leave twenty panes behind; X closes it.
//
// The pane is found again after a Monocle restart through a pane option naming
// the Monocle pane that owns it, so a relaunch does not strand the old split and
// open a second.

// relatedPaneOption tags the related-files pane with its owner's pane id.
const relatedPaneOption = "@monocle_related"

// relatedPaneMu serialises find-then-split, so two stops entered back to back
// cannot both see no pane and both split one.
var relatedPaneMu sync.Mutex

// relatedFile is one file to open, at a line (0 = the top).
type relatedFile struct {
	path string
	line int
}

// relatedFilesFor lists a stop's related files for the editor.
func relatedFilesFor(stop types.WalkthroughStop) []relatedFile {
	out := make([]relatedFile, 0, len(stop.Related))
	for _, r := range stop.Related {
		out = append(out, relatedFile{path: r.Doc, line: r.StartLine})
	}
	return out
}

// isVimLike reports whether an editor takes vim's -o (one window per file,
// stacked) and -c (run a command) — which is what lets several files open at
// once, each at its own line.
func isVimLike(name string) bool {
	switch filepath.Base(name) {
	case "vi", "vim", "nvim", "gvim", "mvim", "lvim":
		return true
	}
	return false
}

// relatedEditorArgv builds the editor invocation for a set of related files.
//
// vim and nvim get every file as a horizontal split (-o) and one -c that visits
// each window and puts its file at its line. One -c joined with `|`, not one per
// step as `-c '1wincmd w' -c '40' …` would be: vim runs at most ten -c commands,
// which five files would already exceed. `exe 'normal! NGzz'` rather than a bare
// `:N` because a range followed by `|` is not reliably a jump.
//
// Any other editor opens one file, so it gets the first, at its line, the same
// way ctrl+g opens a file.
func relatedEditorArgv(configured string, files []relatedFile) []string {
	if len(files) == 0 {
		return nil
	}
	name, args := resolveEditor(configured)
	argv := append([]string{name}, args...)
	if !isVimLike(name) {
		if files[0].line > 0 {
			argv = append(argv, fmt.Sprintf("+%d", files[0].line))
		}
		return append(argv, files[0].path)
	}
	argv = append(argv, "-o")
	for _, f := range files {
		argv = append(argv, f.path)
	}
	var steps []string
	for i, f := range files {
		if f.line > 0 {
			steps = append(steps, fmt.Sprintf("%dwincmd w", i+1), fmt.Sprintf("exe 'normal! %dGzz'", f.line))
		}
	}
	if len(steps) > 0 {
		steps = append(steps, "1wincmd w")
		argv = append(argv, "-c", strings.Join(steps, "|"))
	}
	return argv
}

// relatedPanePlan is everything that decides the tmux command for showing
// related files.
type relatedPanePlan struct {
	existing string // the related-files pane, when one is live; "" = none
	owner    string // Monocle's own pane ($TMUX_PANE): a new split goes beside it
	dir      string // the editor's working directory (the repo root)
	mode     string // editor_mode: "tmux_horizontal" stacks the split, anything else puts it beside
	focus    bool   // whether a NEW split takes focus (a respawn never moves focus)
	argv     []string
}

// relatedPaneArgs builds the tmux arguments that put argv in the related-files
// pane: respawn the live pane in place (reuse), or split a new one beside
// Monocle and print its id so it can be reused next time.
//
// Monocle's own pane is never respawned, whatever the plan says — `respawn-pane
// -k` on it would kill Monocle.
func relatedPaneArgs(p relatedPanePlan) []string {
	cmd := shellJoin(p.argv)
	if p.existing != "" && p.existing != p.owner {
		return []string{"respawn-pane", "-k", "-t", p.existing, "-c", p.dir, cmd}
	}
	args := []string{"split-window"}
	if p.mode == "tmux_horizontal" {
		args = append(args, "-v")
	} else {
		args = append(args, "-h")
	}
	if p.owner != "" {
		args = append(args, "-t", p.owner)
	}
	if !p.focus {
		args = append(args, "-d")
	}
	return append(args, "-c", p.dir, "-P", "-F", "#{pane_id}", cmd)
}

// ownedRelatedPane picks, from `tmux list-panes -a -F '#{pane_id} #{@monocle_related}'`
// output, the pane tagged as owner's related-files pane.
func ownedRelatedPane(listing, owner string) string {
	if owner == "" {
		return ""
	}
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == owner && fields[0] != owner {
			return fields[0]
		}
	}
	return ""
}

// relatedPaneMsg reports what happened to the related-files pane.
type relatedPaneMsg struct {
	pane   string // the pane now holding the related files ("" after a close)
	closed bool   // this was X
	none   bool   // X found no pane to close
	err    error
}

// tmux runs a tmux command, folding its stderr into the error so a failure
// says why ("can't find pane") rather than just "exit status 1".
func tmux(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("tmux %s: %s", args[0], strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("tmux %s: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

// findRelatedPane returns the live related-files pane: the one this Monocle
// last used, or — after a restart has lost that — the one tagged as its own.
func findRelatedPane(tracked, owner string) string {
	if tracked != "" && tracked != owner {
		if id, err := tmux("display-message", "-p", "-t", tracked, "#{pane_id}"); err == nil && id == tracked {
			return tracked
		}
	}
	listing, err := tmux("list-panes", "-a", "-F", "#{pane_id} #{"+relatedPaneOption+"}")
	if err != nil {
		return ""
	}
	return ownedRelatedPane(listing, owner)
}

// showRelatedCmd opens files in the related-files pane, reusing it when live.
func showRelatedCmd(tracked string, plan relatedPanePlan) tea.Cmd {
	return func() tea.Msg {
		relatedPaneMu.Lock()
		defer relatedPaneMu.Unlock()
		plan.existing = findRelatedPane(tracked, plan.owner)
		out, err := tmux(relatedPaneArgs(plan)...)
		if err != nil {
			return relatedPaneMsg{err: err}
		}
		pane := plan.existing
		if pane == "" {
			pane = out
			// Best effort: without the tag the pane is still reused while this
			// Monocle runs; it is only lost across a restart.
			_, _ = tmux("set-option", "-p", "-t", pane, relatedPaneOption, plan.owner)
		}
		return relatedPaneMsg{pane: pane}
	}
}

// closeRelatedCmd closes the related-files pane, if there is one.
func closeRelatedCmd(tracked, owner string) tea.Cmd {
	return func() tea.Msg {
		relatedPaneMu.Lock()
		defer relatedPaneMu.Unlock()
		pane := findRelatedPane(tracked, owner)
		if pane == "" {
			return relatedPaneMsg{closed: true, none: true}
		}
		_, err := tmux("kill-pane", "-t", pane)
		return relatedPaneMsg{closed: true, err: err}
	}
}

// openRelated opens the current stop's related files. A stop with none leaves
// the pane as it is: stepping through a stop that has nothing to add should not
// take away what the last one opened.
func (m appModel) openRelated(stop types.WalkthroughStop) tea.Cmd {
	files := relatedFilesFor(stop)
	if len(files) == 0 {
		return nil
	}
	if !inTmux() {
		return func() tea.Msg {
			return relatedPaneMsg{err: errors.New("related files open in a tmux pane; monocle is not running in tmux")}
		}
	}
	plan := relatedPanePlan{
		owner: os.Getenv("TMUX_PANE"),
		dir:   m.repoRoot,
		mode:  m.editorMode(),
		focus: m.relatedFocus(),
		argv:  relatedEditorArgv(m.editorCommand(), files),
	}
	return showRelatedCmd(m.tour.pane, plan)
}

// relatedFocus is whether a new related-files split takes focus. It follows
// editor_focus when that is set, but defaults to keeping focus on Monocle,
// unlike ctrl+g: the tour is driven from Monocle, and a split that took the
// keyboard on the first stop with related files would swallow the next `.`.
func (m appModel) relatedFocus() bool {
	if m.engine != nil {
		if cfg := m.engine.GetConfig(); cfg != nil && cfg.EditorFocus != nil {
			return *cfg.EditorFocus
		}
	}
	return false
}

// handleRelatedPane records the pane a related-files open landed in, and says
// so when it failed.
func (m appModel) handleRelatedPane(msg relatedPaneMsg) appModel {
	switch {
	case msg.err != nil:
		m.statusBar.searchInfo = "related files: " + msg.err.Error()
	case msg.none:
		m.statusBar.searchInfo = "no related-files pane open"
	case msg.closed:
		m.tour.pane = ""
	default:
		m.tour.pane = msg.pane
	}
	return m
}
