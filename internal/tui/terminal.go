package tui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
)

// openTerminalDoneMsg is returned after launching an external terminal.
type openTerminalDoneMsg struct {
	err error
}

// openTerminalCmd opens a terminal (the user's shell) at dir. When Monocle runs
// inside tmux it opens a tmux split or window (honoring editor_mode's tmux
// variant, defaulting to a new window), reusing the same windowing as the
// editor. Otherwise it opens a native terminal window for the platform.
func openTerminalCmd(dir, mode string, focus bool) tea.Cmd {
	if args, ok := tmuxTerminalArgs(dir, mode, focus); ok {
		cmd := exec.Command("tmux", args...)
		return func() tea.Msg { return openTerminalDoneMsg{err: cmd.Run()} }
	}
	name, args, ok := osTerminalArgs(dir)
	if !ok {
		return func() tea.Msg {
			return openTerminalDoneMsg{err: fmt.Errorf("no terminal opener for %s", runtime.GOOS)}
		}
	}
	cmd := exec.Command(name, args...)
	return func() tea.Msg { return openTerminalDoneMsg{err: cmd.Run()} }
}

// tmuxTerminalArgs builds `tmux` args to open a shell at dir. It returns
// ok=false when not inside tmux. The tmux split/window type follows editor_mode;
// "terminal" (or unset) opens a new tmux window rather than taking over Monocle.
func tmuxTerminalArgs(dir, mode string, focus bool) ([]string, bool) {
	if os.Getenv("TMUX") == "" {
		return nil, false
	}
	var out []string
	switch mode {
	case "tmux_vertical":
		out = []string{"split-window", "-h", "-c", dir}
	case "tmux_horizontal":
		out = []string{"split-window", "-v", "-c", dir}
	default: // "tmux_window", "terminal", or unset → a new window
		out = []string{"new-window", "-c", dir}
	}
	if !focus {
		out = append(out, "-d")
	}
	return out, true
}

// osTerminalArgs returns the command to open a native terminal window at dir.
func osTerminalArgs(dir string) (string, []string, bool) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{"-a", "Terminal", dir}, true
	case "linux":
		// Best effort: x-terminal-emulator is the Debian alternatives entry.
		return "x-terminal-emulator", []string{"--working-directory=" + dir}, true
	default:
		return "", nil, false
	}
}
