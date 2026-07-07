package tui

import (
	"os"
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
)

// openTerminalDoneMsg is returned after an external shell/terminal exits.
type openTerminalDoneMsg struct {
	err error
}

// shellSpec describes a shell invocation and where to place it.
type shellSpec struct {
	dir      string // working directory
	command  string // "" = interactive shell; otherwise run this command, then pause
	mode     string // editor_mode ("terminal", "tmux_vertical", ...)
	focus    bool   // tmux modes: whether the new pane/window takes focus
	takeover bool   // force taking over Monocle's screen, ignoring tmux modes
}

// runShell launches a shell per spec. With an explicit tmux_* editor_mode (and
// not forced takeover) it opens a tmux split or window, reusing the same
// windowing as the editor. Otherwise — including the default "terminal" mode —
// it takes over Monocle's screen via tea.ExecProcess and returns when the shell
// or command exits.
func runShell(spec shellSpec) tea.Cmd {
	if !spec.takeover {
		if args, ok := tmuxShellArgs(spec); ok {
			cmd := exec.Command("tmux", args...)
			return func() tea.Msg { return openTerminalDoneMsg{err: cmd.Run()} }
		}
	}
	name, cArg := userShell()
	var cmd *exec.Cmd
	if spec.command == "" {
		cmd = exec.Command(name) // interactive shell (stdin is a tty under ExecProcess)
	} else {
		cmd = exec.Command(name, cArg, spec.command+shellPauseSuffix())
	}
	if spec.dir != "" {
		cmd.Dir = spec.dir
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return openTerminalDoneMsg{err: err} })
}

// tmuxShellArgs builds tmux split/window args for explicit tmux_* modes. It
// returns ok=false for "terminal"/unset modes (which take over instead) and when
// not running inside tmux.
func tmuxShellArgs(spec shellSpec) ([]string, bool) {
	if os.Getenv("TMUX") == "" {
		return nil, false
	}
	var out []string
	switch spec.mode {
	case "tmux_vertical":
		out = []string{"split-window", "-h", "-c", spec.dir}
	case "tmux_horizontal":
		out = []string{"split-window", "-v", "-c", spec.dir}
	case "tmux_window":
		out = []string{"new-window", "-c", spec.dir}
	default:
		return nil, false
	}
	if !spec.focus {
		out = append(out, "-d")
	}
	if spec.command != "" {
		// tmux runs the shell-command string via the default shell; append a
		// pause so output stays visible until the user presses enter.
		out = append(out, spec.command+shellPauseSuffix())
	}
	return out, true
}

// userShell returns the user's shell and its "run command" flag.
func userShell() (string, string) {
	if runtime.GOOS == "windows" {
		if sh := os.Getenv("COMSPEC"); sh != "" {
			return sh, "/c"
		}
		return "cmd", "/c"
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh, "-c"
	}
	return "/bin/sh", "-c"
}

// shellPauseSuffix keeps command output on screen until the user presses enter,
// before Monocle's TUI redraws over it.
func shellPauseSuffix() string {
	if runtime.GOOS == "windows" {
		return " & pause"
	}
	return `; printf '\n[press enter to return to monocle] '; read _`
}
