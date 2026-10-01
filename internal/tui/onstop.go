package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/josephschmitt/monocle/internal/core"
	"github.com/josephschmitt/monocle/internal/types"
)

// Monocle knows stops and keys; it knows nothing about windows, browsers or
// video players. Everything a stop needs outside the terminal — showing its
// screenshots and recordings, arranging windows into its layout — is handed to
// one configurable command, walkthrough_on_stop, run on every stop the reviewer
// settles on. The command gets the stop in its environment and is left to it.

// onStopTimeout bounds one run of the on-stop command. Generous, because a
// command that opens a browser can take a while, but finite, because a hung one
// must not pile up behind every later stop.
const onStopTimeout = 30 * time.Second

// onStopDoneMsg reports how the on-stop command for a stop went. view is the
// view it was asked to show (1-based), or 0 for arriving at the stop.
type onStopDoneMsg struct {
	id   string
	view int
	err  error
}

// onStopEnv is what the on-stop command learns about the stop, as environment
// entries: its id, the repo root, and the whole stop as JSON with the views'
// targets resolved — repo-relative paths made absolute, artifact ids made into
// files — so the command never has to know where Monocle was started or how to
// ask it for an artifact.
func onStopEnv(stop types.WalkthroughStop, repoRoot string, artifact func(id string) (string, bool)) ([]string, error) {
	resolved := stop
	resolved.Views = resolveStopViews(stop.Views, repoRoot, artifact)
	data, err := json.Marshal(resolved)
	if err != nil {
		return nil, fmt.Errorf("encode stop: %w", err)
	}
	return []string{
		"MONOCLE_STOP_ID=" + stop.ID,
		"MONOCLE_REPO_ROOT=" + repoRoot,
		"MONOCLE_STOP_JSON=" + string(data),
	}, nil
}

// resolveStopViews makes every view's target something a viewer can open
// directly: a path view relative to the repo becomes absolute, an artifact
// becomes the file holding it. A URL, an absolute path, and an artifact that
// cannot be found are left as they are.
func resolveStopViews(views []types.StopView, repoRoot string, artifact func(id string) (string, bool)) []types.StopView {
	if len(views) == 0 {
		return nil
	}
	out := make([]types.StopView, len(views))
	for i, v := range views {
		switch {
		case v.Kind == types.StopViewArtifact:
			if artifact != nil {
				if p, ok := artifact(v.Target); ok {
					v.Target = p
				}
			}
		case v.IsPathView() && !filepath.IsAbs(v.Target) && !isURL(v.Target):
			v.Target = filepath.Join(repoRoot, v.Target)
		}
		out[i] = v
	}
	return out
}

// stopViewName is what the nth view (1-based) of a stop is called outside
// Monocle: "view", then "view2", "view3"… It is the name the on-stop command
// is given with MONOCLE_VIEW_NAME and the key the view-status command answers
// under, so both sides agree on which window is which view.
func stopViewName(n int) string {
	if n <= 1 {
		return "view"
	}
	return fmt.Sprintf("view%d", n)
}

// stopViewEnv is what the on-stop command learns when it is asked for one view
// rather than run for arriving at the stop: which view, by position and name.
func stopViewEnv(n int) []string {
	return []string{fmt.Sprintf("MONOCLE_VIEW_INDEX=%d", n), "MONOCLE_VIEW_NAME=" + stopViewName(n)}
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "file://")
}

// artifactFile resolves an artifact to a file: a media artifact is its stored
// copy, a text one is written to a temp .md the way ctrl+p does it.
func artifactFile(engine core.EngineAPI) func(id string) (string, bool) {
	return func(id string) (string, bool) {
		if engine == nil {
			return "", false
		}
		item, err := engine.GetContentItem(id)
		if err != nil || item == nil {
			return "", false
		}
		if item.IsMedia() {
			return item.MediaPath, true
		}
		path, err := writeArtifactTempMarkdown(item.Title, item.Content)
		if err != nil {
			return "", false
		}
		return path, true
	}
}

// onStopCommand is the configured command, or "" for none.
func (m appModel) onStopCommand() string {
	if m.engine == nil {
		return ""
	}
	if cfg := m.engine.GetConfig(); cfg != nil {
		return strings.TrimSpace(cfg.WalkthroughOnStop)
	}
	return ""
}

// runOnStop runs the on-stop command for a stop, fire-and-forget: it runs in
// the background with the stop in its environment, its output is discarded
// (it would draw over the TUI) except for the last line of stderr, which is
// what a failure is reported with.
func (m appModel) runOnStop(stop types.WalkthroughStop) tea.Cmd {
	command := m.onStopCommand()
	if command == "" {
		return nil
	}
	engine, root := m.engine, m.repoRoot
	return func() tea.Msg {
		env, err := onStopEnv(stop, root, artifactFile(engine))
		if err != nil {
			return onStopDoneMsg{id: stop.ID, err: err}
		}
		return onStopDoneMsg{id: stop.ID, err: execOnStop(command, root, env, onStopTimeout)}
	}
}

// runStopView runs the on-stop command for one view of the stop — `:view 2`,
// or a click on its label — with the stop's usual environment plus
// stopViewEnv. The command opened the stop's views, in windows Monocle cannot
// see, so it is the one that can bring back the window already showing a view
// rather than open a second.
func (m appModel) runStopView(command string, stop types.WalkthroughStop, n int) tea.Cmd {
	engine, root := m.engine, m.repoRoot
	return func() tea.Msg {
		env, err := onStopEnv(stop, root, artifactFile(engine))
		if err != nil {
			return onStopDoneMsg{id: stop.ID, view: n, err: err}
		}
		env = append(env, stopViewEnv(n)...)
		return onStopDoneMsg{id: stop.ID, view: n, err: execOnStop(command, root, env, onStopTimeout)}
	}
}

// execOnStop runs command through the shell and waits for it, up to timeout.
func execOnStop(command, dir string, env []string, timeout time.Duration) error {
	return execHook(command, dir, env, timeout, io.Discard)
}

// execHook runs a configured tour command through the shell and waits for it,
// up to timeout, writing what it prints to stdout.
func execHook(command, dir string, env []string, timeout time.Duration, stdout io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	shell, flag := onStopShell()
	cmd := exec.CommandContext(ctx, shell, flag, command)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = nil
	cmd.Stdout = stdout
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{w: &stderr, left: 4096}
	// A timeout kills the whole process group, not just the shell, so a
	// viewer the command started cannot outlive it holding the pipes open.
	killGroupOnCancel(cmd)
	cmd.WaitDelay = 2 * time.Second

	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("timed out after %s", timeout)
	}
	if err != nil {
		if line := lastLine(stderr.String()); line != "" {
			return fmt.Errorf("%w: %s", err, line)
		}
		return err
	}
	return nil
}

// limitedWriter keeps the first n bytes and drops the rest, so a chatty command
// cannot grow Monocle's memory.
type limitedWriter struct {
	w    io.Writer
	left int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if l.left > 0 {
		keep := p
		if len(keep) > l.left {
			keep = keep[:l.left]
		}
		l.left -= len(keep)
		_, _ = l.w.Write(keep)
	}
	return n, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// handleOnStopDone surfaces a failed on-stop command. Success is silent: the
// command's effect is on screen, or it is not, and either way the reviewer
// can see it. Either way the command may have opened or raised views, so the
// stop's view markers are asked for again.
func (m appModel) handleOnStopDone(msg onStopDoneMsg) (appModel, tea.Cmd) {
	switch {
	case msg.err == nil:
	case msg.view > 0:
		m.statusBar.searchInfo = fmt.Sprintf("view %d of %s failed: %v", msg.view, msg.id, msg.err)
	default:
		m.statusBar.searchInfo = fmt.Sprintf("on-stop %s failed: %v", msg.id, msg.err)
	}
	if stop, ok := m.currentStop(); !ok || stop.ID != msg.id {
		return m, nil
	}
	return m, m.refreshViewStatus()
}

// stopItemNumber reads the n of `:view n` / `:related n`: 1-based, at most
// count, and the first when arg is empty.
func stopItemNumber(arg string, count int) (int, bool) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return 1, count > 0
	}
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 || n > count {
		return 0, false
	}
	return n, true
}

// tourViewMsg asks to open one of the current stop's views — `:view 2`.
type tourViewMsg struct{ arg string }

// openStopView opens view n (1-based; empty means the first) of the current
// stop. With an on-stop command, the command is asked for it (runStopView):
// it owns the windows views appear in. Without one, the view opens in the
// viewer its kind calls for — the same viewers ctrl+p uses.
func (m appModel) openStopView(arg string) (appModel, tea.Cmd) {
	stop, ok := m.currentStop()
	if !ok || !m.tour.on {
		m.statusBar.searchInfo = "no tour stop to open a view from"
		return m, nil
	}
	if len(stop.Views) == 0 {
		m.statusBar.searchInfo = stop.ID + " has no views"
		return m, nil
	}
	n, ok := stopItemNumber(arg, len(stop.Views))
	if !ok {
		m.statusBar.searchInfo = fmt.Sprintf("%s has views 1-%d", stop.ID, len(stop.Views))
		return m, nil
	}
	if command := m.onStopCommand(); command != "" {
		return m, m.runStopView(command, stop, n)
	}
	v := resolveStopViews(stop.Views[n-1:n], m.repoRoot, artifactFile(m.engine))[0]
	var open tea.Cmd
	if v.Kind == types.StopViewMarkdown || (v.Kind == types.StopViewArtifact && isMarkdownPath(v.Target)) {
		open = openInMarkdownViewer(v.Target, m.markdownViewerCommand())
	} else {
		// Images, video, URLs and media artifacts all open in the media viewer,
		// which is a browser by default and so handles every one of them.
		open = openInMediaViewer(v.Target, m.mediaViewerCommand())
	}
	return m, tea.Batch(open, m.refreshViewStatus())
}
