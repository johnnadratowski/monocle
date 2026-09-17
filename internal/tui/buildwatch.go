package tui

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// A TUI is a long-running process reading a binary that is replaced underneath
// it. Installing a new build changes nothing about the sessions already open:
// they keep running the old code, silently, until someone happens to relaunch
// one. On a single terminal that is a small annoyance; across a fleet of panes
// it means never knowing which of them is current.
//
// buildWatch notices when the executable this process was started from changes
// on disk. It compares size and modification time rather than hashing: a deploy
// always moves both, and the check has to be cheap enough to run on a timer
// forever.
type buildWatch struct {
	path    string
	size    int64
	modTime time.Time
	// version is what the binary on disk reports, once a change has been seen
	// and it has been asked. Empty when unknown — the notice still stands, it
	// just cannot name the build.
	version string
	// pending is set once the binary has changed and stays set: an upgrade does
	// not un-happen, and clearing it on a later stat would hide the notice if a
	// second deploy landed mid-read.
	pending bool
}

// newBuildWatch snapshots the running executable. A watch that cannot resolve
// its own path is inert rather than an error: the notice is a convenience, and
// nothing else in the TUI should fail because of it.
func newBuildWatch() buildWatch {
	exe, err := os.Executable()
	if err != nil {
		return buildWatch{}
	}
	w := buildWatch{path: exe}
	if info, err := os.Stat(exe); err == nil {
		w.size, w.modTime = info.Size(), info.ModTime()
	}
	return w
}

// check re-stats the executable and reports whether an upgrade has appeared
// since the last look. The bool is the edge, not the level — it is true only on
// the poll that first sees the change, so the caller can react once.
func (w buildWatch) check() (buildWatch, bool) {
	if w.path == "" || w.pending {
		return w, false
	}
	info, err := os.Stat(w.path)
	if err != nil {
		// Mid-replacement, or removed. Either way there is nothing to report
		// yet; the next poll will see the finished file.
		return w, false
	}
	if info.Size() == w.size && info.ModTime().Equal(w.modTime) {
		return w, false
	}
	w.size, w.modTime = info.Size(), info.ModTime()
	w.pending = true
	w.version = binaryVersion(w.path)
	return w, true
}

// binaryVersion asks the binary on disk what it is. Best-effort and bounded: a
// half-written file, or one that is not executable yet, simply goes unnamed.
func binaryVersion(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// notice renders the one-line upgrade hint for the status bar, or "" when there
// is nothing to say.
func (w buildWatch) notice(key string) string {
	if !w.pending {
		return ""
	}
	what := "a new build"
	if w.version != "" {
		what = w.version
	}
	return "⬆ " + what + " installed · " + key + " to restart"
}

// relaunchRequestMsg asks for a restart onto the installed build. Carried as a
// message so the `:relaunch` command, which cannot touch the model, reaches the
// same code path as the key.
type relaunchRequestMsg struct{}

// requestRelaunch ends the program so the caller can exec the new build in
// place, keeping the terminal (and the tmux pane) it was started in.
//
// It refuses in two cases. With no new build installed there is nothing to
// restart onto, and silently quitting would be a nasty surprise for a mistyped
// key. With an overlay open the reviewer is mid-sentence in a comment editor or
// a modal — that text lives only in this process, and everything else (the
// review, its comments, what is marked reviewed) is in the engine's database
// and survives regardless.
func (m appModel) requestRelaunch() (appModel, tea.Cmd) {
	if !m.buildWatch.pending {
		m.statusBar.searchInfo = "already running the installed build"
		return m, nil
	}
	if m.overlay != overlayNone {
		m.statusBar.searchInfo = "close this first — a restart would discard it"
		return m, nil
	}
	m.relaunchRequested = true
	return m, tea.Quit
}

// WantsRelaunch reports whether the TUI exited asking to be restarted onto a
// newly installed build. The caller execs; doing it here would tear down the
// terminal from under Bubble Tea's own restore.
func WantsRelaunch(final tea.Model) bool {
	m, ok := final.(appModel)
	return ok && m.relaunchRequested
}
