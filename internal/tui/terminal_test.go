package tui

import (
	"runtime"
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestTmuxTerminalArgs(t *testing.T) {
	// Not in tmux → not ok.
	t.Setenv("TMUX", "")
	if _, ok := tmuxTerminalArgs("/x", "tmux_window", true); ok {
		t.Error("expected ok=false when TMUX unset")
	}

	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")
	cases := []struct {
		mode  string
		first string // expected first tmux subcommand
	}{
		{"tmux_vertical", "split-window"},
		{"tmux_horizontal", "split-window"},
		{"tmux_window", "new-window"},
		{"terminal", "new-window"}, // non-tmux editor_mode → new window
		{"", "new-window"},
	}
	for _, c := range cases {
		args, ok := tmuxTerminalArgs("/proj/dir", c.mode, true)
		if !ok || len(args) == 0 || args[0] != c.first {
			t.Errorf("mode %q: got %v ok=%v, want first %q", c.mode, args, ok, c.first)
		}
		if !contains(args, "/proj/dir") {
			t.Errorf("mode %q: args missing -c dir: %v", c.mode, args)
		}
	}

	// focus=false appends -d (detached, keep focus on Monocle).
	args, _ := tmuxTerminalArgs("/x", "tmux_window", false)
	if !contains(args, "-d") {
		t.Errorf("focus=false should append -d: %v", args)
	}
	// focus=true does not.
	args, _ = tmuxTerminalArgs("/x", "tmux_window", true)
	if contains(args, "-d") {
		t.Errorf("focus=true should not append -d: %v", args)
	}
}

func TestOsTerminalArgs(t *testing.T) {
	name, args, ok := osTerminalArgs("/proj")
	switch runtime.GOOS {
	case "darwin":
		if !ok || name != "open" || !contains(args, "Terminal") || !contains(args, "/proj") {
			t.Errorf("darwin: got %q %v ok=%v", name, args, ok)
		}
	case "linux":
		if !ok || !strings.Contains(strings.Join(args, " "), "/proj") {
			t.Errorf("linux: got %q %v ok=%v", name, args, ok)
		}
	}
}

func TestTerminalTargetDir(t *testing.T) {
	// Selected file → its directory.
	m := appModel{repoRoot: "/repo", focus: focusSidebar}
	m.sidebar.files = []types.ChangedFile{{Path: "pkg/util.go"}}
	m.sidebar.rebuildGroups()
	if got := m.terminalTargetDir(); got != "/repo/pkg" {
		t.Errorf("selected file dir = %q, want /repo/pkg", got)
	}

	// Artifact in the diff pane → repo root (no on-disk file).
	a := appModel{repoRoot: "/repo", focus: focusMain}
	a.diffView.contentID = "plan-1"
	if got := a.terminalTargetDir(); got != "/repo" {
		t.Errorf("artifact dir = %q, want /repo", got)
	}

	// File shown in the diff pane → its directory.
	f := appModel{repoRoot: "/repo", focus: focusMain}
	f.diffView.path = "a/b/c.go"
	if got := f.terminalTargetDir(); got != "/repo/a/b" {
		t.Errorf("shown file dir = %q, want /repo/a/b", got)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
