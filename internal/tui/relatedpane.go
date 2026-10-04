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

// absRelated anchors relative paths to the review's checkout, so the editor
// opens the review's files whatever directory its pane ends up in.
func absRelated(root string, files []relatedFile) []relatedFile {
	if root == "" {
		return files
	}
	out := make([]relatedFile, len(files))
	for i, f := range files {
		if !filepath.IsAbs(f.path) {
			f.path = filepath.Join(root, f.path)
		}
		out[i] = f
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

// relatedEditorArgv builds the editor invocation for a set of related files,
// with file active (1-based) the one the editor is left on.
//
// vim and nvim open read-only (-R): the pane shows context, usually in the very
// worktree under review, where one stray keypress in a writable buffer edits the
// change being reviewed. They get every file as a horizontal split (-o) and one
// -c that visits
// each window and puts its file at its line. One -c joined with `|`, not one per
// step as `-c '1wincmd w' -c '40' …` would be: vim runs at most ten -c commands,
// which five files would already exceed. `exe 'normal! NGzz'` rather than a bare
// `:N` because a range followed by `|` is not reliably a jump.
//
// Any other editor opens one file, so it gets the active one, at its line, the
// same way ctrl+g opens a file — writable, since editors share no read-only
// flag.
func relatedEditorArgv(configured string, files []relatedFile, active int) []string {
	if len(files) == 0 {
		return nil
	}
	if active < 1 || active > len(files) {
		active = 1
	}
	name, args := resolveEditor(configured)
	argv := append([]string{name}, args...)
	if !isVimLike(name) {
		f := files[active-1]
		if f.line > 0 {
			argv = append(argv, fmt.Sprintf("+%d", f.line))
		}
		return append(argv, f.path)
	}
	argv = append(argv, "-R", "-o")
	for _, f := range files {
		argv = append(argv, f.path)
	}
	var steps []string
	for i, f := range files {
		if f.line > 0 {
			steps = append(steps, fmt.Sprintf("%dwincmd w", i+1), fmt.Sprintf("exe 'normal! %dGzz'", f.line))
		}
	}
	if len(steps) > 0 || active > 1 {
		steps = append(steps, fmt.Sprintf("%dwincmd w", active))
		argv = append(argv, "-c", strings.Join(steps, "|"))
	}
	return argv
}

// vimCommandLimit is how many +cmd, -c and -S arguments vim and nvim take
// together; --cmd has a limit of its own of the same size. Measured on nvim
// 0.12.4 and vim 9.2: an eleventh fails with `Too many "+command", "-c
// command" or "--cmd command" arguments`.
const vimCommandLimit = 10

// withEditorArgs appends related_editor_args to the related pane's editor
// argv. For vim and nvim it leaves them out, and says why, when they would
// take the commands past vimCommandLimit: the editor would exit on the spot
// and the pane would show nothing.
func withEditorArgs(argv, extra []string) ([]string, error) {
	if len(extra) == 0 || len(argv) == 0 {
		return argv, nil
	}
	out := append(append([]string(nil), argv...), extra...)
	if isVimLike(argv[0]) {
		if cmds, pre := vimCommandCounts(out[1:]); cmds > vimCommandLimit || pre > vimCommandLimit {
			return argv, fmt.Errorf("related_editor_args left out: with monocle's own they make %d +cmd/-c/-S and %d --cmd, and vim takes at most %d of each",
				cmds, pre, vimCommandLimit)
		}
	}
	return out, nil
}

// vimCommandCounts counts vim's command arguments in args: +cmd, -c and -S
// together, and --cmd on its own.
func vimCommandCounts(args []string) (cmds, pre int) {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--":
			return cmds, pre
		case a == "-c":
			cmds++
			i++
		case a == "--cmd":
			pre++
			i++
		case a == "-S", strings.HasPrefix(a, "+"):
			cmds++
		}
	}
	return cmds, pre
}

// relatedPanePlan is everything that decides the tmux command for showing
// related files.
type relatedPanePlan struct {
	existing string // the related-files pane, when one is live; "" = none
	owner    string // Monocle's own pane ($TMUX_PANE): a new split goes beside it
	dir      string // the editor's working directory (the repo root)
	mode     string // editor_mode: "tmux_horizontal" stacks the split, anything else puts it beside
	focus    bool   // whether a NEW split takes focus (a respawn never moves focus)
	reveal   bool   // unzoom Monocle's window first, so the pane can be seen
	argv     []string
	files    []relatedFile // what argv opens, recorded once it has
	// selectPane moves focus to the pane, respawned or new: the reviewer asked
	// to go and read something there.
	selectPane bool
}

// zoomedOption is Monocle's window option for a hidden window: a setup that
// hides a window's splits by zooming Monocle's pane sets it while hidden, and
// Monocle clears it when it unzooms, so that setup does not go on thinking the
// window is hidden.
const zoomedOption = "@monocle_zoomed"

// unzoomArgs are the tmux commands that unzoom the window holding Monocle's
// pane, given that window's #{window_zoomed_flag}: none when it is not zoomed.
// resize-pane -Z toggles, so it must only run on a zoomed window.
func unzoomArgs(owner, zoomedFlag string) [][]string {
	if owner == "" || strings.TrimSpace(zoomedFlag) != "1" {
		return nil
	}
	return [][]string{
		{"resize-pane", "-Z", "-t", owner},
		{"set-option", "-w", "-u", "-t", owner, zoomedOption},
	}
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
	pane   string        // the pane now holding the related files ("" after a close)
	files  []relatedFile // the files it now holds
	closed bool          // this was X
	none   bool          // X found no pane to close
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

// showRelated is showRelatedCmd, as a variable so a test can see the plan
// without a tmux server.
var showRelated = showRelatedCmd

// showRelatedCmd opens files in the related-files pane, reusing it when live.
func showRelatedCmd(tracked string, plan relatedPanePlan) tea.Cmd {
	return func() tea.Msg {
		relatedPaneMu.Lock()
		defer relatedPaneMu.Unlock()
		if plan.reveal {
			// Best effort: if the window stays zoomed the pane still updates,
			// it is just not in sight.
			if flag, err := tmux("display-message", "-p", "-t", plan.owner, "#{window_zoomed_flag}"); err == nil {
				for _, args := range unzoomArgs(plan.owner, flag) {
					_, _ = tmux(args...)
				}
			}
		}
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
		if plan.selectPane {
			_, _ = tmux("select-pane", "-t", pane)
		}
		return relatedPaneMsg{pane: pane, files: plan.files}
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
	return m.showRelatedFiles(files, 1, false, false)
}

// errRelatedNeedsTmux is what showing related files says outside tmux.
var errRelatedNeedsTmux = errors.New("related files open in a tmux pane; monocle is not running in tmux")

// showRelatedFiles puts files in the related-files pane — respawning it when
// live, splitting it when not — with file active (1-based) the one the editor
// is left on. reveal unzooms Monocle's window first; takeFocus moves focus to
// the pane even when it is respawned.
func (m appModel) showRelatedFiles(files []relatedFile, active int, reveal, takeFocus bool) tea.Cmd {
	if !inTmux() {
		return func() tea.Msg { return relatedPaneMsg{err: errRelatedNeedsTmux} }
	}
	files = absRelated(m.repoRoot, files)
	owner := os.Getenv("TMUX_PANE")
	argv, argsErr := withEditorArgs(relatedEditorArgv(m.editorCommand(), files, active), expandOwner(m.relatedEditorArgs(), owner))
	plan := relatedPanePlan{
		owner:      owner,
		dir:        m.repoRoot,
		mode:       m.editorMode(),
		focus:      m.relatedFocus() || takeFocus,
		reveal:     reveal,
		argv:       argv,
		files:      files,
		selectPane: takeFocus,
	}
	show := showRelated(m.tour.pane, plan)
	if argsErr != nil {
		// The files still open; the note says what was left out of the editor.
		return tea.Batch(show, func() tea.Msg { return relatedPaneMsg{err: argsErr} })
	}
	return show
}

// expandOwner replaces {owner} in each of args with Monocle's own tmux pane
// id, so the editor can be told something unique to this Monocle — a server
// socket to listen on, say, that related_editor_add can then reach.
func expandOwner(args []string, owner string) []string {
	if len(args) == 0 {
		return args
	}
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = strings.ReplaceAll(a, "{owner}", owner)
	}
	return out
}

// relatedEditorArgs is the configured related_editor_args.
func (m appModel) relatedEditorArgs() []string {
	if m.engine == nil {
		return nil
	}
	if cfg := m.engine.GetConfig(); cfg != nil {
		return cfg.RelatedEditorArgs
	}
	return nil
}

// tourRelatedMsg asks to bring up one of the current stop's related files —
// `:related 2`, or a click on its label.
type tourRelatedMsg struct{ arg string }

// openStopRelated brings up the related-files pane with all of the current
// stop's related files, as arriving at the stop does, but with file n
// (1-based; empty means the first) the active one — and unzooms Monocle's
// window if it is zoomed, since asking for a file means wanting to see it.
// Keyboard focus follows the same rule as on arriving: a respawn never moves
// it, a new split takes it only with editor_focus.
func (m appModel) openStopRelated(arg string) (appModel, tea.Cmd) {
	stop, ok := m.currentStop()
	if !ok || !m.tour.on {
		m.statusBar.searchInfo = "no tour stop to open a related file from"
		return m, nil
	}
	files := relatedFilesFor(stop)
	if len(files) == 0 {
		m.statusBar.searchInfo = stop.ID + " has no related files"
		return m, nil
	}
	n, ok := stopItemNumber(arg, len(files))
	if !ok {
		m.statusBar.searchInfo = fmt.Sprintf("%s has related files 1-%d", stop.ID, len(files))
		return m, nil
	}
	return m, m.showRelatedFiles(files, n, true, false)
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
		m.tour.pane, m.tour.paneFiles = "", nil
	default:
		m.tour.pane, m.tour.paneFiles = msg.pane, msg.files
	}
	return m
}
