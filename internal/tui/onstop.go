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

// onStopDoneMsg reports how the on-stop command for a stop went.
type onStopDoneMsg struct {
	id  string
	err error
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

// execOnStop runs command through the shell and waits for it, up to timeout.
func execOnStop(command, dir string, env []string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	shell, flag := onStopShell()
	cmd := exec.CommandContext(ctx, shell, flag, command)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
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
// can see it.
func (m appModel) handleOnStopDone(msg onStopDoneMsg) appModel {
	if msg.err != nil {
		m.statusBar.searchInfo = fmt.Sprintf("on-stop %s failed: %v", msg.id, msg.err)
	}
	return m
}

// tourViewMsg asks to open one of the current stop's views — `:view 2`.
type tourViewMsg struct{ arg string }

// openStopView opens view n (1-based; empty means the first) of the current
// stop in the viewer its kind calls for — the same viewers ctrl+p uses — for
// the reviewer who has no on-stop command, or who closed the window it opened.
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
	n := 1
	if arg = strings.TrimSpace(arg); arg != "" {
		if _, err := fmt.Sscanf(arg, "%d", &n); err != nil || n < 1 || n > len(stop.Views) {
			m.statusBar.searchInfo = fmt.Sprintf("%s has views 1-%d", stop.ID, len(stop.Views))
			return m, nil
		}
	}
	v := resolveStopViews(stop.Views[n-1:n], m.repoRoot, artifactFile(m.engine))[0]
	if v.Kind == types.StopViewMarkdown || (v.Kind == types.StopViewArtifact && isMarkdownPath(v.Target)) {
		return m, openInMarkdownViewer(v.Target, m.markdownViewerCommand())
	}
	// Images, video, URLs and media artifacts all open in the media viewer,
	// which is a browser by default and so handles every one of them.
	return m, openInMediaViewer(v.Target, m.mediaViewerCommand())
}
