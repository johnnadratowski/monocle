package tui

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestTmuxShellArgs(t *testing.T) {
	// Not in tmux → not ok (takes over instead).
	t.Setenv("TMUX", "")
	if _, ok := tmuxShellArgs(shellSpec{dir: "/x", mode: "tmux_window", focus: true}); ok {
		t.Error("expected ok=false when TMUX unset")
	}

	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")

	// "terminal" and unset modes take over rather than open a tmux pane.
	for _, mode := range []string{"terminal", ""} {
		if _, ok := tmuxShellArgs(shellSpec{dir: "/x", mode: mode, focus: true}); ok {
			t.Errorf("mode %q should take over (ok=false), not open a tmux pane", mode)
		}
	}

	cases := []struct {
		mode  string
		first string
	}{
		{"tmux_vertical", "split-window"},
		{"tmux_horizontal", "split-window"},
		{"tmux_window", "new-window"},
	}
	for _, c := range cases {
		args, ok := tmuxShellArgs(shellSpec{dir: "/proj/dir", mode: c.mode, focus: true})
		if !ok || len(args) == 0 || args[0] != c.first {
			t.Errorf("mode %q: got %v ok=%v, want first %q", c.mode, args, ok, c.first)
		}
		if !contains(args, "/proj/dir") {
			t.Errorf("mode %q: args missing -c dir: %v", c.mode, args)
		}
	}

	// focus=false appends -d (detached).
	if args, _ := tmuxShellArgs(shellSpec{dir: "/x", mode: "tmux_window", focus: false}); !contains(args, "-d") {
		t.Errorf("focus=false should append -d: %v", args)
	}
	if args, _ := tmuxShellArgs(shellSpec{dir: "/x", mode: "tmux_window", focus: true}); contains(args, "-d") {
		t.Errorf("focus=true should not append -d: %v", args)
	}

	// A command is appended (with a pause) as the tmux shell-command argument.
	args, _ := tmuxShellArgs(shellSpec{dir: "/x", mode: "tmux_window", focus: true, command: "wc -l foo.txt"})
	if !strings.Contains(strings.Join(args, " "), "wc -l foo.txt") {
		t.Errorf("command not passed to tmux: %v", args)
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

func TestUserShell(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	if name, flag := userShell(); name != "/bin/zsh" || flag != "-c" {
		t.Errorf("userShell = (%q, %q), want (/bin/zsh, -c)", name, flag)
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

func TestShellTargetFile(t *testing.T) {
	// Changed file → repo-relative path, cwd repo root.
	m := appModel{repoRoot: "/repo", focus: focusSidebar}
	m.sidebar.files = []types.ChangedFile{{Path: "pkg/util.go"}}
	m.sidebar.rebuildGroups()
	arg, cwd, ok := m.shellTargetFile()
	if !ok || arg != "pkg/util.go" || cwd != "/repo" {
		t.Errorf("changed file: got arg=%q cwd=%q ok=%v", arg, cwd, ok)
	}

	// File shown in the diff pane.
	f := appModel{repoRoot: "/repo", focus: focusMain}
	f.diffView.path = "main.go"
	if arg, _, ok := f.shellTargetFile(); !ok || arg != "main.go" {
		t.Errorf("shown file: got arg=%q ok=%v", arg, ok)
	}
}

func TestRenderShellPrompt(t *testing.T) {
	// Cursor at start: the prompt begins with "!" and contains the buffer text.
	out := renderShellPrompt(" foo.txt", 0)
	if !strings.HasPrefix(out, "!") || !strings.Contains(out, "foo.txt") {
		t.Errorf("prompt = %q", out)
	}
	// Out-of-range cursor is clamped without panicking.
	_ = renderShellPrompt("abc", 99)
	_ = renderShellPrompt("abc", -5)
}
